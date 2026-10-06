package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"go.hasteful.org/discogsctl/api"
	"go.hasteful.org/discogsctl/internal/cli"
)

const usernameHelp = "Discogs username (default the token holder)"

func newCollectionCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collection",
		Short: "Read a user's collection",
		Long: `Read a user's collection. Without --username, the collection is the token
holder's. Folder 0 holds every release; any other folder, and a private
collection, needs a token for the owner.`,
	}

	var username string
	cmd.PersistentFlags().StringVar(&username, "username", "", usernameHelp)

	cmd.AddCommand(&cobra.Command{
		Use:   "folders",
		Short: "List the folders in a collection",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			user, err := cfg.username(cmd.Context(), username)
			if err != nil {
				return err
			}
			folders, err := cfg.client.CollectionFolders(cmd.Context(), user)
			if err != nil {
				return err
			}
			return cfg.print(cmd, folders)
		},
	})

	list := &cobra.Command{
		Use:     "list",
		Short:   "List the releases in a collection folder",
		Example: "  discogsctl collection list --all --output json\n  discogsctl collection list --username <username> --folder 0 --all --output json",
		Args:    cobra.NoArgs,
	}
	var folder int
	list.Flags().IntVar(&folder, "folder", api.AllFolder, "folder ID; 0 is every release")
	pages := cli.BindPageFlags(list.Flags())
	list.RunE = listRun(cfg, pages, func(ctx context.Context, args []string, page api.Page) (*api.Paginated[api.CollectionItem], error) {
		user, err := cfg.username(ctx, username)
		if err != nil {
			return nil, err
		}
		return cfg.client.CollectionItems(ctx, user, folder, page)
	})
	cmd.AddCommand(list)

	cmd.AddCommand(&cobra.Command{
		Use:   "value",
		Short: "Show the estimated value of a collection",
		Long:  "Show the minimum, median and maximum value of a collection. Discogs only shows this to the owner.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			user, err := cfg.username(cmd.Context(), username)
			if err != nil {
				return err
			}
			v, err := cfg.client.CollectionValue(cmd.Context(), user)
			if err != nil {
				return err
			}
			return cfg.print(cmd, v)
		},
	})

	cmd.AddCommand(newCollectionAddCmd(cfg), newCollectionRemoveCmd(cfg))

	return cmd
}

// ownCollection resolves the token holder for a write, which only ever
// changes their own collection, so --username is refused rather than ignored.
func ownCollection(cmd *cobra.Command, cfg *config) (string, error) {
	if cmd.Flags().Changed("username") {
		return "", fmt.Errorf("%s changes your own collection, so it does not take --username", cmd.CommandPath())
	}
	return cfg.tokenHolder(cmd.Context())
}

func newCollectionAddCmd(cfg *config) *cobra.Command {
	var folder int
	cmd := &cobra.Command{
		Use:   "add <release_id>",
		Short: "Add a release to your collection",
		Long: `Add a copy of a release to a folder in your collection, by default
Uncategorized (folder 1). Adding a release you already have adds another
copy. Folder IDs are in collection folders. This needs a token.`,
		Example: "  discogsctl collection add 5077187\n  discogsctl collection add 5077187 --folder 3",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("release", args[0])
			if err != nil {
				return err
			}
			if folder < 1 {
				return fmt.Errorf("--folder must be 1 or more, got %d; folder 0 holds every release and cannot be added to", folder)
			}
			user, err := ownCollection(cmd, cfg)
			if err != nil {
				return err
			}
			i, err := cfg.client.AddToCollection(cmd.Context(), user, folder, id)
			if err != nil {
				return err
			}
			return cfg.print(cmd, i)
		},
	}
	cmd.Flags().IntVar(&folder, "folder", api.UncategorizedFolder, "folder ID to add the release to")
	return cmd
}

func newCollectionRemoveCmd(cfg *config) *cobra.Command {
	var instance int
	cmd := &cobra.Command{
		Use:   "remove <release_id>",
		Short: "Remove a release from your collection",
		Long: `Remove a copy of a release from your collection. With one copy, that copy
is removed. With more than one, --instance chooses which, and the command
lists them when it is missing. A release not in the collection exits 4.
This needs a token.`,
		Example: "  discogsctl collection remove 5077187\n  discogsctl collection remove 5077187 --instance 1234567890",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("release", args[0])
			if err != nil {
				return err
			}
			user, err := ownCollection(cmd, cfg)
			if err != nil {
				return err
			}
			copies, err := cfg.client.CollectionInstances(cmd.Context(), user, id)
			if err != nil {
				return err
			}
			c, err := pickInstance(copies, id, instance, cmd.Flags().Changed("instance"))
			if err != nil {
				return err
			}
			if err := cfg.client.RemoveFromCollection(cmd.Context(), user, c.FolderID, id, c.InstanceID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Removed instance %d of release %d from folder %d\n", c.InstanceID, id, c.FolderID)
			return nil
		},
	}
	cmd.Flags().IntVar(&instance, "instance", 0, "instance ID of the copy to remove, needed when you have more than one")
	return cmd
}

// pickInstance chooses the copy of release id to remove: the one named by
// instance when given, otherwise the only copy.
func pickInstance(copies []api.CollectionItem, id, instance int, given bool) (api.CollectionItem, error) {
	if len(copies) == 0 {
		return api.CollectionItem{}, fmt.Errorf("release %d is not in your collection: %w", id, cli.ErrNotFound)
	}
	if given {
		for _, c := range copies {
			if c.InstanceID == instance {
				return c, nil
			}
		}
		return api.CollectionItem{}, fmt.Errorf("release %d has no instance %d in your collection: %w", id, instance, cli.ErrNotFound)
	}
	if len(copies) == 1 {
		return copies[0], nil
	}
	list := make([]string, len(copies))
	for i, c := range copies {
		list[i] = fmt.Sprintf("instance %d in folder %d", c.InstanceID, c.FolderID)
	}
	return api.CollectionItem{}, fmt.Errorf("release %d is in your collection %d times; choose one with --instance: %s", id, len(copies), strings.Join(list, ", "))
}
