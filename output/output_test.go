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

func TestUserAccountFields(t *testing.T) {
	body := `{"id": 1001, "username": "user", "num_pending": 4, "buyer_rating": 100, "buyer_rating_stars": 5,
		"buyer_num_ratings": 1, "seller_rating": 0, "seller_rating_stars": 0, "seller_num_ratings": 0,
		"activated": true, "marketplace_suspended": true, "is_staff": false,
		"avatar_url": "https://example.com/avatar.png", "banner_url": "",
		"inventory_url": "https://api.discogs.com/users/user/inventory",
		"collection_folders_url": "https://api.discogs.com/users/user/collection/folders",
		"collection_fields_url": "https://api.discogs.com/users/user/collection/fields",
		"wantlist_url": "https://api.discogs.com/users/user/wants"}`
	var u api.User
	if err := json.Unmarshal([]byte(body), &u); err != nil {
		t.Fatal(err)
	}
	if u.NumPending != 4 || u.BuyerRatingStars != 5 || u.Activated == nil || !*u.Activated ||
		u.MarketplaceSuspended == nil || !*u.MarketplaceSuspended || u.IsStaff == nil || *u.IsStaff ||
		u.AvatarURL == "" || u.InventoryURL == "" || u.CollectionFoldersURL == "" || u.CollectionFieldsURL == "" || u.WantlistURL == "" {
		t.Errorf("decoded %+v, want every account field", u)
	}

	var buf bytes.Buffer
	if err := Write(&buf, FormatText, &u); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Staff:       No\n", "Activated:   Yes\n", "Seller:      marketplace suspended\n"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("text output\n%s\nwant a line %q", buf.String(), want)
		}
	}

	var bare api.User
	if err := json.Unmarshal([]byte(`{"id": 1001, "username": "user"}`), &bare); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := Write(&buf, FormatText, &bare); err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"Staff:", "Activated:", "suspended"} {
		if strings.Contains(buf.String(), absent) {
			t.Errorf("a profile without the fields printed %q:\n%s", absent, buf.String())
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

func TestTextOrderMessages(t *testing.T) {
	body := `[
		{"type": "message", "timestamp": "2015-06-02T13:17:07-07:00", "from": {"username": "example_seller"},
		 "message": "Thank you for your order!\r\nIt ships Monday.\r\n"},
		{"type": "status", "timestamp": "2015-06-02T13:16:57-07:00", "actor": {"username": "example_seller"},
		 "message": "example_buyer changed the order status to Shipped."},
		{"type": "refund_sent", "timestamp": "not a time", "message": "example_seller sent refund of $5.00."}]`
	var msgs []api.OrderMessage
	if err := json.Unmarshal([]byte(body), &msgs); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, FormatText, msgs); err != nil {
		t.Fatal(err)
	}
	want := "2015-06-02 13:17  message  example_seller\n" +
		"  Thank you for your order!\n" +
		"  It ships Monday.\n" +
		"\n" +
		"2015-06-02 13:16  status  example_seller\n" +
		"  example_buyer changed the order status to Shipped.\n" +
		"\n" +
		"not a time  refund_sent\n" +
		"  example_seller sent refund of $5.00.\n"
	if buf.String() != want {
		t.Errorf("text output\n%q\nwant\n%q", buf.String(), want)
	}
}

func TestTextInventory(t *testing.T) {
	body := `[{"id": 1, "status": "For Sale", "price": {"currency": "USD", "value": 8.5},
		"condition": "Very Good (VG)", "sleeve_condition": "Very Good Plus (VG+)", "posted": "2025-03-21T16:46:25-07:00",
		"release": {"description": "Vallanzaska - Cheope (CD, Album, RE)"}},
		{"id": 2, "status": "Draft", "price": {"currency": "EUR", "value": 20}, "condition": "Mint (M)", "sleeve_condition": "",
		"release": {"description": "Untitled"}}]`
	var listings []api.Listing
	if err := json.Unmarshal([]byte(body), &listings); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, FormatText, listings); err != nil {
		t.Fatal(err)
	}
	want := "ID  STATUS    CONDITION  PRICE      POSTED      RELEASE\n" +
		"1   For Sale  VG / VG+   8.50 USD   2025-03-21  Vallanzaska - Cheope (CD, Album, RE)\n" +
		"2   Draft     M          20.00 EUR              Untitled\n"
	if buf.String() != want {
		t.Errorf("text output\n%q\nwant\n%q", buf.String(), want)
	}
}

func TestTextPriceSuggestions(t *testing.T) {
	body := `{"Poor (P)": {"currency": "USD", "value": 28.75}, "Mint (M)": {"currency": "USD", "value": 546.25},
		"Very Good (VG)": {"currency": "USD", "value": 258.75}, "Shiny (S)": {"currency": "USD", "value": 1}}`
	var p api.PriceSuggestions
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, FormatText, &p); err != nil {
		t.Fatal(err)
	}
	want := "CONDITION       PRICE\n" +
		"Mint (M)        546.25 USD\n" +
		"Very Good (VG)  258.75 USD\n" +
		"Poor (P)        28.75 USD\n" +
		"Shiny (S)       1.00 USD\n"
	if buf.String() != want {
		t.Errorf("text output\n%q\nwant best first, then unknown grades\n%q", buf.String(), want)
	}

	var none api.PriceSuggestions
	if err := json.Unmarshal([]byte(`{}`), &none); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := Write(&buf, FormatText, &none); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "No results.\n" {
		t.Errorf("no suggestions printed %q", buf.String())
	}
}

