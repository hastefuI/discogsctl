package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
)

// SearchTypes are the values Discogs accepts for SearchQuery.Type.
var SearchTypes = []string{"release", "master", "artist", "label"}

// SearchQuery is a database search. Every field is optional, and an empty
// field is left out of the request. Title searches the combined
// "Artist - Release Title" field; ReleaseTitle searches release titles only.
type SearchQuery struct {
	Query        string
	Type         string
	Title        string
	ReleaseTitle string
	Credit       string
	Artist       string
	ANV          string
	Label        string
	Genre        string
	Style        string
	Country      string
	Year         string
	Format       string
	Catno        string
	Barcode      string
	Track        string
	Submitter    string
	Contributor  string

	Page
}

func (q SearchQuery) values() (url.Values, error) {
	if q.Type != "" && !slices.Contains(SearchTypes, q.Type) {
		return nil, fmt.Errorf("api: search type %q must be one of %v", q.Type, SearchTypes)
	}
	v := url.Values{}
	for name, value := range map[string]string{
		"q":             q.Query,
		"type":          q.Type,
		"title":         q.Title,
		"release_title": q.ReleaseTitle,
		"credit":        q.Credit,
		"artist":        q.Artist,
		"anv":           q.ANV,
		"label":         q.Label,
		"genre":         q.Genre,
		"style":         q.Style,
		"country":       q.Country,
		"year":          q.Year,
		"format":        q.Format,
		"catno":         q.Catno,
		"barcode":       q.Barcode,
		"track":         q.Track,
		"submitter":     q.Submitter,
		"contributor":   q.Contributor,
	} {
		if value != "" {
			v.Set(name, value)
		}
	}
	if err := q.Page.apply(v); err != nil {
		return nil, err
	}
	return v, nil
}

// SearchResult is one hit from a database search. Type says which resource ID
// refers to. Year is a string, and is empty when Discogs has none.
type SearchResult struct {
	ID          int      `json:"id"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Year        string   `json:"year"`
	Country     string   `json:"country"`
	Format      []string `json:"format"`
	Label       []string `json:"label"`
	Catno       string   `json:"catno"`
	Genre       []string `json:"genre"`
	Style       []string `json:"style"`
	MasterID    int      `json:"master_id"`
	URI         string   `json:"uri"`
	ResourceURL string   `json:"resource_url"`

	raw json.RawMessage
}

type searchResult SearchResult

func (r *SearchResult) UnmarshalJSON(b []byte) error {
	return decodeKeep(b, (*searchResult)(r), &r.raw)
}
func (r SearchResult) MarshalJSON() ([]byte, error) { return encodeKept(r.raw, searchResult(r)) }

// Search returns one page of database search results. The Discogs docs say
// search needs authentication, though it has also answered without a token.
func (c *Client) Search(ctx context.Context, q SearchQuery) (*Paginated[SearchResult], error) {
	u, err := c.endpoint("database", "search")
	if err != nil {
		return nil, err
	}
	v, err := q.values()
	if err != nil {
		return nil, err
	}
	u.RawQuery = v.Encode()
	return getPage[SearchResult](ctx, c, u, "results")
}
