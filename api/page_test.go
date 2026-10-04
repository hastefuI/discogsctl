package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPageValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL)
	}))
	defer srv.Close()
	c := newTestClient(t, srv, "")

	for _, p := range []Page{{Page: -1}, {PerPage: -1}, {PerPage: MaxPerPage + 1}} {
		if _, err := c.LabelReleases(t.Context(), 1, p); err == nil {
			t.Errorf("LabelReleases(%+v) succeeded, want error", p)
		}
	}
}

func TestEachFollowsNextOverHTTPS(t *testing.T) {
	var host string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Discogs token=tok" {
			t.Errorf("page %s sent without the token header", r.URL.Query().Get("page"))
		}
		page := r.URL.Query().Get("page")
		// Discogs has served next links with an http scheme. The client must
		// keep https for them, so this server, which only speaks TLS, sees
		// every page.
		next := ""
		switch page {
		case "", "1":
			page = "1"
			next = fmt.Sprintf("http://%s/users/u/wants?page=2&per_page=2", host)
		case "2":
			next = fmt.Sprintf("http://%s/users/u/wants?page=3&per_page=2", host)
		}
		fmt.Fprintf(w, `{"pagination": {"page": %s, "pages": 3, "per_page": 2, "items": 5, "urls": {"next": %q}},
			"wants": [{"id": %s1}, {"id": %s2}]}`, page, next, page, page)
	}))
	defer srv.Close()
	host = strings.TrimPrefix(srv.URL, "https://")

	c := newTestClient(t, srv, "tok")
	first, err := c.Wantlist(t.Context(), "u", Page{PerPage: 2})
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	if err := first.Each(t.Context(), func(w Want) error {
		ids = append(ids, w.ID)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := []int{11, 12, 21, 22, 31, 32}
	if fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}

func TestNextRefusesAnotherHost(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"pagination": {"page": 1, "pages": 2, "urls": {"next": "https://evil.example/users/u/wants?page=2"}}, "wants": []}`))
	}))
	defer srv.Close()

	first, err := newTestClient(t, srv, "tok").Wantlist(t.Context(), "u", Page{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Next(t.Context()); err == nil || !strings.Contains(err.Error(), "evil.example") {
		t.Errorf("Next error = %v, want a refusal naming evil.example", err)
	}
	if calls != 1 {
		t.Errorf("server saw %d requests, want 1", calls)
	}
}

func TestEmptyListIsNotNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"pagination": {"page": 1, "pages": 0, "urls": {}}, "results": null}`))
	}))
	defer srv.Close()

	p, err := newTestClient(t, srv, "tok").Search(t.Context(), SearchQuery{Query: "nothing"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Items == nil {
		t.Error("Items is nil, want an empty slice so JSON prints []")
	}
	if next, err := p.Next(t.Context()); next != nil || err != nil {
		t.Errorf("Next = %v, %v, want nil, nil on the last page", next, err)
	}
}

func TestSearchQuery(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Write([]byte(`{"pagination": {}, "results": []}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv, "tok")

	_, err := c.Search(t.Context(), SearchQuery{
		Type: "release", Artist: "Nine Inch Nails", Year: "1994", ReleaseTitle: "The Downward Spiral",
		Page: Page{Page: 2, PerPage: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "artist=Nine+Inch+Nails&page=2&per_page=10&release_title=The+Downward+Spiral&type=release&year=1994"
	if query != want {
		t.Errorf("query = %q, want %q", query, want)
	}

	if _, err := c.Search(t.Context(), SearchQuery{Type: "track"}); err == nil {
		t.Error("Search(type track) succeeded, want error")
	}
}
