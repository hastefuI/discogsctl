package dump

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"go.hasteful.org/discogsctl/api"
)

const testAgent = "discogsctl-test/1.0 (+https://example.com)"

// rootPage and yearPage follow the listing data.discogs.com served in
// October 2026, trimmed.
const rootPage = `<html><body><pre>Download Discogs Data</pre><h2>Discogs Data</h2>
    <pre>Last Modified                   Size           Name
-----------------------------------------------------------------------------------------------
                                -              <a href="?prefix=data%2F2008%2F">2008/</a>
                                -              <a href="?prefix=data%2F2025%2F">2025/</a>
                                -              <a href="?prefix=data%2F2026%2F">2026/</a>
</pre></body></html>`

const yearPage = `<html><body><h2>Discogs Data</h2>
    <pre>Last Modified                   Size           Name
-----------------------------------------------------------------------------------------------
                                               <a href="?prefix=data%2F">../</a>
2026-09-01 19:31:21             388 B          <a href="?download=data%2F2026%2Fdiscogs_20260901_CHECKSUM.txt">discogs_20260901_CHECKSUM.txt</a>
2026-09-01 19:31:20             474.1 MB       <a href="?download=data%2F2026%2Fdiscogs_20260901_artists.xml.gz">discogs_20260901_artists.xml.gz</a>
2026-09-01 19:21:51             10.5 GB        <a href="?download=data%2F2026%2Fdiscogs_20260901_releases.xml.gz">discogs_20260901_releases.xml.gz</a>
2026-10-01 19:36:11             388 B          <a href="?download=data%2F2026%2Fdiscogs_20261001_CHECKSUM.txt">discogs_20261001_CHECKSUM.txt</a>
2026-10-01 16:36:15             475.7 MB       <a href="?download=data%2F2026%2Fdiscogs_20261001_artists.xml.gz">discogs_20261001_artists.xml.gz</a>
2026-10-01 09:01:19             87.0 MB        <a href="?download=data%2F2026%2Fdiscogs_20261001_labels.xml.gz">discogs_20261001_labels.xml.gz</a>
2026-10-01 11:24:06             600.5 MB       <a href="?download=data%2F2026%2Fdiscogs_20261001_masters.xml.gz">discogs_20261001_masters.xml.gz</a>
2026-10-01 19:35:58             10.5 GB        <a href="?download=data%2F2026%2Fdiscogs_20261001_releases.xml.gz">discogs_20261001_releases.xml.gz</a>
<a href="?download=data%2F2026%2Fdiscogs_20261001_releases.xml.gz">duplicate</a>
<a href="?download=data%2F2026%2F..%2F..%2Fetc%2Fpasswd">traversal</a>
<a href="https://evil.example/?download=data%2F2026%2Fdiscogs_20261101_releases.xml.gz">other host</a>
<a href="?download=data%2F2026%2Fdiscogs_20261341_releases.xml.gz">bad date</a>
</pre></body></html>`

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := New(Options{BaseURL: srv.URL, UserAgent: testAgent, HTTP: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func listing(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); ua != testAgent {
			t.Errorf("User-Agent = %q, want %q", ua, testAgent)
		}
		switch r.URL.Query().Get("prefix") {
		case "":
			fmt.Fprint(w, rootPage)
		case "data/2026/":
			fmt.Fprint(w, yearPage)
		default:
			fmt.Fprint(w, `<html><body><pre></pre></body></html>`)
		}
	}
}

func TestNewUserAgent(t *testing.T) {
	for _, agent := range []string{"", "curl/8.0", "Mozilla/5.0 (Macintosh)"} {
		if _, err := New(Options{UserAgent: agent}); err == nil {
			t.Errorf("New(UserAgent %q) succeeded, want error", agent)
		}
	}
}

