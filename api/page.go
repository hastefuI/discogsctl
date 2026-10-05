package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// MaxPerPage is the largest page Discogs serves.
const MaxPerPage = 100

// Page selects one page of a paginated listing. A zero field leaves the
// choice to Discogs, which serves page 1 with 50 items.
type Page struct {
	Page    int
	PerPage int
}

func (p Page) apply(q url.Values) error {
	if p.Page < 0 {
		return fmt.Errorf("api: page %d must be 1 or more", p.Page)
	}
	if p.PerPage < 0 || p.PerPage > MaxPerPage {
		return fmt.Errorf("api: per page %d must be between 1 and %d", p.PerPage, MaxPerPage)
	}
	if p.Page > 0 {
		q.Set("page", strconv.Itoa(p.Page))
	}
	if p.PerPage > 0 {
		q.Set("per_page", strconv.Itoa(p.PerPage))
	}
	return nil
}

// Pagination is the pagination object on a paginated response.
type Pagination struct {
	Page    int      `json:"page"`
	Pages   int      `json:"pages"`
	PerPage int      `json:"per_page"`
	Items   int      `json:"items"`
	URLs    PageURLs `json:"urls"`
}

// PageURLs links to the other pages of a listing. A link is empty when there
// is no such page.
type PageURLs struct {
	First string `json:"first,omitzero"`
	Prev  string `json:"prev,omitzero"`
	Next  string `json:"next,omitzero"`
	Last  string `json:"last,omitzero"`
}

// Paginated is one page of a listing. Next and Each fetch the pages after it
// through the same client and rate limiter.
type Paginated[T any] struct {
	Pagination Pagination
	Items      []T

	client *Client
	key    string
}

// getPage fetches u and decodes the pagination object and the array under key,
// which names the items: "releases", "results", "versions" or "wants".
func getPage[T any](ctx context.Context, c *Client, u *url.URL, key string) (*Paginated[T], error) {
	var body map[string]json.RawMessage
	if err := c.get(ctx, u, &body); err != nil {
		return nil, err
	}

	p := &Paginated[T]{client: c, key: key}
	if raw, ok := body["pagination"]; ok {
		if err := json.Unmarshal(raw, &p.Pagination); err != nil {
			return nil, fmt.Errorf("discogs: decoding pagination: %w", err)
		}
	}
	raw, ok := body[key]
	if !ok {
		return nil, fmt.Errorf("discogs: response from %s has no %q field", u.Path, key)
	}
	if err := json.Unmarshal(raw, &p.Items); err != nil {
		return nil, fmt.Errorf("discogs: decoding %s: %w", key, err)
	}
	if p.Items == nil {
		p.Items = []T{}
	}

	c.log.DebugContext(ctx, "discogs page",
		"page", p.Pagination.Page,
		"pages", p.Pagination.Pages,
		"per_page", p.Pagination.PerPage,
		"items", p.Pagination.Items,
	)
	return p, nil
}

// Next fetches the page after p, following pagination.urls.next. It returns
// nil and no error when p is the last page.
func (p *Paginated[T]) Next(ctx context.Context) (*Paginated[T], error) {
	if p.Pagination.URLs.Next == "" {
		return nil, nil
	}
	u, err := p.client.follow(p.Pagination.URLs.Next)
	if err != nil {
		return nil, err
	}
	return getPage[T](ctx, p.client, u, p.key)
}

// Each calls fn for every item on p and on every page after it, stopping at
// the first error from fn or from a fetch.
func (p *Paginated[T]) Each(ctx context.Context, fn func(T) error) error {
	for page := p; page != nil; {
		for _, item := range page.Items {
			if err := fn(item); err != nil {
				return err
			}
		}
		next, err := page.Next(ctx)
		if err != nil {
			return err
		}
		page = next
	}
	return nil
}

// sortValues checks sort against sorts and order against asc and desc, and
// sets each that is not empty on v. what names the listing in an error.
func sortValues(v url.Values, what, sort, order string, sorts []string) error {
	if sort != "" && !slices.Contains(sorts, sort) {
		return fmt.Errorf("api: %s sort %q must be one of %s", what, sort, strings.Join(sorts, ", "))
	}
	if order != "" && order != "asc" && order != "desc" {
		return fmt.Errorf("api: sort order %q must be asc or desc", order)
	}
	if sort != "" {
		v.Set("sort", sort)
	}
	if order != "" {
		v.Set("sort_order", order)
	}
	return nil
}
