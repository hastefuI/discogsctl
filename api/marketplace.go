package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
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
