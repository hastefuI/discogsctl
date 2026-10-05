package dump

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ErrNotFound is wrapped by the error Find and Latest return when no dump
// matches.
var ErrNotFound = errors.New("not found")

var (
	idPrefix  = regexp.MustCompile(`^\d{4,8}$`)
	dataName  = regexp.MustCompile(`^discogs_\d{8}_[a-z]+\.xml\.gz$`)
	sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

	errStalled = errors.New("no data received")
)

// Fetched is a dump file saved to disk.
type Fetched struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	// Skipped is true when the file was already at Path with the right
	// SHA-256, and was not downloaded again.
	Skipped bool `json:"skipped"`
}

// Find returns the dump whose ID is id, or the one dump whose ID starts with
// it, such as 202609 for 20260901. id must have at least the four digits of a
// year, since that year's listing is the one read. No match is an error
// wrapping ErrNotFound, and more than one is an error naming them.
func (c *Client) Find(ctx context.Context, id string) (Dump, error) {
	if !idPrefix.MatchString(id) {
		return Dump{}, fmt.Errorf("dump: invalid ID %q, want a date stamp such as 20261001, or the start of one with at least the year", id)
	}
	year, _ := strconv.Atoi(id[:4])
	dumps, err := c.List(ctx, year)
	if err != nil {
		return Dump{}, err
	}
	var ids []string
	var match Dump
	for _, d := range dumps {
		if strings.HasPrefix(d.ID, id) {
			ids = append(ids, d.ID)
			match = d
		}
	}
	switch len(ids) {
	case 0:
		return Dump{}, fmt.Errorf("dump: no dump with ID %s: %w", id, ErrNotFound)
	case 1:
		return match, nil
	}
	return Dump{}, fmt.Errorf("dump: ID %s matches %d dumps, give more digits: %s", id, len(ids), strings.Join(ids, ", "))
}

// Latest returns the newest dump with a CHECKSUM file and a file of every type
// in types, looking in the newest year with dumps and the year before. Discogs
// uploads the CHECKSUM file after the others, so a dump that has one is
// complete. No match is an error wrapping ErrNotFound.
func (c *Client) Latest(ctx context.Context, types ...string) (Dump, error) {
	years, err := c.Years(ctx)
	if err != nil {
		return Dump{}, err
	}
	for _, y := range slices.Backward(years[max(0, len(years)-2):]) {
		dumps, err := c.List(ctx, y)
		if err != nil {
			return Dump{}, err
		}
		for _, d := range dumps {
			if _, ok := d.Checksum(); ok && d.Has(types...) {
				return d, nil
			}
		}
	}
	return Dump{}, fmt.Errorf("dump: no recent dump with a checksum and %s: %w", cmp.Or(strings.Join(types, ", "), "any files"), ErrNotFound)
}

// Checksums returns the SHA-256 of each data file in d, in lowercase hex,
// keyed by file name, read from the dump's CHECKSUM file.
func (c *Client) Checksums(ctx context.Context, d Dump) (map[string]string, error) {
	body, err := c.ChecksumFile(ctx, d)
	if err != nil {
		return nil, err
	}
	return ParseChecksums(bytes.NewReader(body))
}

// ChecksumFile returns the CHECKSUM file of d as published, for saving beside
// the data files so Verify can check them later.
func (c *Client) ChecksumFile(ctx context.Context, d Dump) ([]byte, error) {
	f, ok := d.Checksum()
	if !ok {
		return nil, fmt.Errorf("dump: %s has no CHECKSUM file, so its files cannot be verified", d.ID)
	}
	return c.small(ctx, f.URL)
}

// ParseChecksums reads a CHECKSUM file, one "<sha256> <file name>" line per
// data file. A line in any other form is an error rather than skipped, so a
// file is never taken as verified against something that was not read.
func ParseChecksums(r io.Reader) (map[string]string, error) {
	sums := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return nil, fmt.Errorf("dump: unreadable CHECKSUM line %q", sc.Text())
		}
		sum, name := strings.ToLower(fields[0]), strings.TrimPrefix(fields[1], "*")
		if !sha256Hex.MatchString(sum) || !dataName.MatchString(name) {
			return nil, fmt.Errorf("dump: unreadable CHECKSUM line %q", sc.Text())
		}
		sums[name] = sum
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("dump: reading CHECKSUM: %w", err)
	}
	if len(sums) == 0 {
		return nil, errors.New("dump: CHECKSUM file lists no files")
	}
	return sums, nil
}

