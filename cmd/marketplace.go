package cmd

import "github.com/spf13/cobra"

func newMarketplaceCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "marketplace",
		Short: "Read marketplace data",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "stats <release_id>",
		Short: "Get the marketplace stats of a release",
		Long: `Get how many copies of a release are for sale, the lowest price, and whether
the release is blocked from sale. The price is in --currency.

A release with no copies for sale, or one blocked from sale, has no count
or price.`,
		Example: "  discogsctl marketplace stats 249504\n  discogsctl marketplace stats 249504 --currency GBP --output json | jq '.lowest_price.value'",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("release", args[0])
			if err != nil {
				return err
			}
			s, err := cfg.client.MarketplaceStats(cmd.Context(), id)
			if err != nil {
				return err
			}
			return cfg.print(cmd, s)
		},
	})

	return cmd
}
