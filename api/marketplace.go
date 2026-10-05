package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Price is an amount in a currency, such as 2.09 USD.
type Price struct {
	Value    float64 `json:"value"`
	Currency string  `json:"currency"`
}

// MarketplaceStats is the current state of a release in the marketplace.
// LowestPrice and NumForSale are nil when no copy is for sale, and when the
// release is blocked from sale.
type MarketplaceStats struct {
	LowestPrice     *Price `json:"lowest_price"`
	NumForSale      *int   `json:"num_for_sale"`
	BlockedFromSale bool   `json:"blocked_from_sale"`

	raw json.RawMessage
}

type marketplaceStats MarketplaceStats

func (s *MarketplaceStats) UnmarshalJSON(b []byte) error {
	return decodeKeep(b, (*marketplaceStats)(s), &s.raw)
}
func (s MarketplaceStats) MarshalJSON() ([]byte, error) {
	return encodeKept(s.raw, marketplaceStats(s))
}

// MarketplaceStats returns the marketplace state of the release with id, with
// the lowest price in the client's currency. Without a token or a currency,
// Discogs prices it in US dollars.
func (c *Client) MarketplaceStats(ctx context.Context, id int) (*MarketplaceStats, error) {
	if id < 1 {
		return nil, fmt.Errorf("api: release id %d must be 1 or more", id)
	}
	u, err := c.endpoint("marketplace", "stats", strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	if c.currency != "" {
		u.RawQuery = url.Values{"curr_abbr": {c.currency}}.Encode()
	}
	var s MarketplaceStats
	if err := c.get(ctx, u, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// OrderStatuses are the values Discogs accepts for OrderQuery.Status.
var OrderStatuses = []string{
	"All", "New Order", "Buyer Contacted", "Invoice Sent", "Payment Pending",
	"Payment Received", "In Progress", "Shipped", "Merged", "Order Changed",
	"Refund Sent", "Cancelled", "Cancelled (Non-Paying Buyer)",
	"Cancelled (Item Unavailable)", "Cancelled (Per Buyer's Request)",
	"Cancelled (Refund Received)",
}

// OrderSorts are the values Discogs accepts for OrderQuery.Sort.
var OrderSorts = []string{"id", "buyer", "created", "status", "last_activity"}

// OrderQuery filters a listing of orders. Every field is optional, and a zero
// field is left out of the request. Archived nil lists archived and
// unarchived orders alike. SortOrder is "asc" or "desc".
type OrderQuery struct {
	Status        string
	CreatedAfter  time.Time
	CreatedBefore time.Time
	Archived      *bool
	Sort          string
	SortOrder     string

	Page
}

func (q OrderQuery) values() (url.Values, error) {
	if q.Status != "" && !slices.Contains(OrderStatuses, q.Status) {
		return nil, fmt.Errorf("api: order status %q must be one of %s", q.Status, strings.Join(OrderStatuses, ", "))
	}
	if q.Sort != "" && !slices.Contains(OrderSorts, q.Sort) {
		return nil, fmt.Errorf("api: order sort %q must be one of %s", q.Sort, strings.Join(OrderSorts, ", "))
	}
	if q.SortOrder != "" && q.SortOrder != "asc" && q.SortOrder != "desc" {
		return nil, fmt.Errorf("api: sort order %q must be asc or desc", q.SortOrder)
	}
	v := url.Values{}
	for name, value := range map[string]string{"status": q.Status, "sort": q.Sort, "sort_order": q.SortOrder} {
		if value != "" {
			v.Set(name, value)
		}
	}
	if !q.CreatedAfter.IsZero() {
		v.Set("created_after", q.CreatedAfter.UTC().Format(time.RFC3339))
	}
	if !q.CreatedBefore.IsZero() {
		v.Set("created_before", q.CreatedBefore.UTC().Format(time.RFC3339))
	}
	if q.Archived != nil {
		v.Set("archived", strconv.FormatBool(*q.Archived))
	}
	if err := q.Page.apply(v); err != nil {
		return nil, err
	}
	return v, nil
}

// UserRef names a user on an order.
type UserRef struct {
	ID          int    `json:"id"`
	Username    string `json:"username"`
	ResourceURL string `json:"resource_url"`
}

// OrderItem is one listing in an order.
type OrderItem struct {
	ID      int `json:"id"`
	Release struct {
		ID          int    `json:"id"`
		Description string `json:"description"`
		ResourceURL string `json:"resource_url"`
	} `json:"release"`
	Price Price `json:"price"`
}

// Shipping is the shipping charged on an order.
type Shipping struct {
	Value    float64 `json:"value"`
	Currency string  `json:"currency"`
	Method   string  `json:"method"`
}

// Order is a marketplace order. ID is a string such as "1-1": the seller's ID
// and the order's number. ShippingAddress is the buyer's address as they
// entered it, and is personal data.
type Order struct {
	ID                     string      `json:"id"`
	Status                 string      `json:"status"`
	Created                string      `json:"created"`
	LastActivity           string      `json:"last_activity"`
	Archived               bool        `json:"archived"`
	Buyer                  UserRef     `json:"buyer"`
	Seller                 UserRef     `json:"seller"`
	Items                  []OrderItem `json:"items"`
	Total                  Price       `json:"total"`
	Fee                    Price       `json:"fee"`
	Shipping               Shipping    `json:"shipping"`
	ShippingAddress        string      `json:"shipping_address"`
	AdditionalInstructions string      `json:"additional_instructions"`
	NextStatus             []string    `json:"next_status"`
	URI                    string      `json:"uri"`
	ResourceURL            string      `json:"resource_url"`
	MessagesURL            string      `json:"messages_url"`

	raw json.RawMessage
}

type order Order

func (o *Order) UnmarshalJSON(b []byte) error { return decodeKeep(b, (*order)(o), &o.raw) }
func (o Order) MarshalJSON() ([]byte, error)  { return encodeKept(o.raw, order(o)) }

// Orders returns one page of the authenticated user's marketplace orders. It
// needs a token; without one Discogs answers 401.
func (c *Client) Orders(ctx context.Context, q OrderQuery) (*Paginated[Order], error) {
	u, err := c.endpoint("marketplace", "orders")
	if err != nil {
		return nil, err
	}
	v, err := q.values()
	if err != nil {
		return nil, err
	}
	u.RawQuery = v.Encode()
	return getPage[Order](ctx, c, u, "orders")
}
