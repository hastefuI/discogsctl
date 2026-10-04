package cmd

import (
	"context"

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

	return cmd
}
