package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMarketplaceStats(t *testing.T) {
	var uri string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uri = r.URL.RequestURI()
		w.Write([]byte(`{"num_for_sale": 119, "lowest_price": {"value": 0.59, "currency": "EUR"}, "blocked_from_sale": false}`))
	}))
	defer srv.Close()

	c, err := New(Options{BaseURL: srv.URL, UserAgent: testAgent, Currency: "eur", HTTP: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.MarketplaceStats(t.Context(), 249504)
	if err != nil {
		t.Fatal(err)
	}
	if uri != "/marketplace/stats/249504?curr_abbr=EUR" {
		t.Errorf("request = %q, want /marketplace/stats/249504?curr_abbr=EUR", uri)
	}
	if s.NumForSale == nil || *s.NumForSale != 119 || s.LowestPrice == nil || *s.LowestPrice != (Price{0.59, "EUR"}) || s.BlockedFromSale {
		t.Errorf("stats = %+v, want 119 for sale from 0.59 EUR", s)
	}
}

func TestMarketplaceStatsNothingForSale(t *testing.T) {
	body := `{"num_for_sale":null,"lowest_price":null,"blocked_from_sale":true}`
	var s MarketplaceStats
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatal(err)
	}
	if s.NumForSale != nil || s.LowestPrice != nil || !s.BlockedFromSale {
		t.Errorf("stats = %+v, want nil count and price, blocked", s)
	}
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != body {
		t.Errorf("re-encoded = %s, want the body as sent", out)
	}
}

func TestMarketplaceStatsRejectsBadID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL)
	}))
	defer srv.Close()
	if _, err := newTestClient(t, srv, "").MarketplaceStats(t.Context(), 0); err == nil {
		t.Error("MarketplaceStats(0) succeeded, want error")
	}
}

func TestOrderQueryValues(t *testing.T) {
	archived := false
	q := OrderQuery{
		Status:       "Payment Received",
		CreatedAfter: time.Date(2026, 9, 1, 0, 0, 0, 0, time.FixedZone("PDT", -7*3600)),
		Archived:     &archived,
		Sort:         "created",
		SortOrder:    "desc",
		Page:         Page{Page: 2, PerPage: 100},
	}
	v, err := q.values()
	if err != nil {
		t.Fatal(err)
	}
	want := "archived=false&created_after=2026-09-01T07%3A00%3A00Z&page=2&per_page=100&sort=created&sort_order=desc&status=Payment+Received"
	if got := v.Encode(); got != want {
		t.Errorf("query = %s\nwant    %s", got, want)
	}
	if v, _ := (OrderQuery{}).values(); len(v) != 0 {
		t.Errorf("empty query = %v, want no parameters", v)
	}

	for _, bad := range []OrderQuery{{Status: "payment received"}, {Sort: "price"}, {SortOrder: "up"}} {
		if _, err := bad.values(); err == nil {
			t.Errorf("values(%+v) succeeded, want error", bad)
		}
	}
}

func TestOrders(t *testing.T) {
	var uri, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uri, auth = r.URL.RequestURI(), r.Header.Get("Authorization")
		w.Write([]byte(`{"pagination": {"page": 1, "pages": 1, "items": 1, "urls": {}}, "orders": [
			{"id": "1-1", "status": "New Order", "created": "2011-10-21T09:25:17-07:00",
			 "buyer": {"id": 2, "username": "example_buyer"}, "total": {"currency": "USD", "value": 42.0},
			 "items": [{"id": 41578242, "release": {"id": 1, "description": "Persuader, The - Stockholm"}, "price": {"currency": "USD", "value": 42.0}}],
			 "shipping_address": "Asdf Exampleton", "extra": true}]}`))
	}))
	defer srv.Close()

	page, err := newTestClient(t, srv, "test-token").Orders(t.Context(), OrderQuery{Status: "New Order"})
	if err != nil {
		t.Fatal(err)
	}
	if uri != "/marketplace/orders?status=New+Order" || auth != "Discogs token=test-token" {
		t.Errorf("request = %q with Authorization %q", uri, auth)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %+v, want one order", page.Items)
	}
	o := page.Items[0]
	if o.ID != "1-1" || o.Buyer.Username != "example_buyer" || o.Total != (Price{42, "USD"}) || len(o.Items) != 1 || o.Items[0].Release.ID != 1 {
		t.Errorf("order = %+v", o)
	}
	if b, _ := json.Marshal(o); !bytes.Contains(b, []byte(`"extra":true`)) {
		t.Errorf("re-encoded order lost fields the type does not name: %s", b)
	}
}

