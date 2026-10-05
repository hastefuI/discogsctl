// Package dump reads the Discogs data dumps published at
// https://data.discogs.com: monthly XML exports of the artists, labels,
// masters and releases in the database, under the CC0 licence.
//
// Create a Client with New. Like the API, the dump host is told which
// application is calling, so Options.UserAgent is mandatory and New rejects an
// empty or generic one:
//
//	client, err := dump.New(dump.Options{
//		UserAgent: "myapp/0.1 (+https://example.com)",
//	})
//
// Discogs publishes no machine-readable index of the dumps. Years and List
// read the download links from the HTML listing at data.discogs.com and
// nothing else on the page, so a change to its layout does not change the
// result.
//
// Downloads from the dump host are not API requests, and the API rate limit
// does not apply to them.
package dump

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"go.hasteful.org/discogsctl/api"
	"go.hasteful.org/discogsctl/internal/useragent"
)

const (
	// DefaultBaseURL is the dump host used unless Options.BaseURL says
	// otherwise.
	DefaultBaseURL = "https://data.discogs.com/"
	// DefaultTimeout is the HTTP timeout used unless Options.HTTP says
	// otherwise. The listing pages are small, and the host has been seen to
	// hold a request open without answering.
	DefaultTimeout = 30 * time.Second

	maxPageBytes = 4 << 20
)

// Options configures New. UserAgent is required.
type Options struct {
	// BaseURL defaults to DefaultBaseURL.
	BaseURL string
	// UserAgent names the calling application, not this library, preferably
	// as product/version followed by a contact URL.
	UserAgent string
	// HTTP defaults to a client with DefaultTimeout.
	HTTP *http.Client
	// Logger receives one debug record per response. It defaults to
	// discarding them.
	Logger *slog.Logger
}

// Client reads the dump listing. A Client is safe for concurrent use.
type Client struct {
	base      *url.URL
	userAgent string
	http      *http.Client
	log       *slog.Logger
}

// New returns a Client, or an error if UserAgent is missing or generic, or
// BaseURL is not an absolute URL.
func New(opts Options) (*Client, error) {
	if err := useragent.Check(opts.UserAgent); err != nil {
		return nil, fmt.Errorf("dump: %w", err)
	}
	base, err := url.Parse(cmp.Or(opts.BaseURL, DefaultBaseURL))
	if err != nil {
		return nil, fmt.Errorf("dump: invalid BaseURL: %w", err)
	}
	if base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") {
		return nil, fmt.Errorf("dump: BaseURL %q must be an http or https URL", base.Redacted())
	}
	if base.Path == "" {
		base.Path = "/"
	}

	httpClient := opts.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Client{base: base, userAgent: opts.UserAgent, http: httpClient, log: logger}, nil
}

// page fetches the listing for prefix, which is empty for the root. A status
// of 400 or above is returned as an *api.Error, so callers map it to the same
// exit codes as an API failure.
func (c *Client) page(ctx context.Context, prefix string) ([]byte, error) {
	u := *c.base
	if prefix != "" {
		u.RawQuery = url.Values{"prefix": {prefix}}.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("dump: building request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dump: %w", err)
	}
	defer resp.Body.Close()
	c.log.DebugContext(ctx, "dump response", "method", req.Method, "url", u.String(), "status", resp.StatusCode)

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, &api.Error{StatusCode: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes))
	if err != nil {
		return nil, fmt.Errorf("dump: reading %s: %w", u.String(), err)
	}
	return body, nil
}
