package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"go.hasteful.org/discogsctl/api"
	"go.hasteful.org/discogsctl/internal/cli"
)

func newArtistCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artist",
		Short: "Read artists",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "get <id>",
		Short: "Get an artist",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("artist", args[0])
			if err != nil {
				return err
			}
			a, err := cfg.client.Artist(cmd.Context(), id)
			if err != nil {
				return err
			}
			return cfg.print(cmd, a)
		},
	})

	releases := &cobra.Command{
		Use:   "releases <id>",
		Short: "List the releases and masters of an artist",
		Args:  cobra.ExactArgs(1),
	}
	pages := cli.BindPageFlags(releases.Flags())
	releases.RunE = listRun(cfg, pages, func(ctx context.Context, args []string, page api.Page) (*api.Paginated[api.ArtistRelease], error) {
		id, err := parseID("artist", args[0])
		if err != nil {
			return nil, err
		}
		return cfg.client.ArtistReleases(ctx, id, page)
	})
	cmd.AddCommand(releases)

	return cmd
}