func TestYears(t *testing.T) {
	c := newTestClient(t, listing(t))
	years, err := c.Years(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{2008, 2025, 2026}; !slices.Equal(years, want) {
		t.Errorf("Years = %v, want %v", years, want)
	}
}

func TestYearsWithoutLinksIsAnError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body>Checking your browser</body></html>`)
	})
	if _, err := c.Years(t.Context()); err == nil {
		t.Error("Years on a page with no year links succeeded, want error")
	}
}

func TestList(t *testing.T) {
	c := newTestClient(t, listing(t))
	dumps, err := c.List(t.Context(), 2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(dumps) != 2 || dumps[0].Date != "2026-10-01" || dumps[1].Date != "2026-09-01" {
		t.Fatalf("dates = %+v, want 2026-10-01 then 2026-09-01", dumps)
	}
	if dumps[0].ID != "20261001" || dumps[1].ID != "20260901" {
		t.Errorf("IDs = %q, %q; want 20261001, 20260901", dumps[0].ID, dumps[1].ID)
	}

	oct := dumps[0]
	if got, want := oct.Types(), []string{"artists", "labels", "masters", "releases"}; !slices.Equal(got, want) {
		t.Errorf("October types = %v, want %v", got, want)
	}
	if got, want := dumps[1].Types(), []string{"artists", "releases"}; !slices.Equal(got, want) {
		t.Errorf("September types = %v, want %v", got, want)
	}

	sum, ok := oct.Checksum()
	if !ok || sum.Name != "discogs_20261001_CHECKSUM.txt" || sum.Size != "388 B" {
		t.Errorf("October checksum = %+v, %v", sum, ok)
	}

	releases := oct.Files[len(oct.Files)-1]
	want := File{
		Type:     "releases",
		Name:     "discogs_20261001_releases.xml.gz",
		URL:      c.base.String() + "?download=data%2F2026%2Fdiscogs_20261001_releases.xml.gz",
		Size:     "10.5 GB",
		Modified: "2026-10-01 19:35:58",
	}
	if releases != want {
		t.Errorf("releases file = %+v\nwant %+v", releases, want)
	}

	for _, d := range dumps {
		for _, f := range d.Files {
			if !strings.HasPrefix(f.URL, c.base.String()) || strings.Contains(f.Name, "/") {
				t.Errorf("file %+v leaves the dump host or names a path", f)
			}
		}
	}
	if n := len(oct.Files); n != 5 {
		t.Errorf("October has %d files, want 5 with the duplicate and bad links ignored", n)
	}
}

func TestListEmptyYear(t *testing.T) {
	c := newTestClient(t, listing(t))
	dumps, err := c.List(t.Context(), 2030)
	if err != nil || len(dumps) != 0 {
		t.Errorf("List(2030) = %v, %v; want no dumps and no error", dumps, err)
	}
}

func TestListStatusIsAnAPIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	_, err := c.List(t.Context(), 2026)
	apiErr, ok := errors.AsType[*api.Error](err)
	if !ok || apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("error = %v, want an *api.Error with status 503", err)
	}
}

func TestHasAndOnly(t *testing.T) {
	d := Dump{ID: "20250201", Files: []File{
		{Type: TypeChecksum, Name: "discogs_20250201_CHECKSUM.txt"},
		{Type: "artists", Name: "discogs_20250201_artists.xml.gz"},
		{Type: "releases", Name: "discogs_20250201_releases.xml.gz"},
	}}
	if !d.Has() || !d.Has("releases") || !d.Has("artists", "releases") {
		t.Error("Has is false for types the dump has")
	}
	if d.Has("masters") || d.Has("releases", "masters") {
		t.Error("Has is true for a type the dump lacks")
	}

	only := d.Only("releases")
	var names []string
	for _, f := range only.Files {
		names = append(names, f.Name)
	}
	if want := []string{"discogs_20250201_CHECKSUM.txt", "discogs_20250201_releases.xml.gz"}; !slices.Equal(names, want) {
		t.Errorf("Only(releases) files = %v, want %v", names, want)
	}
	if len(d.Files) != 3 {
		t.Errorf("Only changed the original dump: %d files, want 3", len(d.Files))
	}
}
