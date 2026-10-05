package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEditProfile(t *testing.T) {
	var method, path string
	var body map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		body = nil
		json.Unmarshal(b, &body)
		w.Write([]byte(`{"id": 7, "username": "hasteful", "location": "Anytown", "home_page": ""}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv, "test-token")

	u, err := c.EditProfile(t.Context(), "hasteful", ProfileEdit{Location: new("Anytown"), HomePage: new(""), Currency: new("EUR")})
	if err != nil || u.Location != "Anytown" {
		t.Fatalf("EditProfile = %+v, %v", u, err)
	}
	if method != http.MethodPost || path != "/users/hasteful" {
		t.Errorf("request = %s %s", method, path)
	}
	// Only the fields set are sent, and an empty one is sent to clear it.
	want := map[string]string{"location": "Anytown", "home_page": "", "curr_abbr": "EUR"}
	if len(body) != len(want) || body["location"] != "Anytown" || body["curr_abbr"] != "EUR" {
		t.Errorf("body = %v, want %v", body, want)
	}
	if v, ok := body["home_page"]; !ok || v != "" {
		t.Errorf("body = %v, want home_page sent empty to clear it", body)
	}

	method = ""
	if _, err := c.EditProfile(t.Context(), "hasteful", ProfileEdit{}); err == nil {
		t.Error("an empty edit succeeded, want error")
	}
	if _, err := c.EditProfile(t.Context(), "hasteful", ProfileEdit{Currency: new("XYZ")}); err == nil {
		t.Error("an unsupported currency succeeded, want error")
	}
	if method != "" {
		t.Errorf("refused edits sent a %s", method)
	}
}

func TestContributions(t *testing.T) {
	var uri string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uri = r.URL.RequestURI()
		w.Write([]byte(`{"pagination": {"page": 1, "pages": 1, "urls": {}}, "contributions": [
			{"id": 24661865, "title": "FRONTWAVE", "year": 2022, "date_added": "2022-09-28T17:16:19-07:00",
			 "artists": [{"name": "Popular Front"}], "estimated_weight": 230}]}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv, "")

	page, err := c.Contributions(t.Context(), "hasteful", ContributionQuery{Sort: "title", SortOrder: "desc", Page: Page{PerPage: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if uri != "/users/hasteful/contributions?per_page=100&sort=title&sort_order=desc" {
		t.Errorf("request = %q", uri)
	}
	r := page.Items[0]
	if r.ID != 24661865 || r.Title != "FRONTWAVE" || r.DateAdded != "2022-09-28T17:16:19-07:00" {
		t.Errorf("contribution = %+v", r)
	}
	if b, _ := json.Marshal(r); !strings.Contains(string(b), `"estimated_weight"`) {
		t.Errorf("re-encoded contribution lost fields the type does not name: %s", b)
	}

	uri = ""
	for _, bad := range []ContributionQuery{{Sort: "name"}, {SortOrder: "up"}} {
		if _, err := c.Contributions(t.Context(), "hasteful", bad); err == nil {
			t.Errorf("Contributions(%+v) succeeded, want error", bad)
		}
	}
	if uri != "" {
		t.Errorf("bad queries sent %q", uri)
	}
}