func TestOrder(t *testing.T) {
	var uri string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uri = r.URL.EscapedPath()
		w.Write([]byte(`{"id": "1-1", "status": "Shipped",
			"items": [{"id": 41578242, "release": {"id": 1, "description": "Persuader, The - Stockholm (2x12\")"},
			  "price": {"currency": "USD", "value": 42}, "media_condition": "Mint (M)", "sleeve_condition": "Very Good Plus (VG+)"}],
			"tracking": {"number": "1Z999", "carrier": "UPS", "url": "https://www.ups.com/track?tracknum=1Z999"}}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv, "test-token")

	o, err := c.Order(t.Context(), "1-1")
	if err != nil {
		t.Fatal(err)
	}
	if uri != "/marketplace/orders/1-1" {
		t.Errorf("path = %q, want /marketplace/orders/1-1", uri)
	}
	if len(o.Items) != 1 || o.Items[0].MediaCondition != "Mint (M)" || o.Items[0].SleeveCondition != "Very Good Plus (VG+)" {
		t.Errorf("items = %+v", o.Items)
	}
	if o.Tracking == nil || *o.Tracking != (Tracking{"1Z999", "UPS", "https://www.ups.com/track?tracknum=1Z999"}) {
		t.Errorf("tracking = %+v", o.Tracking)
	}

	if _, err := c.Order(t.Context(), "1-1/messages"); err != nil {
		t.Fatal(err)
	}
	if uri != "/marketplace/orders/1-1%2Fmessages" {
		t.Errorf("path = %q, want the slash escaped so an ID cannot reach another route", uri)
	}
	for _, id := range []string{"", " ", ".."} {
		if _, err := c.Order(t.Context(), id); err == nil {
			t.Errorf("Order(%q) succeeded, want error", id)
		}
	}
}

func TestOrderMessages(t *testing.T) {
	var uri string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uri = r.URL.RequestURI()
		w.Write([]byte(`{"pagination": {"page": 1, "pages": 1, "urls": {}}, "messages": [
			{"type": "refund_sent", "timestamp": "2015-06-02T13:17:44-07:00", "message": "example_seller sent refund of $5.00.",
			 "refund": {"amount": 5, "order": {"id": "845236-9"}}, "order": {"id": "845236-9"}},
			{"type": "message", "timestamp": "2015-06-02T13:17:07-07:00", "message": "Thank you for your order!",
			 "from": {"id": 1001, "username": "example_seller"}, "order": {"id": "845236-9"}},
			{"type": "status", "status_id": 6, "timestamp": "2015-06-02T13:16:57-07:00",
			 "actor": {"username": "example_seller"}, "message": "example_buyer changed the order status to Shipped."}]}`))
	}))
	defer srv.Close()

	page, err := newTestClient(t, srv, "test-token").OrderMessages(t.Context(), "845236-9", Page{Page: 1, PerPage: 100})
	if err != nil {
		t.Fatal(err)
	}
	if uri != "/marketplace/orders/845236-9/messages?page=1&per_page=100" {
		t.Errorf("request = %q", uri)
	}
	if len(page.Items) != 3 {
		t.Fatalf("messages = %+v, want 3", page.Items)
	}
	refund, msg, status := page.Items[0], page.Items[1], page.Items[2]
	if refund.Refund == nil || refund.Refund.Amount != 5 || refund.Order.ID != "845236-9" {
		t.Errorf("refund = %+v", refund)
	}
	if msg.From == nil || msg.From.Username != "example_seller" || msg.Actor != nil {
		t.Errorf("message = %+v", msg)
	}
	if status.Actor == nil || status.StatusID != 6 || status.From != nil {
		t.Errorf("status = %+v", status)
	}
}

func TestInventory(t *testing.T) {
	var uri string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uri = r.URL.RequestURI()
		w.Write([]byte(`{"pagination": {"page": 1, "pages": 1, "urls": {}}, "listings": [
			{"id": 150899904, "status": "For Sale", "price": {"currency": "USD", "value": 149.99},
			 "condition": "Near Mint (NM or M-)", "sleeve_condition": "Very Good Plus (VG+)", "posted": "2014-07-01T10:20:17-07:00",
			 "seller": {"username": "rappcats"}, "original_price": {"curr_abbr": "USD", "value": 149.99},
			 "release": {"id": 2992668, "description": "Danger Mouse & Daniele Luppi - Rome", "catalog_number": "TMR092", "year": 2011}}]}`))
	}))
	defer srv.Close()

	page, err := newTestClient(t, srv, "").Inventory(t.Context(), "rappcats", InventoryQuery{Status: "For Sale", Sort: "price", SortOrder: "desc", Page: Page{PerPage: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if uri != "/users/rappcats/inventory?per_page=100&sort=price&sort_order=desc&status=For+Sale" {
		t.Errorf("request = %q", uri)
	}
	l := page.Items[0]
	if l.ID != 150899904 || l.Price != (Price{149.99, "USD"}) || l.Release.ID != 2992668 || l.Release.CatalogNumber != "TMR092" || l.Seller.Username != "rappcats" {
		t.Errorf("listing = %+v", l)
	}
	if b, _ := json.Marshal(l); !bytes.Contains(b, []byte(`"original_price"`)) {
		t.Errorf("re-encoded listing lost fields the type does not name: %s", b)
	}

	for _, bad := range []InventoryQuery{{Status: "for sale"}, {Sort: "condition"}, {SortOrder: "down"}} {
		if _, err := bad.values(); err == nil {
			t.Errorf("values(%+v) succeeded, want error", bad)
		}
	}
}

func TestPriceSuggestions(t *testing.T) {
	body := `{"Mint (M)":{"currency":"USD","value":546.25},"Poor (P)":{"currency":"USD","value":28.75}}`
	var uri string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uri = r.URL.RequestURI()
		if r.URL.Path == "/marketplace/price_suggestions/2" {
			w.Write([]byte(`{}`))
			return
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()
	c := newTestClient(t, srv, "test-token")

	p, err := c.PriceSuggestions(t.Context(), 182213)
	if err != nil {
		t.Fatal(err)
	}
	if uri != "/marketplace/price_suggestions/182213" {
		t.Errorf("request = %q", uri)
	}
	if len(p.Prices) != 2 || p.Prices["Mint (M)"] != (Price{546.25, "USD"}) {
		t.Errorf("prices = %+v", p.Prices)
	}
	if out, _ := json.Marshal(p); string(out) != body {
		t.Errorf("re-encoded = %s\nwant the body as sent: %s", out, body)
	}

	empty, err := c.PriceSuggestions(t.Context(), 2)
	if err != nil || len(empty.Prices) != 0 {
		t.Errorf("no suggestions = %+v, %v; want an empty map", empty, err)
	}
	if _, err := c.PriceSuggestions(t.Context(), 0); err == nil {
		t.Error("PriceSuggestions(0) succeeded, want error")
	}
}

func TestListing(t *testing.T) {
	var uri string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uri = r.URL.RequestURI()
		w.Write([]byte(`{"id": 3000001, "status": "For Sale", "price": {"value": 7.565, "currency": "EUR"},
			"allow_offers": true, "condition": "Very Good (VG)", "release": {"id": 11180538, "description": "Vallanzaska - Cheope"},
			"shipping_price": {}}`))
	}))
	defer srv.Close()

	c, err := New(Options{BaseURL: srv.URL, UserAgent: testAgent, Currency: "eur", HTTP: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	l, err := c.Listing(t.Context(), 3000001)
	if err != nil {
		t.Fatal(err)
	}
	if uri != "/marketplace/listings/3000001?curr_abbr=EUR" {
		t.Errorf("request = %q, want the client's currency sent", uri)
	}
	if l.Price != (Price{7.565, "EUR"}) || !l.AllowOffers || l.Release.ID != 11180538 {
		t.Errorf("listing = %+v", l)
	}

	uri = ""
	if _, err := newTestClient(t, srv, "").Listing(t.Context(), 0); err == nil || uri != "" {
		t.Errorf("Listing(0) = %v after request %q, want an error and none", err, uri)
	}
	if _, err := newTestClient(t, srv, "").Listing(t.Context(), 5); err != nil || uri != "/marketplace/listings/5" {
		t.Errorf("without a currency, request = %q, %v", uri, err)
	}
}
