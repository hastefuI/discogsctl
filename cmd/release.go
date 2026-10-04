package cmd

import "github.com/spf13/cobra"

func newReleaseCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release",
		Short: "Read releases",
	}

	cmd.AddCommand(&cobra.Command{
		Use:     "get <id>",
		Short:   "Get a release",
		Long:    "Get a release. Marketplace prices are in --currency.",
		Example: "  discogsctl release get 249504\n  discogsctl release get 249504 --output json | jq '.tracklist[].title'",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("release", args[0])
			if err != nil {
				return err
			}
			r, err := cfg.client.Release(cmd.Context(), id)
			if err != nil {
				return err
			}
			return cfg.print(cmd, r)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "rating <id>",
		Short: "Get the community rating of a release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("release", args[0])
			if err != nil {
				return err
			}
			r, err := cfg.client.ReleaseRating(cmd.Context(), id)
			if err != nil {
				return err
			}
			return cfg.print(cmd, r)
		},
	})

	return cmd
}
