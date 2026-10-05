package cmd

import (
	"context"
	"errors"
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

	cmd.AddCommand(&cobra.Command{
		Use:   "order <order_id>",
		Short: "Get one of your marketplace orders",
		Long: `Get one of your marketplace orders as a seller, with its items, shipping, fee
and tracking. This needs a token for the order's seller. Order IDs, such as
1234567-89, are in the first column of marketplace orders.

The text view leaves out the shipping address. --output json includes it,
and it is the buyer's personal data.`,
		Example: "  discogsctl marketplace order 1234567-89\n  discogsctl marketplace order 1234567-89 --output json | jq '.items[].release.description'",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(args[0]) == "" {
				return errors.New("order ID is empty")
			}
			o, err := cfg.client.Order(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cfg.print(cmd, o)
		},
	})

	messages := &cobra.Command{
		Use:   "messages <order_id>",
		Short: "List the messages and history of one of your orders",
		Long: `List the history of one of your orders as a seller, most recent first:
messages between you and the buyer, status changes and refunds. This needs
a token for the order's seller.

Messages are your conversation with the buyer, and can hold their personal
details.`,
		Example: "  discogsctl marketplace messages 1234567-89\n  discogsctl marketplace messages 1234567-89 --all --output json | jq -r '.[] | select(.type == \"message\") | .message'",
		Args:    cobra.ExactArgs(1),
	}
	pages := cli.BindPageFlags(messages.Flags())
	messages.RunE = listRun(cfg, pages, func(ctx context.Context, args []string, page api.Page) (*api.Paginated[api.OrderMessage], error) {
		if strings.TrimSpace(args[0]) == "" {
			return nil, errors.New("order ID is empty")
		}
		return cfg.client.OrderMessages(ctx, args[0], page)
	})
	cmd.AddCommand(messages)
	cmd.AddCommand(newInventoryCmd(cfg))
	return cmd
}

func newInventoryCmd(cfg *config) *cobra.Command {
	var (
		q                       api.InventoryQuery
		status, sort, sortOrder string
	)
	cmd := &cobra.Command{
		Use:   "inventory [username]",
		Short: "List a seller's inventory",
		Long: `List the listings in a seller's inventory. Without a username, the inventory
is the token holder's. Anyone but the owner sees only listings for sale, and
Discogs ignores --status for them rather than refusing it. The owner can
filter by --status and also sort by status or location.

--status takes one of: ` + strings.Join(api.ListingStatuses, ", ") + `.
Case does not matter. Prices are in the seller's currency: Discogs ignores
--currency here.`,
		Example: `  discogsctl marketplace inventory <username> --sort price --sort-order desc
  discogsctl marketplace inventory <username> --all --output json | jq -r '.[] | "\(.price.value) \(.release.description)"'
  discogsctl marketplace inventory --status draft`,
		Args: cobra.MaximumNArgs(1),
	}

	f := cmd.Flags()
	f.StringVar(&status, "status", "", `only listings with this status, such as "For Sale"`)
	f.StringVar(&sort, "sort", "", "sort by: "+strings.Join(api.InventorySorts, ", "))
	f.StringVar(&sortOrder, "sort-order", "", "asc or desc")
	pages := cli.BindPageFlags(f)

	cmd.RunE = listRun(cfg, pages, func(ctx context.Context, args []string, page api.Page) (*api.Paginated[api.Listing], error) {
		// Every flag is checked before the username is resolved, which can
		// cost a request.
		var err error
		if q.Status, err = oneOf("--status", status, api.ListingStatuses); err != nil {
			return nil, err
		}
		if q.Sort, err = oneOf("--sort", sort, api.InventorySorts); err != nil {
			return nil, err
		}
		if q.SortOrder, err = oneOf("--sort-order", sortOrder, []string{"asc", "desc"}); err != nil {
			return nil, err
		}
		user, err := cfg.username(ctx, strings.Join(args, ""))
		if err != nil {
			return nil, err
		}
		q.Page = page
		return cfg.client.Inventory(ctx, user, q)
	})
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
		Short: "List your marketplace orders as a seller",
		Long: `List the marketplace orders of the token holder as a seller. This needs a
token. Discogs has no listing of the orders you bought.

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
		if q.Status, err = oneOf("--status", status, api.OrderStatuses); err != nil {
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

// oneOf returns the value in options that matches s, ignoring case, so a
// flag can take "payment received" for Discogs' "Payment Received".
func oneOf(flag, s string, options []string) (string, error) {
	if s == "" {
		return "", nil
	}
	for _, o := range options {
		if strings.EqualFold(o, strings.TrimSpace(s)) {
			return o, nil
		}
	}
	return "", fmt.Errorf("invalid %s %q, want one of %s", flag, s, strings.Join(options, ", "))
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
