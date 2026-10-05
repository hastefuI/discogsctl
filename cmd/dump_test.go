package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// dumpHost serves a listing with the years in pages, keyed by year, and
// records each request's query.
func dumpHost(t *testing.T, pages map[string]string) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("dump request sent an Authorization header")
		}
		if ua := r.Header.Get("User-Agent"); ua != userAgent("test") {
			t.Errorf("User-Agent = %q, want %q", ua, userAgent("test"))
		}
		prefix := r.URL.Query().Get("prefix")
		seen = append(seen, prefix)
		if prefix == "" {
			for year := range pages {
				fmt.Fprintf(w, `<a href="?prefix=data%%2F%s%%2F">%s/</a>`+"\n", year, year)
			}
			return
		}
		year := strings.TrimSuffix(strings.TrimPrefix(prefix, "data/"), "/")
		fmt.Fprint(w, "<pre>\n"+pages[year]+"</pre>")
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func dumpRow(date, name string) string {
	return fmt.Sprintf("%s 00:00:00             1.0 MB         <a href=\"?download=data%%2F%s%%2F%s\">%s</a>\n", date, date[:4], name, name)
}

func TestDumpListNewestYear(t *testing.T) {
	t.Setenv(envToken, "test-token")
	srv, seen := dumpHost(t, map[string]string{
		"2025": dumpRow("2025-12-01", "discogs_20251201_releases.xml.gz"),
		"2026": dumpRow("2026-10-01", "discogs_20261001_CHECKSUM.txt") +
			dumpRow("2026-10-01", "discogs_20261001_releases.xml.gz") +
			dumpRow("2026-10-01", "discogs_20261001_masters.xml.gz") +
			dumpRow("2026-09-01", "discogs_20260901_releases.xml.gz"),
	})

	stdout, _, err := run(t, srv, "dump", "list")
	if err != nil {
		t.Fatal(err)
	}
	want := "ID        DATE        TYPES             CHECKSUM\n" +
		"20261001  2026-10-01  masters,releases  yes\n" +
		"20260901  2026-09-01  releases          no\n"
	if stdout != want {
		t.Errorf("stdout\n%q\nwant\n%q", stdout, want)
	}
	if got := strings.Join(*seen, " "); got != " data/2026/" {
		t.Errorf("requests = %q, want the root then 2026 only", got)
	}
}

func TestDumpListEmptyNewestYearFallsBack(t *testing.T) {
	srv, seen := dumpHost(t, map[string]string{
		"2024": dumpRow("2024-12-01", "discogs_20241201_releases.xml.gz"),
		"2025": dumpRow("2025-12-01", "discogs_20251201_releases.xml.gz"),
		"2026": "",
	})

	stdout, _, err := run(t, srv, "dump", "list", "--output", "json")
	if err != nil {
		t.Fatal(err)
	}
	var dumps []struct {
		ID    string `json:"id"`
		Date  string `json:"date"`
		Files []struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(stdout), &dumps); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\n%s", err, stdout)
	}
	if len(dumps) != 1 || dumps[0].ID != "20251201" || dumps[0].Date != "2025-12-01" || dumps[0].Files[0].Type != "releases" {
		t.Errorf("dumps = %+v, want the December 2025 releases dump", dumps)
	}
	if got := strings.Join(*seen, " "); got != " data/2026/ data/2025/" {
		t.Errorf("requests = %q, want 2026 then 2025 and no further", got)
	}
}

func TestDumpListAllAndYear(t *testing.T) {
	srv, seen := dumpHost(t, map[string]string{
		"2025": dumpRow("2025-12-01", "discogs_20251201_releases.xml.gz"),
		"2026": dumpRow("2026-01-01", "discogs_20260101_releases.xml.gz"),
	})

	stdout, _, err := run(t, srv, "dump", "list", "--all")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "2026-01-01") || strings.Index(stdout, "2026-01-01") > strings.Index(stdout, "2025-12-01") {
		t.Errorf("--all stdout\n%s\nwant both years, newest first", stdout)
	}

	*seen = nil
	if _, _, err := run(t, srv, "dump", "list", "--year", "2025"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*seen, " "); got != "data/2025/" {
		t.Errorf("--year 2025 requests = %q, want only data/2025/", got)
	}

	if _, _, err := run(t, srv, "dump", "list", "--year", "2025", "--all"); err == nil {
		t.Error("--year with --all succeeded, want error")
	}
	if _, _, err := run(t, srv, "dump", "list", "--year", "0"); err == nil {
		t.Error("--year 0 succeeded, want error")
	}
}

func TestDumpListType(t *testing.T) {
	srv, _ := dumpHost(t, map[string]string{
		"2025": dumpRow("2025-03-01", "discogs_20250301_CHECKSUM.txt") +
			dumpRow("2025-03-01", "discogs_20250301_masters.xml.gz") +
			dumpRow("2025-03-01", "discogs_20250301_releases.xml.gz") +
			dumpRow("2025-02-01", "discogs_20250201_CHECKSUM.txt") +
			dumpRow("2025-02-01", "discogs_20250201_releases.xml.gz"),
	})

	stdout, _, err := run(t, srv, "dump", "list", "--year", "2025", "--type", "releases")
	if err != nil {
		t.Fatal(err)
	}
	want := "ID        DATE        TYPES     CHECKSUM\n" +
		"20250301  2025-03-01  releases  yes\n" +
		"20250201  2025-02-01  releases  yes\n"
	if stdout != want {
		t.Errorf("--type releases stdout\n%q\nwant\n%q", stdout, want)
	}

	stdout, _, err = run(t, srv, "dump", "list", "--year", "2025", "--type", "Masters", "--type", "releases", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var dumps []struct {
		ID    string `json:"id"`
		Files []struct {
			Type string `json:"type"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(stdout), &dumps); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\n%s", err, stdout)
	}
	if len(dumps) != 1 || dumps[0].ID != "20250301" || len(dumps[0].Files) != 3 {
		t.Errorf("--type masters,releases = %+v, want only 20250301 with its checksum, masters and releases", dumps)
	}

	if _, _, err := run(t, srv, "dump", "list", "--year", "2025", "--type", "release"); err == nil || !strings.Contains(err.Error(), "invalid --type") {
		t.Errorf("--type release error = %v, want an invalid --type error", err)
	}
}