// VerifyFile returns an error unless the file at path has the SHA-256 sum,
// given in hex.
func VerifyFile(path, sum string) error {
	got, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if got != strings.ToLower(sum) {
		return fmt.Errorf("dump: %s has SHA-256 %s, want %s", path, got, sum)
	}
	return nil
}

// fileSHA256 returns the SHA-256 of the file at path in lowercase hex.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("dump: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("dump: reading %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Fetch downloads the data file f into dir under its published name. sum is
// its SHA-256 in hex, from Checksums.
//
// The download is written to a .part file beside the final path, which is
// renamed into place only when its SHA-256 matches sum, and deleted on a
// mismatch or any other failure, including cancellation. A file already at
// the final path with the right SHA-256 is kept and not downloaded again; one
// with the wrong SHA-256 is replaced. A download that receives nothing for
// StallTimeout is abandoned.
//
// progress, if not nil, is called as data arrives with the bytes received so
// far and the total, which is -1 when the server does not send a length.
func (c *Client) Fetch(ctx context.Context, f File, sum, dir string, progress func(done, total int64)) (Fetched, error) {
	if !dataName.MatchString(f.Name) {
		return Fetched{}, fmt.Errorf("dump: %q is not a dump data file name", f.Name)
	}
	sum = strings.ToLower(sum)
	if !sha256Hex.MatchString(sum) {
		return Fetched{}, fmt.Errorf("dump: invalid SHA-256 %q for %s", sum, f.Name)
	}
	result := Fetched{Type: f.Type, Name: f.Name, Path: filepath.Join(dir, f.Name), SHA256: sum}

	if _, err := os.Stat(result.Path); err == nil && VerifyFile(result.Path, sum) == nil {
		result.Skipped = true
		return result, nil
	}

	part := result.Path + ".part"
	got, err := c.download(ctx, f, part, progress)
	if err == nil && got != sum {
		err = fmt.Errorf("dump: %s has SHA-256 %s, want %s", f.Name, got, sum)
	}
	if err == nil {
		err = os.Rename(part, result.Path)
	}
	if err != nil {
		os.Remove(part)
		return Fetched{}, err
	}
	return result, nil
}

// download streams f into the file at path and returns its SHA-256 in hex.
func (c *Client) download(ctx context.Context, f File, path string, progress func(done, total int64)) (string, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	stall := time.AfterFunc(c.stall, func() { cancel(errStalled) })
	defer stall.Stop()

	resp, err := c.open(ctx, c.downloads, f.URL)
	if err != nil {
		return "", c.stalled(ctx, f, err)
	}
	defer resp.Body.Close()

	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", fmt.Errorf("dump: %w", err)
	}
	h := sha256.New()
	body := &watchdog{r: resp.Body, stall: stall, after: c.stall, total: resp.ContentLength, progress: progress}
	n, err := io.Copy(io.MultiWriter(out, h), body)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", c.stalled(ctx, f, err)
	}
	if resp.ContentLength >= 0 && n != resp.ContentLength {
		return "", fmt.Errorf("dump: %s ended after %d of %d bytes", f.Name, n, resp.ContentLength)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// stalled names the stall as the cause when the watchdog ended the download.
func (c *Client) stalled(ctx context.Context, f File, err error) error {
	if errors.Is(context.Cause(ctx), errStalled) {
		return fmt.Errorf("dump: %s: %w for %s", f.Name, errStalled, c.stall)
	}
	return fmt.Errorf("dump: downloading %s: %w", f.Name, err)
}

// watchdog resets the stall timer on every read that returns data, and
// reports progress.
type watchdog struct {
	r        io.Reader
	stall    *time.Timer
	after    time.Duration
	done     int64
	total    int64
	progress func(done, total int64)
}

func (w *watchdog) Read(p []byte) (int, error) {
	n, err := w.r.Read(p)
	if n > 0 {
		w.stall.Reset(w.after)
		w.done += int64(n)
		if w.progress != nil {
			w.progress(w.done, w.total)
		}
	}
	return n, err
}
