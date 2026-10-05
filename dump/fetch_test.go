package dump

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// dumpServer serves a 2026 listing with two dumps, their CHECKSUM files and
// their data files. files maps a file name to its body; sums overrides the
// SHA-256 the CHECKSUM file gives a name.
type dumpServer struct {
	files     map[string]string
	sums      map[string]string
	downloads atomic.Int32
	encoding  atomic.Value
	handler   func(w http.ResponseWriter, r *http.Request, name string) bool
}

func newDumpServer(t *testing.T) (*dumpServer, *Client) {
	t.Helper()
	ds := &dumpServer{
		files: map[string]string{
			"discogs_20261001_labels.xml.gz":   "october labels",
			"discogs_20261001_releases.xml.gz": "october releases",
			"discogs_20260901_releases.xml.gz": "september releases",
		},
		sums: map[string]string{},
	}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("prefix") == "":
			if q.Get("download") == "" {
				fmt.Fprint(w, `<a href="?prefix=data%2F2025%2F">2025/</a><a href="?prefix=data%2F2026%2F">2026/</a>`)
				return
			}
		case q.Get("prefix") == "data/2026/":
			var b strings.Builder
			for _, name := range []string{
				"discogs_20261001_CHECKSUM.txt", "discogs_20261001_labels.xml.gz", "discogs_20261001_releases.xml.gz",
				"discogs_20260901_CHECKSUM.txt", "discogs_20260901_releases.xml.gz",
			} {
				fmt.Fprintf(&b, "<a href=\"?download=data%%2F2026%%2F%s\">%s</a>\n", name, name)
			}
			fmt.Fprint(w, b.String())
			return
		default:
			fmt.Fprint(w, "<pre></pre>")
			return
		}

		name := filepath.Base(q.Get("download"))
		if strings.HasSuffix(name, "_CHECKSUM.txt") {
			date := strings.TrimSuffix(strings.TrimPrefix(name, "discogs_"), "_CHECKSUM.txt")
			for file, body := range ds.files {
				if strings.Contains(file, date) {
					sum := sha(body)
					if s, ok := ds.sums[file]; ok {
						sum = s
					}
					fmt.Fprintf(w, "%s %s\n", sum, file)
				}
			}
			return
		}
		ds.downloads.Add(1)
		ds.encoding.Store(r.Header.Get("Accept-Encoding"))
		if ds.handler != nil && ds.handler(w, r, name) {
			return
		}
		body, ok := ds.files[name]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, body)
	})
	return ds, c
}

func TestFind(t *testing.T) {
	_, c := newDumpServer(t)
	for _, id := range []string{"20261001", "202610", "2026100"} {
		d, err := c.Find(t.Context(), id)
		if err != nil || d.ID != "20261001" {
			t.Errorf("Find(%q) = %q, %v; want 20261001", id, d.ID, err)
		}
	}

	_, err := c.Find(t.Context(), "2026")
	if err == nil || !strings.Contains(err.Error(), "20261001, 20260901") {
		t.Errorf("Find(2026) error = %v, want one naming both matches", err)
	}
	if _, err := c.Find(t.Context(), "202611"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Find(202611) error = %v, want ErrNotFound", err)
	}
	for _, id := range []string{"", "202", "2026-10", "202610011", "../x"} {
		if _, err := c.Find(t.Context(), id); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("Find(%q) error = %v, want an invalid ID error", id, err)
		}
	}
}

func TestLatest(t *testing.T) {
	_, c := newDumpServer(t)
	d, err := c.Latest(t.Context())
	if err != nil || d.ID != "20261001" {
		t.Errorf("Latest() = %q, %v; want 20261001", d.ID, err)
	}
	if _, err := c.Latest(t.Context(), "masters"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Latest(masters) error = %v, want ErrNotFound", err)
	}
}

