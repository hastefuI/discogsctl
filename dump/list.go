package dump

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"time"
)

// TypeChecksum is the File.Type of the file that lists the SHA-256 of every
// other file in a dump.
const TypeChecksum = "CHECKSUM"

// DataTypes are the kinds of data Discogs publishes a file of. Not every dump
// has all of them.
var DataTypes = []string{"artists", "labels", "masters", "releases"}

// File is one file in a dump.
type File struct {
	// Type is the kind of data, such as "artists", "labels", "masters" or
	// "releases", or TypeChecksum.
	Type string `json:"type"`
	// Name is the file name Discogs publishes it under, such as
	// discogs_20260801_releases.xml.gz.
	Name string `json:"name"`
	URL  string `json:"url"`
	// Size and Modified are as the listing page shows them, such as "10.5 GB"
	// and "2026-10-01 19:35:58". The size is rounded and the page does not
	// state a time zone. Both are empty when the page row could not be read.
	Size     string `json:"size,omitzero"`
	Modified string `json:"modified,omitzero"`
}

// Dump is the set of files Discogs published under one date.
type Dump struct {
	// ID is the date stamp in the dump's file names, such as 20261001. No two
	// dumps share one, so it is how a dump is referred to on the command
	// line.
	ID string `json:"id"`
	// Date is the ID as YYYY-MM-DD. Recent dumps are dated the first of a
	// month; some early ones are not.
	Date  string `json:"date"`
	Files []File `json:"files"`
}

// Types returns the data types in d, sorted, leaving out the checksum file.
// Not every dump has every type.
func (d Dump) Types() []string {
	var types []string
	for _, f := range d.Files {
		if f.Type != TypeChecksum {
			types = append(types, f.Type)
		}
	}
	slices.Sort(types)
	return slices.Compact(types)
}

// Has reports whether d has a file of every type in types.
func (d Dump) Has(types ...string) bool {
	have := d.Types()
	for _, t := range types {
		if !slices.Contains(have, t) {
			return false
		}
	}
	return true
}

// Only returns d with just its files of types, and its checksum file, which
// covers them.
func (d Dump) Only(types ...string) Dump {
	d.Files = slices.DeleteFunc(slices.Clone(d.Files), func(f File) bool {
		return f.Type != TypeChecksum && !slices.Contains(types, f.Type)
	})
	return d
}

// Checksum returns the checksum file of d, if Discogs published one.
func (d Dump) Checksum() (File, bool) {
	i := slices.IndexFunc(d.Files, func(f File) bool { return f.Type == TypeChecksum })
	if i < 0 {
		return File{}, false
	}
	return d.Files[i], true
}

// The listing links to a year as ?prefix=data/2026/ and to a file as
// ?download=data/2026/discogs_20260801_releases.xml.gz. Only links of those
// two shapes are read, and a file name must match fileKey exactly, so nothing
// else on the page can name a file or a host.
var (
	href = regexp.MustCompile(`href="([^"]*)"`)
	// row is a line of the listing: modified time, size, then the link.
	row     = regexp.MustCompile(`^\s*(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\s+(\d+(?:\.\d+)? [KMGT]?B)\s+<a\s`)
	yearKey = regexp.MustCompile(`^data/(\d{4})/$`)
	fileKey = regexp.MustCompile(`^data/\d{4}/(discogs_(\d{8})_(?:([a-z]+)\.xml\.gz|CHECKSUM\.txt))$`)
)

// link returns the value of query parameter param in an href attribute value,
// or "" when the link has none or points anywhere but the listing itself.
func link(attr, param string) string {
	u, err := url.Parse(html.UnescapeString(attr))
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path != "" {
		return ""
	}
	return u.Query().Get(param)
}

// Years returns the years that have a folder of dumps, oldest first.
func (c *Client) Years(ctx context.Context) ([]int, error) {
	body, err := c.page(ctx, "")
	if err != nil {
		return nil, err
	}
	var years []int
	for _, m := range href.FindAllSubmatch(body, -1) {
		k := yearKey.FindStringSubmatch(link(string(m[1]), "prefix"))
		if k == nil {
			continue
		}
		y, _ := strconv.Atoi(k[1])
		years = append(years, y)
	}
	if len(years) == 0 {
		return nil, fmt.Errorf("dump: no years listed at %s; the listing page may have changed", c.base)
	}
	slices.Sort(years)
	return slices.Compact(years), nil
}

// List returns the dumps published in year, newest first, each with its files
// sorted by name. A year with no dumps gives an empty slice.
func (c *Client) List(ctx context.Context, year int) ([]Dump, error) {
	if year < 1 || year > 9999 {
		return nil, fmt.Errorf("dump: invalid year %d", year)
	}
	body, err := c.page(ctx, fmt.Sprintf("data/%04d/", year))
	if err != nil {
		return nil, err
	}

	byDate := map[string]*Dump{}
	seen := map[string]bool{}
	for line := range bytes.Lines(body) {
		for _, m := range href.FindAllSubmatch(line, -1) {
			key := link(string(m[1]), "download")
			k := fileKey.FindStringSubmatch(key)
			if k == nil || seen[k[1]] {
				continue
			}
			date, err := time.Parse("20060102", k[2])
			if err != nil {
				continue
			}
			seen[k[1]] = true

			f := File{
				Type: cmp.Or(k[3], TypeChecksum),
				Name: k[1],
				URL:  c.download(key),
			}
			if r := row.FindSubmatch(line); r != nil {
				f.Modified, f.Size = string(r[1]), string(r[2])
			}

			d := date.Format(time.DateOnly)
			if byDate[d] == nil {
				byDate[d] = &Dump{ID: k[2], Date: d}
			}
			byDate[d].Files = append(byDate[d].Files, f)
		}
	}

	dumps := make([]Dump, 0, len(byDate))
	for _, d := range byDate {
		slices.SortFunc(d.Files, func(a, b File) int { return cmp.Compare(a.Name, b.Name) })
		dumps = append(dumps, *d)
	}
	slices.SortFunc(dumps, func(a, b Dump) int { return cmp.Compare(b.Date, a.Date) })
	return dumps, nil
}

// download returns the URL that downloads key from the client's own host.
func (c *Client) download(key string) string {
	u := *c.base
	u.RawQuery = url.Values{"download": {key}}.Encode()
	return u.String()
}
