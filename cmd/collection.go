package cmd

import (
	"context"

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
		Example: "  discogsctl collection list --all --output json\n  discogsctl collection list --username hasteful --folder 0 --all --output json",
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

	return cmd
}