func TestTextReleases(t *testing.T) {
	body := `[{"id": 24661865, "date_added": "2022-09-28T17:16:19-07:00", "year": 2022, "title": "FRONTWAVE",
		"artists": [{"name": "Popular Front"}], "formats": [{"name": "Vinyl", "qty": "1", "descriptions": ["LP"]}]}]`
	var releases []api.Release
	if err := json.Unmarshal([]byte(body), &releases); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, FormatText, releases); err != nil {
		t.Fatal(err)
	}
	want := "ID        ADDED       YEAR  ARTIST         TITLE      FORMAT\n" +
		"24661865  2022-09-28  2022  Popular Front  FRONTWAVE  Vinyl, LP\n"
	if buf.String() != want {
		t.Errorf("text output\n%q\nwant\n%q", buf.String(), want)
	}
}

func TestTextList(t *testing.T) {
	body := `{"id": 100, "name": "Example List", "public": true, "date_added": "2009-06-23T03:02:05-07:00",
		"date_changed": "2010-01-02T03:02:05-07:00", "user": {"username": "user"}, "uri": "https://www.discogs.com/lists/100",
		"description": "Line one.\r\nLine two.",
		"items": [{"id": 26694, "type": "release", "display_title": "Paolo Zerletti - Power", "comment": "opener"},
		          {"id": 3227, "type": "artist", "display_title": "Silent Phase", "comment": ""}]}`
	var l api.List
	if err := json.Unmarshal([]byte(body), &l); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, FormatText, &l); err != nil {
		t.Fatal(err)
	}
	want := "ID:          100\n" +
		"Name:        Example List\n" +
		"Owner:       user\n" +
		"Public:      yes\n" +
		"Added:       2009-06-23\n" +
		"Changed:     2010-01-02\n" +
		"Items:       2\n" +
		"URL:         https://www.discogs.com/lists/100\n" +
		"Description: Line one. Line two.\n" +
		"\nItems:\n" +
		"  release  26694  Paolo Zerletti - Power  opener\n" +
		"  artist   3227   Silent Phase            \n"
	if buf.String() != want {
		t.Errorf("text output\n%q\nwant\n%q", buf.String(), want)
	}
}

func TestTextSubmissions(t *testing.T) {
	body := `{"artists": [{"id": 9, "name": "Westside Gunn", "data_quality": "Correct"}],
		"labels": [{"id": 2939267, "name": "Popular Front", "data_quality": "Needs Vote"}],
		"releases": [{"id": 24661865, "title": " FRONTWAVE ", "artists": [{"name": "Popular Front"}], "data_quality": "Needs Vote"}]}`
	var s api.Submissions
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, FormatText, &s); err != nil {
		t.Fatal(err)
	}
	want := "TYPE     ID        NAME                       QUALITY\n" +
		"artist   9         Westside Gunn              Correct\n" +
		"label    2939267   Popular Front              Needs Vote\n" +
		"release  24661865  Popular Front - FRONTWAVE  Needs Vote\n"
	if buf.String() != want {
		t.Errorf("text output\n%q\nwant\n%q", buf.String(), want)
	}
}

func TestTextListing(t *testing.T) {
	body := `{"id": 3000001, "status": "For Sale", "price": {"value": 8.5, "currency": "USD"}, "allow_offers": true,
		"condition": "Very Good (VG)", "sleeve_condition": "Very Good Plus (VG+)", "ships_from": "United States",
		"posted": "2025-03-21T16:46:25-07:00", "seller": {"username": "seller"},
		"uri": "https://www.discogs.com/sell/item/3000001", "comments": "some scuffs\non disc",
		"release": {"id": 11180538, "description": "Vallanzaska - Cheope (CD, Album, RE)"}}`
	var l api.Listing
	if err := json.Unmarshal([]byte(body), &l); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, FormatText, &l); err != nil {
		t.Fatal(err)
	}
	want := "ID:         3000001\n" +
		"Status:     For Sale\n" +
		"Release:    Vallanzaska - Cheope (CD, Album, RE) (11180538)\n" +
		"Price:      8.50 USD\n" +
		"Offers:     yes\n" +
		"Media:      Very Good (VG)\n" +
		"Sleeve:     Very Good Plus (VG+)\n" +
		"Ships from: United States\n" +
		"Posted:     2025-03-21\n" +
		"Seller:     seller\n" +
		"URL:        https://www.discogs.com/sell/item/3000001\n" +
		"Comments:   some scuffs on disc\n"
	if buf.String() != want {
		t.Errorf("text output\n%q\nwant\n%q", buf.String(), want)
	}
}
