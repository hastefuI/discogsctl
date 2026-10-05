package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.hasteful.org/discogsctl/api"
	"go.hasteful.org/discogsctl/internal/cli"
)

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

	cmd.AddCommand(newOrdersCmd(cfg))
	return cmd
}

func newOrdersCmd(cfg *config) *cobra.Command {
	var (
		q                     api.OrderQuery
		status, after, before string
		archived              bool
	)
	cmd := &cobra.Command{
		Use:   "orders",
		Short: "List your marketplace orders",
		Long: `List the marketplace orders of the token holder. This needs a token.

--status takes one of: ` + strings.Join(api.OrderStatuses, ", ") + `.
Case does not matter. --created-after and --created-before take a date, such
as 2026-09-01, which is midnight UTC, or a time such as 2026-09-01T12:00:00Z.

The table leaves out shipping addresses. --output json includes them, and
they are buyers' personal data.`,
		Example: `  discogsctl marketplace orders --status "payment received"
  discogsctl marketplace orders --created-after 2026-09-01 --all --output json | jq -r '.[] | "\(.id) \(.total.value)"'`,
		Args: cobra.NoArgs,
	}

	f := cmd.Flags()
	f.StringVar(&status, "status", "", `only orders with this status, such as "Payment Received"`)
	f.StringVar(&after, "created-after", "", "only orders created after this date or time")
	f.StringVar(&before, "created-before", "", "only orders created before this date or time")
	f.BoolVar(&archived, "archived", false, "only archived orders, or with =false only unarchived ones (default both)")
	f.StringVar(&q.Sort, "sort", "", "sort by: "+strings.Join(api.OrderSorts, ", "))
	f.StringVar(&q.SortOrder, "sort-order", "", "asc or desc")
	pages := cli.BindPageFlags(f)

	cmd.RunE = listRun(cfg, pages, func(ctx context.Context, args []string, page api.Page) (*api.Paginated[api.Order], error) {
		var err error
		if q.Status, err = orderStatus(status); err != nil {
			return nil, err
		}
		if q.CreatedAfter, err = parseTime("--created-after", after); err != nil {
			return nil, err
		}
		if q.CreatedBefore, err = parseTime("--created-before", before); err != nil {
			return nil, err
		}
		q.Archived = nil
		if cmd.Flags().Changed("archived") {
			q.Archived = &archived
		}
		q.Page = page
		return cfg.client.Orders(ctx, q)
	})
	return cmd
}

// orderStatus returns the status Discogs spells the same as s, ignoring case.
func orderStatus(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	for _, status := range api.OrderStatuses {
		if strings.EqualFold(status, strings.TrimSpace(s)) {
			return status, nil
		}
	}
	return "", fmt.Errorf("invalid --status %q, want one of %s", s, strings.Join(api.OrderStatuses, ", "))
}

// parseTime reads a date such as 2026-09-01, taken as midnight UTC, or an
// RFC 3339 time. Empty gives the zero time.
func parseTime(flag, s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid %s %q, want a date such as 2026-09-01 or a time such as 2026-09-01T12:00:00Z", flag, s)
}
