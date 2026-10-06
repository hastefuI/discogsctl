package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

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

	var rating int
	rate := &cobra.Command{
		Use:   "rate <id>",
		Short: "Rate a release",
		Long: `Give a release your rating, from 1 to 5, replacing any you gave before. Your
wantlist shows this rating for the release. This needs a token.`,
		Example: "  discogsctl release rate 182213 --rating 5",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("release", args[0])
			if err != nil {
				return err
			}
			if rating < 1 || rating > 5 {
				return fmt.Errorf("--rating must be between 1 and 5, got %d", rating)
			}
			user, err := cfg.tokenHolder(cmd.Context())
			if err != nil {
				return err
			}
			r, err := cfg.client.RateRelease(cmd.Context(), id, user, rating)
			if err != nil {
				return err
			}
			return cfg.print(cmd, r)
		},
	}
	rate.Flags().IntVar(&rating, "rating", 0, "your rating, from 1 to 5")
	rate.MarkFlagRequired("rating")
	cmd.AddCommand(rate)

	cmd.AddCommand(&cobra.Command{
		Use:     "unrate <id>",
		Short:   "Remove your rating of a release",
		Long:    "Remove the rating you gave a release. Removing one that is not there succeeds.\nThis needs a token.",
		Example: "  discogsctl release unrate 182213",
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
			if err := cfg.client.UnrateRelease(cmd.Context(), id, user); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Removed the rating of release %d by %s\n", id, user)
			return nil
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
