package dump

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDump writes data files and a CHECKSUM file for dump 20260801 into a
// new directory. sums overrides the SHA-256 the CHECKSUM file lists for a
// name, and a name mapped to "" is left out of it.
func writeDump(t *testing.T, files, sums map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	var list strings.Builder
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		sum, ok := sums[name]
		if !ok {
			sum = sha(body)
		}
		if sum != "" {
			fmt.Fprintf(&list, "%s %s\n", sum, name)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "discogs_20260801_CHECKSUM.txt"), []byte(list.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestChecksumName(t *testing.T) {
	if got, err := ChecksumName("discogs_20260801_releases.xml.gz"); err != nil || got != "discogs_20260801_CHECKSUM.txt" {
		t.Errorf("ChecksumName = %q, %v", got, err)
	}
	for _, bad := range []string{"releases.xml.gz", "discogs_20260801_CHECKSUM.txt", "discogs_2026_releases.xml.gz"} {
		if _, err := ChecksumName(bad); err == nil {
			t.Errorf("ChecksumName(%q) succeeded, want error", bad)
		}
	}
}

func TestVerify(t *testing.T) {
	dir := writeDump(t, map[string]string{
		"discogs_20260801_labels.xml.gz":   "labels",
		"discogs_20260801_masters.xml.gz":  "masters",
		"discogs_20260801_releases.xml.gz": "releases",
	}, map[string]string{
		"discogs_20260801_masters.xml.gz":  sha("something else"),
		"discogs_20260801_releases.xml.gz": "",
	})
	path := func(name string) string { return filepath.Join(dir, name) }

	v, err := Verify(path("discogs_20260801_labels.xml.gz"), "")
	if err != nil || !v.OK || v.SHA256 != sha("labels") || v.Error != "" {
		t.Errorf("matching file = %+v, %v", v, err)
	}

	v, err = Verify(path("discogs_20260801_masters.xml.gz"), "")
	if !errors.Is(err, ErrChecksum) || v.OK || v.SHA256 != sha("masters") || v.Error == "" {
		t.Errorf("mismatched file = %+v, %v; want ErrChecksum with the file's own SHA-256", v, err)
	}

	if v, err := Verify(path("discogs_20260801_releases.xml.gz"), ""); err == nil || !strings.Contains(err.Error(), "does not list") || v.OK {
		t.Errorf("unlisted file = %+v, %v", v, err)
	}

	other := t.TempDir()
	moved := filepath.Join(other, "discogs_20260801_labels.xml.gz")
	os.WriteFile(moved, []byte("labels"), 0o644)
	if _, err := Verify(moved, ""); err == nil || !strings.Contains(err.Error(), "no CHECKSUM file") {
		t.Errorf("file without a CHECKSUM beside it = %v", err)
	}
	if v, err := Verify(moved, path("discogs_20260801_CHECKSUM.txt")); err != nil || !v.OK {
		t.Errorf("file with --checksum = %+v, %v", v, err)
	}

	renamed := filepath.Join(dir, "labels.xml.gz")
	os.WriteFile(renamed, []byte("labels"), 0o644)
	if _, err := Verify(renamed, ""); err == nil || !strings.Contains(err.Error(), "not a dump data file name") {
		t.Errorf("renamed file = %v", err)
	}
	if _, err := Verify(path("discogs_20260801_missing.xml.gz"), path("discogs_20260801_CHECKSUM.txt")); err == nil {
		t.Error("a file that does not exist verified")
	}
}
