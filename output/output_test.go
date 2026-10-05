package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"go.hasteful.org/discogsctl/api"
)

func TestJSONIsTheDiscogsBodyUnescaped(t *testing.T) {
	var items []api.LabelRelease
	body := `[{"id":1,"title":"Stars","artist":"R & S <Mix>","extra":true}]`
	if err := json.Unmarshal([]byte(body), &items); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, FormatJSON, items); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, `"artist": "R & S <Mix>"`) || !strings.Contains(got, `"extra": true`) {
		t.Errorf("JSON output\n%s\nwant the body with every field and no HTML escaping", got)
	}
}

func TestTextTable(t *testing.T) {
	items := []api.Folder{{ID: 0, Name: "All", Count: 23}, {ID: 1, Name: "Uncategorized", Count: 20}}
	var buf bytes.Buffer
	if err := Write(&buf, FormatText, items); err != nil {
		t.Fatal(err)
	}
	want := "ID  NAME           COUNT\n0   All            23\n1   Uncategorized  20\n"
	if buf.String() != want {
		t.Errorf("text output\n%q\nwant\n%q", buf.String(), want)
	}
}

func TestTextEmptyListing(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, FormatText, []api.Want{}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "No results.\n" {
		t.Errorf("text output %q, want No results.", buf.String())
	}
}

func TestArtists(t *testing.T) {
	credits := []api.ArtistCredit{
		{Name: "Eric B.", Join: "&"},
		{Name: "Rakim (2)", ANV: "Rakim", Join: ","},
		{Name: "Someone"},
	}
	if got, want := artists(credits), "Eric B. & Rakim, Someone"; got != want {
		t.Errorf("artists = %q, want %q", got, want)
	}
}

func TestCellTruncatesByCharacter(t *testing.T) {
	s := strings.Repeat("ニ", maxCell+5)
	got := cell(s)
	if r := []rune(got); len(r) != maxCell || r[len(r)-1] != '…' {
		t.Errorf("cell(%d runes) = %d runes ending %q, want %d ending …", len([]rune(s)), len(r), r[len(r)-1], maxCell)
	}
}

func TestInvalidFormat(t *testing.T) {
	if err := Write(&bytes.Buffer{}, "yaml", 1); err == nil {
		t.Error("Write(yaml) succeeded, want error")
	}
}

func TestTextUserPrivateCounts(t *testing.T) {
	for _, tt := range []struct {
		name, body, want string
	}{
		{"private", `{"id": 1, "username": "a", "num_for_sale": 0}`, "Collection:  private\nWantlist:    private\n"},
		{"public and empty", `{"id": 1, "username": "a", "num_collection": 0, "num_wantlist": 3}`, "Collection:  0\nWantlist:    3\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var u api.User
			if err := json.Unmarshal([]byte(tt.body), &u); err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := Write(&buf, FormatText, &u); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(buf.String(), tt.want) {
				t.Errorf("text output\n%s\nwant it to contain\n%s", buf.String(), tt.want)
			}
		})
	}
}

func TestTextUserRatings(t *testing.T) {
	body := `{"id": 2000001, "username": "user", "rank": 116.0, "rating_avg": 5.0,
		"releases_contributed": 9, "releases_rated": 1, "buyer_rating": 100.0, "buyer_num_ratings": 25,
		"seller_rating": 99.5, "seller_num_ratings": 160}`
	var u api.User
	if err := json.Unmarshal([]byte(body), &u); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, FormatText, &u); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Rank:        116\n",
		"Rating avg:  5.00\n",
		"Contributed: 9\n",
		"Buyer:       100.00% (25 ratings)\n",
		"Seller:      99.50% (160 ratings)\n",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("text output\n%s\nwant a line %q", buf.String(), want)
		}
	}
}

func TestTextMarketplaceStats(t *testing.T) {
	for _, tt := range []struct {
		name, body, want string
	}{
		{"for sale", `{"num_for_sale": 119, "lowest_price": {"value": 0.59, "currency": "EUR"}, "blocked_from_sale": false}`,
			"For sale: 119\nLowest:   0.59 EUR\nBlocked:  no\n"},
		{"none for sale", `{"num_for_sale": null, "lowest_price": null, "blocked_from_sale": false}`,
			"For sale: 0\nBlocked:  no\n"},
		{"blocked", `{"num_for_sale": null, "lowest_price": null, "blocked_from_sale": true}`,
			"Blocked: yes\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var s api.MarketplaceStats
			if err := json.Unmarshal([]byte(tt.body), &s); err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := Write(&buf, FormatText, &s); err != nil {
				t.Fatal(err)
			}
			if buf.String() != tt.want {
				t.Errorf("text output\n%q\nwant\n%q", buf.String(), tt.want)
			}
		})
	}
}

func TestTextOrder(t *testing.T) {
	body := `{"id": "1-1", "status": "Shipped", "created": "2011-10-21T09:25:17-07:00", "last_activity": "2011-10-22T09:25:17-07:00",
		"buyer": {"username": "example_buyer"}, "total": {"currency": "USD", "value": 42}, "fee": {"currency": "USD", "value": 2.52},
		"shipping": {"currency": "USD", "method": "Standard", "value": 5}, "archived": false,
		"tracking": {"number": "1Z999", "carrier": "UPS"}, "next_status": ["Shipped", "Refund Sent"],
		"additional_instructions": "please use\nsturdy packaging.", "uri": "https://www.discogs.com/sell/order/1-1",
		"shipping_address": "Asdf Exampleton\n234 NE Asdf St.",
		"items": [{"release": {"id": 1, "description": "Persuader, The - Stockholm"}, "price": {"currency": "USD", "value": 42},
		  "media_condition": "Mint (M)", "sleeve_condition": "Mint (M)"}]}`
	var o api.Order
	if err := json.Unmarshal([]byte(body), &o); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, FormatText, &o); err != nil {
		t.Fatal(err)
	}
	want := "ID:            1-1\n" +
		"Status:        Shipped\n" +
		"Created:       2011-10-21\n" +
		"Last activity: 2011-10-22\n" +
		"Buyer:         example_buyer\n" +
		"Total:         42.00 USD\n" +
		"Shipping:      5.00 USD (Standard)\n" +
		"Fee:           2.52 USD\n" +
		"Tracking:      UPS 1Z999\n" +
		"Archived:      no\n" +
		"Next status:   Shipped, Refund Sent\n" +
		"Instructions:  please use sturdy packaging.\n" +
		"URL:           https://www.discogs.com/sell/order/1-1\n" +
		"\nItems:\n" +
		"  1  Persuader, The - Stockholm  Mint (M) / Mint (M)  42.00 USD\n"
	if buf.String() != want {
		t.Errorf("text output\n%s\nwant\n%s", buf.String(), want)
	}
	if strings.Contains(buf.String(), "Exampleton") {
		t.Error("the text view printed the shipping address")
	}
}
