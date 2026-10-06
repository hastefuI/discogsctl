package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"go.hasteful.org/discogsctl/api"
	"go.hasteful.org/discogsctl/internal/cli"
)

func newWantlistCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wantlist",
		Short: "Read a user's wantlist",
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List the releases in a wantlist",
		Long:  "List the releases in a wantlist. Notes are shown only to the owner.",
		Args:  cobra.NoArgs,
	}
	var username string
	list.Flags().StringVar(&username, "username", "", usernameHelp)
	pages := cli.BindPageFlags(list.Flags())
	list.RunE = listRun(cfg, pages, func(ctx context.Context, args []string, page api.Page) (*api.Paginated[api.Want], error) {
		user, err := cfg.username(ctx, username)
		if err != nil {
			return nil, err
		}
		return cfg.client.Wantlist(ctx, user, page)
	})
	cmd.AddCommand(list)
	cmd.AddCommand(newWantAddCmd(cfg), newWantEditCmd(cfg), newWantRemoveCmd(cfg))

	return cmd
}

func newWantAddCmd(cfg *config) *cobra.Command {
	var notes string
	cmd := &cobra.Command{
		Use:   "add <release_id>",
		Short: "Add a release to your wantlist",
		Long: `Add a release to your wantlist, with optional notes. This needs a token.

Discogs ignores notes sent with an add, so --notes is set by a second
request. A rating is not set here: Discogs keeps it on the release, so use
release rate.`,
		Example: `  discogsctl wantlist add 182213
  discogsctl wantlist add 182213 --notes "first press only"`,
		Args: cobra.ExactArgs(1),
	}
	cmd.Flags().StringVar(&notes, "notes", "", "your notes on the release")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return writeWant(cmd, cfg, args[0], func(ctx context.Context, user string, id int) (*api.Want, error) {
			return cfg.client.AddWant(ctx, user, id, notes)
		})
	}
	return cmd
}

func newWantEditCmd(cfg *config) *cobra.Command {
	var notes string
	cmd := &cobra.Command{
		Use:   "edit <release_id>",
		Short: "Change your notes on a release in your wantlist",
		Long: `Change your notes on a release in your wantlist. This needs a token.

Discogs ignores empty notes, so an edit cannot clear them. To clear them,
remove the release and add it again, which also resets its date added.

Discogs shows a change in wantlist list after a delay, which has been 20 to
40 seconds; the entry printed here is the new one.`,
		Example: `  discogsctl wantlist edit 182213 --notes "any pressing"`,
		Args:    cobra.ExactArgs(1),
	}
	cmd.Flags().StringVar(&notes, "notes", "", "your notes on the release")
	cmd.MarkFlagRequired("notes")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return writeWant(cmd, cfg, args[0], func(ctx context.Context, user string, id int) (*api.Want, error) {
			return cfg.client.EditWant(ctx, user, id, notes)
		})
	}
	return cmd
}

func newWantRemoveCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <release_id>",
		Short:   "Remove a release from your wantlist",
		Long:    "Remove a release from your wantlist, with its notes. A rating you gave the\nrelease stays, since Discogs keeps it on the release. This needs a token.",
		Example: "  discogsctl wantlist remove 182213",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("release", args[0])
			if err != nil {
				return err
			}
			user, err := cfg.tokenHolder(cmd.Context())
			if err != nil {
				return err
			}
			if err := cfg.client.RemoveWant(cmd.Context(), user, id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Removed release %d from the wantlist of %s\n", id, user)
			return nil
		},
	}
}

// writeWant checks the release ID, resolves the token holder, makes the
// change and prints the entry Discogs returns.
func writeWant(cmd *cobra.Command, cfg *config, arg string, write func(context.Context, string, int) (*api.Want, error)) error {
	id, err := parseID("release", arg)
	if err != nil {
		return err
	}
	user, err := cfg.tokenHolder(cmd.Context())
	if err != nil {
		return err
	}
	w, err := write(cmd.Context(), user, id)
	if err != nil {
		return err
	}
	return cfg.print(cmd, w)
}
