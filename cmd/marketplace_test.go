package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMarketplaceOrders(t *testing.T) {
	t.Setenv(envToken, "test-token")
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Discogs token=test-token" {
			t.Errorf("%s sent without the token", r.URL)
		}
		seen = append(seen, r.URL.RequestURI())
		if r.URL.Query().Get("page") == "1" {
			fmt.Fprintf(w, `{"pagination": {"page": 1, "pages": 2, "urls": {"next": "http://%s/marketplace/orders?page=2&per_page=100&status=Payment+Received"}},
				"orders": [{"id": "1-1", "status": "Payment Received", "created": "2026-09-02T10:00:00-07:00",
				"buyer": {"username": "buyer_one"}, "items": [{"id": 1}, {"id": 2}], "total": {"currency": "USD", "value": 42.5},
				"shipping_address": "1 Private Lane"}]}`, r.Host)
			return
		}
		fmt.Fprint(w, `{"pagination": {"page": 2, "pages": 2, "urls": {}}, "orders": [{"id": "1-2", "status": "Payment Received",
			"created": "2026-09-03T10:00:00-07:00", "buyer": {"username": "buyer_two"}, "items": [{"id": 3}],
			"total": {"currency": "USD", "value": 9}}]}`)
	}))
	defer srv.Close()

	stdout, _, err := run(t, srv, "marketplace", "orders", "--status", "payment received", "--created-after", "2026-09-01",
		"--archived=false", "--sort", "created", "--sort-order", "asc", "--all")
	if err != nil {
		t.Fatal(err)
	}
	if want := "/marketplace/orders?archived=false&created_after=2026-09-01T00%3A00%3A00Z&page=1&per_page=100&sort=created&sort_order=asc&status=Payment+Received"; seen[0] != want {
		t.Errorf("first request = %s\nwant            %s", seen[0], want)
	}
	if len(seen) != 2 {
		t.Errorf("requests = %q, want two pages", seen)
	}
	want := "ID   CREATED     STATUS            BUYER      ITEMS  TOTAL\n" +
		"1-1  2026-09-02  Payment Received  buyer_one  2      42.50 USD\n" +
		"1-2  2026-09-03  Payment Received  buyer_two  1      9.00 USD\n"
	if stdout != want {
		t.Errorf("stdout\n%q\nwant\n%q", stdout, want)
	}
	if strings.Contains(stdout, "Private Lane") {
		t.Error("the table printed a shipping address")
	}
}

func TestMarketplaceOrdersFlagErrors(t *testing.T) {
	t.Setenv(envToken, "test-token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL)
	}))
	defer srv.Close()

	for _, args := range [][]string{
		{"--status", "paid"},
		{"--created-after", "September"},
		{"--created-before", "2026-13-01"},
		{"--sort", "price"},
		{"--sort-order", "up"},
	} {
		if _, _, err := run(t, srv, append([]string{"marketplace", "orders"}, args...)...); err == nil {
			t.Errorf("%q succeeded, want error", args)
		}
	}
}

func TestMarketplaceOrderEmptyID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL)
	}))
	defer srv.Close()
	if _, _, err := run(t, srv, "marketplace", "order", " "); err == nil {
		t.Error("an empty order ID succeeded, want error")
	}
}

func TestMarketplaceMessages(t *testing.T) {
	t.Setenv(envToken, "test-token")
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		if r.URL.Query().Get("page") == "1" {
			fmt.Fprintf(w, `{"pagination": {"page": 1, "pages": 2, "urls": {"next": "http://%s/marketplace/orders/1-1/messages?page=2&per_page=100"}},
				"messages": [{"type": "message", "message": "second"}]}`, r.Host)
			return
		}
		fmt.Fprint(w, `{"pagination": {"page": 2, "pages": 2, "urls": {}}, "messages": [{"type": "message", "message": "first"}]}`)
	}))
	defer srv.Close()

	stdout, _, err := run(t, srv, "marketplace", "messages", "1-1", "--all", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[0] != "/marketplace/orders/1-1/messages?page=1&per_page=100" {
		t.Errorf("requests = %q", seen)
	}
	if !strings.Contains(stdout, `"second"`) || !strings.Contains(stdout, `"first"`) {
		t.Errorf("stdout = %s, want both pages", stdout)
	}

	seen = nil
	if _, _, err := run(t, srv, "marketplace", "messages", " "); err == nil || len(seen) != 0 {
		t.Errorf("empty order ID gave %v after %d requests, want an error and none", err, len(seen))
	}
}

func TestMarketplaceInventory(t *testing.T) {
	t.Setenv(envToken, "test-token")
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		if r.URL.Path == "/oauth/identity" {
			fmt.Fprint(w, `{"id": 7, "username": "hasteful"}`)
			return
		}
		fmt.Fprint(w, `{"pagination": {"page": 1, "pages": 1, "urls": {}}, "listings": []}`)
	}))
	defer srv.Close()

	if _, _, err := run(t, srv, "marketplace", "inventory", "seller", "--status", "for sale", "--sort", "price"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, srv, "marketplace", "inventory", "--status", "DRAFT"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/users/seller/inventory?page=1&per_page=50&sort=price&status=For+Sale",
		"/oauth/identity",
		"/users/hasteful/inventory?page=1&per_page=50&status=Draft",
	}
	if strings.Join(seen, " ") != strings.Join(want, " ") {
		t.Errorf("requests = %q\nwant       %q", seen, want)
	}

	seen = nil
	for _, args := range [][]string{{"--status", "listed"}, {"--sort", "condition"}, {"a", "b"}} {
		if _, _, err := run(t, srv, append([]string{"marketplace", "inventory"}, args...)...); err == nil {
			t.Errorf("%q succeeded, want error", args)
		}
	}
	if len(seen) != 0 {
		t.Errorf("bad flags sent %q, want no requests", seen)
	}
}
