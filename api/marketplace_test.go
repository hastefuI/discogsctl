package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
