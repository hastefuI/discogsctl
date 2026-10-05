package dump

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrChecksum is wrapped by the error Verify returns when a file's SHA-256 is
// not the one its CHECKSUM file lists.
var ErrChecksum = errors.New("SHA-256 does not match")

// Verified is the result of checking one dump file.
type Verified struct {
	Path string `json:"path"`
	// SHA256 is the file's own SHA-256, set whenever it could be read.
	SHA256 string `json:"sha256,omitzero"`
	OK     bool   `json:"ok"`
	// Error says why the file failed, and is empty when OK.
	Error string `json:"error,omitzero"`
}

// ChecksumName returns the published name of the CHECKSUM file that lists
// the data file name, such as discogs_20260801_CHECKSUM.txt for
// discogs_20260801_releases.xml.gz.
func ChecksumName(name string) (string, error) {
	if !dataName.MatchString(name) {
		return "", fmt.Errorf("dump: %q is not a dump data file name such as discogs_20260801_releases.xml.gz", name)
	}
	date, _, _ := strings.Cut(strings.TrimPrefix(name, "discogs_"), "_")
	return "discogs_" + date + "_CHECKSUM.txt", nil
}

// Verify checks the dump file at path against the SHA-256 its CHECKSUM file
// lists. checksums is the path of that file; empty means the one beside path
// under its published name. The file must keep its published name, which is
// how the CHECKSUM file lists it.
//
// The result names the file, and its SHA-256 whenever it could be read. The
// error says why it failed: an unreadable or unlisted CHECKSUM file, a file
// that cannot be read, or one that does not match, which wraps ErrChecksum.
func Verify(path, checksums string) (Verified, error) {
	v := Verified{Path: path}
	fail := func(err error) (Verified, error) {
		v.Error = err.Error()
		return v, err
	}

	name := filepath.Base(path)
	if checksums == "" {
		sumName, err := ChecksumName(name)
		if err != nil {
			return fail(err)
		}
		checksums = filepath.Join(filepath.Dir(path), sumName)
	}
	f, err := os.Open(checksums)
	if err != nil {
		return fail(fmt.Errorf("dump: no CHECKSUM file to verify %s against: %w", name, err))
	}
	sums, err := ParseChecksums(f)
	f.Close()
	if err != nil {
		return fail(err)
	}
	want, ok := sums[name]
	if !ok {
		return fail(fmt.Errorf("dump: %s does not list %s", checksums, name))
	}

	if v.SHA256, err = fileSHA256(path); err != nil {
		return fail(err)
	}
	if v.SHA256 != want {
		return fail(fmt.Errorf("dump: %s: %w: has %s, %s lists %s", name, ErrChecksum, v.SHA256, filepath.Base(checksums), want))
	}
	v.OK = true
	return v, nil
}
