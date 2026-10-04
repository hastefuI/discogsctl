package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"go.hasteful.org/discogsctl/api"
	"go.hasteful.org/discogsctl/internal/cli"
)

func newMasterCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "master",
		Short: "Read master releases",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "get <id>",
		Short: "Get a master release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("master", args[0])
			if err != nil {
				return err
			}
			m, err := cfg.client.Master(cmd.Context(), id)
			if err != nil {
				return err
			}
			return cfg.print(cmd, m)
		},
	})

	versions := &cobra.Command{
		Use:     "versions <id>",
		Short:   "List the releases under a master",
		Example: "  discogsctl master versions 1000 --all --output json",
		Args:    cobra.ExactArgs(1),
	}
	pages := cli.BindPageFlags(versions.Flags())
	versions.RunE = listRun(cfg, pages, func(ctx context.Context, args []string, page api.Page) (*api.Paginated[api.MasterVersion], error) {
		id, err := parseID("master", args[0])
		if err != nil {
			return nil, err
		}
		return cfg.client.MasterVersions(ctx, id, page)
	})
	cmd.AddCommand(versions)

	return cmd
}
