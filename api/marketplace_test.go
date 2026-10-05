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
