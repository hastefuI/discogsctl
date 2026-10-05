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

// OrderItem is one listing in an order. The conditions are sent for a single
// order, and may be empty in a listing of orders.
type OrderItem struct {
	ID      int `json:"id"`
	Release struct {
		ID          int    `json:"id"`
		Description string `json:"description"`
		ResourceURL string `json:"resource_url"`
	} `json:"release"`
	Price           Price  `json:"price"`
	MediaCondition  string `json:"media_condition"`
	SleeveCondition string `json:"sleeve_condition"`
}

// Tracking is the shipment tracking on an order.
type Tracking struct {
	Number  string `json:"number"`
	Carrier string `json:"carrier"`
	URL     string `json:"url"`
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
	Tracking               *Tracking   `json:"tracking"`
	NextStatus             []string    `json:"next_status"`
	URI                    string      `json:"uri"`
	ResourceURL            string      `json:"resource_url"`
	MessagesURL            string      `json:"messages_url"`

	raw json.RawMessage
}

type order Order

func (o *Order) UnmarshalJSON(b []byte) error { return decodeKeep(b, (*order)(o), &o.raw) }
func (o Order) MarshalJSON() ([]byte, error)  { return encodeKept(o.raw, order(o)) }

// Order returns the marketplace order with id, such as "1-1". It needs a
// token for the order's seller.
func (c *Client) Order(ctx context.Context, id string) (*Order, error) {
	u, err := c.endpoint("marketplace", "orders", strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	var o Order
	if err := c.get(ctx, u, &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// Orders returns one page of the authenticated user's marketplace orders, the
// ones they sold: Discogs has no listing of purchases. It needs a token;
// without one Discogs answers 401.
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

// Refund is the refund an order message records.
type Refund struct {
	Amount float64 `json:"amount"`
}

// OrderMessage is one entry in an order's history. Type is "message" for a
// message between buyer and seller, which has From; "status" for a status
// change, which has Actor and StatusID; "refund_sent" or "refund_received",
// which have Refund; or another type. Discogs also sends "payment" and
// "tracking", which it does not document; both have Actor. Message is the
// text, and for anything but a message it is Discogs' own description of the
// event.
type OrderMessage struct {
	Type      string   `json:"type"`
	Timestamp string   `json:"timestamp"`
	Subject   string   `json:"subject"`
	Message   string   `json:"message"`
	From      *UserRef `json:"from"`
	Actor     *UserRef `json:"actor"`
	StatusID  int      `json:"status_id"`
	Refund    *Refund  `json:"refund"`
	Order     struct {
		ID          string `json:"id"`
		ResourceURL string `json:"resource_url"`
	} `json:"order"`

	raw json.RawMessage
}

type orderMessage OrderMessage

func (m *OrderMessage) UnmarshalJSON(b []byte) error {
	return decodeKeep(b, (*orderMessage)(m), &m.raw)
}
func (m OrderMessage) MarshalJSON() ([]byte, error) { return encodeKept(m.raw, orderMessage(m)) }

// OrderMessages returns one page of the history of the order with id, most
// recent first. It needs a token for the order's seller.
func (c *Client) OrderMessages(ctx context.Context, id string, page Page) (*Paginated[OrderMessage], error) {
	u, err := c.endpoint("marketplace", "orders", strings.TrimSpace(id), "messages")
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	if err := page.apply(q); err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()
	return getPage[OrderMessage](ctx, c, u, "messages")
}

// ListingStatuses are the values Discogs accepts for InventoryQuery.Status.
// Anyone but the inventory's owner sees only "For Sale" listings, and Discogs
// ignores the status filter for them rather than refusing it.
var ListingStatuses = []string{"All", "For Sale", "Draft", "Expired", "Sold", "Deleted", "Suspended", "Violation"}

// InventorySorts are the values Discogs accepts for InventoryQuery.Sort.
// "item" is the release title. "status" and "location" need a token for the
// inventory's owner.
var InventorySorts = []string{"listed", "price", "item", "artist", "label", "catno", "audio", "status", "location"}

// InventoryQuery filters a seller's inventory. Every field is optional, and an
// empty field is left out of the request. SortOrder is "asc" or "desc".
type InventoryQuery struct {
	Status    string
	Sort      string
	SortOrder string

	Page
}

func (q InventoryQuery) values() (url.Values, error) {
	if q.Status != "" && !slices.Contains(ListingStatuses, q.Status) {
		return nil, fmt.Errorf("api: listing status %q must be one of %s", q.Status, strings.Join(ListingStatuses, ", "))
	}
	if q.Sort != "" && !slices.Contains(InventorySorts, q.Sort) {
		return nil, fmt.Errorf("api: inventory sort %q must be one of %s", q.Sort, strings.Join(InventorySorts, ", "))
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
	if err := q.Page.apply(v); err != nil {
		return nil, err
	}
	return v, nil
}

// ListingRelease is the release a listing sells, as a listing describes it.
type ListingRelease struct {
	ID            int    `json:"id"`
	Description   string `json:"description"`
	Artist        string `json:"artist"`
	Title         string `json:"title"`
	Format        string `json:"format"`
	CatalogNumber string `json:"catalog_number"`
	Year          int    `json:"year"`
	ResourceURL   string `json:"resource_url"`
}

// Listing is one item in a seller's inventory. Price is in the seller's
// currency. Location and ExternalID are sent only to the inventory's owner,
// as are weight and quantity, which the JSON keeps but this type does not
// name.
type Listing struct {
	ID              int            `json:"id"`
	Status          string         `json:"status"`
	Price           Price          `json:"price"`
	AllowOffers     bool           `json:"allow_offers"`
	Condition       string         `json:"condition"`
	SleeveCondition string         `json:"sleeve_condition"`
	Posted          string         `json:"posted"`
	ShipsFrom       string         `json:"ships_from"`
	Comments        string         `json:"comments"`
	Audio           bool           `json:"audio"`
	Seller          UserRef        `json:"seller"`
	Release         ListingRelease `json:"release"`
	Location        string         `json:"location"`
	ExternalID      string         `json:"external_id"`
	URI             string         `json:"uri"`
	ResourceURL     string         `json:"resource_url"`

	raw json.RawMessage
}

type listing Listing

func (l *Listing) UnmarshalJSON(b []byte) error { return decodeKeep(b, (*listing)(l), &l.raw) }
func (l Listing) MarshalJSON() ([]byte, error)  { return encodeKept(l.raw, listing(l)) }

// Inventory returns one page of the listings in the inventory of username.
// Without a token for its owner, only listings for sale are visible. Discogs
// ignores a currency here, so prices are in the seller's own.
func (c *Client) Inventory(ctx context.Context, username string, q InventoryQuery) (*Paginated[Listing], error) {
	u, err := c.endpoint("users", username, "inventory")
	if err != nil {
		return nil, err
	}
	v, err := q.values()
	if err != nil {
		return nil, err
	}
	u.RawQuery = v.Encode()
	return getPage[Listing](ctx, c, u, "listings")
}
