package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"go.hasteful.org/discogsctl/api"
	"go.hasteful.org/discogsctl/internal/cli"
)

func newLabelCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label",
		Short: "Read labels",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "get <id>",
		Short: "Get a label",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("label", args[0])
			if err != nil {
				return err
			}
			l, err := cfg.client.Label(cmd.Context(), id)
			if err != nil {
				return err
			}
			return cfg.print(cmd, l)
		},
	})

	releases := &cobra.Command{
		Use:   "releases <id>",
		Short: "List the releases on a label",
		Args:  cobra.ExactArgs(1),
	}
	pages := cli.BindPageFlags(releases.Flags())
	releases.RunE = listRun(cfg, pages, func(ctx context.Context, args []string, page api.Page) (*api.Paginated[api.LabelRelease], error) {
		id, err := parseID("label", args[0])
		if err != nil {
			return nil, err
		}
		return cfg.client.LabelReleases(ctx, id, page)
	})
	cmd.AddCommand(releases)

	return cmd
}