func TestParseChecksums(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	got, err := ParseChecksums(strings.NewReader(sum + " discogs_20261001_labels.xml.gz\n\n" + strings.ToUpper(sum) + "  *discogs_20261001_releases.xml.gz\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["discogs_20261001_labels.xml.gz"] != sum || got["discogs_20261001_releases.xml.gz"] != sum {
		t.Errorf("ParseChecksums = %v", got)
	}

	for _, body := range []string{
		"",
		"nothing here",
		sum + " ../etc/passwd",
		"abc discogs_20261001_labels.xml.gz",
		sum + " discogs_20261001_labels.xml.gz extra",
	} {
		if _, err := ParseChecksums(strings.NewReader(body)); err == nil {
			t.Errorf("ParseChecksums(%q) succeeded, want error", body)
		}
	}
}

// fetchOne finds dump id, reads its checksums and fetches its file of typ.
func fetchOne(t *testing.T, c *Client, id, typ, dir string) (Fetched, error) {
	t.Helper()
	d, err := c.Find(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	sums, err := c.Checksums(t.Context(), d)
	if err != nil {
		t.Fatal(err)
	}
	f := d.Only(typ).Files[1]
	return c.Fetch(t.Context(), f, sums[f.Name], dir, nil)
}

func TestFetch(t *testing.T) {
	ds, c := newDumpServer(t)
	dir := t.TempDir()
	var calls []int64
	d, _ := c.Find(t.Context(), "20261001")
	sums, _ := c.Checksums(t.Context(), d)
	f := d.Only("releases").Files[1]

	got, err := c.Fetch(t.Context(), f, sums[f.Name], dir, func(done, total int64) { calls = append(calls, done) })
	if err != nil {
		t.Fatal(err)
	}
	want := Fetched{Type: "releases", Name: f.Name, Path: filepath.Join(dir, f.Name), SHA256: sha("october releases")}
	if got != want {
		t.Errorf("Fetch = %+v\nwant %+v", got, want)
	}
	if b, _ := os.ReadFile(got.Path); string(b) != "october releases" {
		t.Errorf("file holds %q", b)
	}
	if len(calls) == 0 || calls[len(calls)-1] != int64(len("october releases")) {
		t.Errorf("progress calls = %v, want the last at the full length", calls)
	}
	if enc, _ := ds.encoding.Load().(string); enc != "identity" {
		t.Errorf("Accept-Encoding = %q, want identity so a .gz is not unzipped in transit", enc)
	}

	again, err := c.Fetch(t.Context(), f, sums[f.Name], dir, nil)
	if err != nil || !again.Skipped || ds.downloads.Load() != 1 {
		t.Errorf("second Fetch = %+v, %v after %d downloads; want skipped with no new download", again, err, ds.downloads.Load())
	}

	os.WriteFile(got.Path, []byte("corrupt"), 0o644)
	again, err = c.Fetch(t.Context(), f, sums[f.Name], dir, nil)
	if b, _ := os.ReadFile(got.Path); err != nil || again.Skipped || string(b) != "october releases" {
		t.Errorf("Fetch over a corrupt file = %+v, %v, file %q; want it downloaded again", again, err, b)
	}
}

func TestFetchChecksumMismatchLeavesNothing(t *testing.T) {
	ds, c := newDumpServer(t)
	ds.sums["discogs_20261001_labels.xml.gz"] = sha("something else")
	dir := t.TempDir()

	_, err := fetchOne(t, c, "20261001", "labels", dir)
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("error = %v, want a SHA-256 mismatch", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("dir holds %v after a mismatch, want nothing", entries)
	}
}

func TestFetchShortBody(t *testing.T) {
	ds, c := newDumpServer(t)
	ds.handler = func(w http.ResponseWriter, r *http.Request, name string) bool {
		w.Header().Set("Content-Length", "100")
		fmt.Fprint(w, "october labels")
		return true
	}
	dir := t.TempDir()
	if _, err := fetchOne(t, c, "20261001", "labels", dir); err == nil {
		t.Error("Fetch of a short body succeeded, want error")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("dir holds %v after a short body, want nothing", entries)
	}
}

func TestFetchStall(t *testing.T) {
	ds, c := newDumpServer(t)
	c.stall = 50 * time.Millisecond
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	ds.handler = func(w http.ResponseWriter, r *http.Request, name string) bool {
		w.Header().Set("Content-Length", "100")
		fmt.Fprint(w, "part")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
		return true
	}
	dir := t.TempDir()
	_, err := fetchOne(t, c, "20261001", "labels", dir)
	if err == nil || !strings.Contains(err.Error(), "no data received") {
		t.Errorf("error = %v, want the stall named", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("dir holds %v after a stall, want nothing", entries)
	}
}

func TestFetchRefusesOtherHosts(t *testing.T) {
	ds, c := newDumpServer(t)
	f := File{Type: "labels", Name: "discogs_20261001_labels.xml.gz", URL: "https://evil.example/?download=x"}
	if _, err := c.Fetch(t.Context(), f, sha("x"), t.TempDir(), nil); err == nil {
		t.Error("Fetch from another host succeeded, want error")
	}
	f = File{Type: "labels", Name: "../escape.xml.gz", URL: c.downloadURL("data/2026/x")}
	if _, err := c.Fetch(t.Context(), f, sha("x"), t.TempDir(), nil); err == nil {
		t.Error("Fetch of a path-like name succeeded, want error")
	}
	if n := ds.downloads.Load(); n != 0 {
		t.Errorf("%d downloads were sent, want none", n)
	}
}
