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
// Fetch downloads a dump file and checks it against the SHA-256 in the dump's
// CHECKSUM file before it is given its published name.
//
// Downloads from the dump host are not API requests, and the API rate limit
// does not apply to them. The host limits requests on its own, so nothing here
// retries.
package dump

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.hasteful.org/discogsctl/api"
	"go.hasteful.org/discogsctl/internal/useragent"
)

const (
	// DefaultBaseURL is the dump host used unless Options.BaseURL says
	// otherwise.
	DefaultBaseURL = "https://data.discogs.com/"
	// DefaultTimeout is the HTTP timeout for the listing pages and checksum
	// files unless Options.HTTP says otherwise. They are small, and the host
	// has been seen to hold a request open without answering.
	DefaultTimeout = 30 * time.Second
	// StallTimeout ends a download that receives no data for this long. A
	// whole-request timeout would end a 10 GB download that is still
	// arriving.
	StallTimeout = 60 * time.Second

	maxPageBytes = 4 << 20
)

// Options configures New. UserAgent is required.
type Options struct {
	// BaseURL defaults to DefaultBaseURL.
	BaseURL string
	// UserAgent names the calling application, not this library, preferably
	// as product/version followed by a contact URL.
	UserAgent string
	// HTTP defaults to a client with DefaultTimeout for listings, and one
	// with no overall timeout for downloads, which end on StallTimeout
	// instead. A client given here is used for both, so its Timeout, if
	// any, also bounds each download.
	HTTP *http.Client
	// Logger receives one debug record per response. It defaults to
	// discarding them.
	Logger *slog.Logger
}

// Client reads the dump listing and downloads dump files. A Client is safe for
// concurrent use.
type Client struct {
	base      *url.URL
	userAgent string
	http      *http.Client
	downloads *http.Client
	stall     time.Duration
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

	httpClient, downloads := opts.HTTP, opts.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
		downloads = &http.Client{}
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Client{
		base:      base,
		userAgent: opts.UserAgent,
		http:      httpClient,
		downloads: downloads,
		stall:     StallTimeout,
		log:       logger,
	}, nil
}

// page fetches the listing for prefix, which is empty for the root.
func (c *Client) page(ctx context.Context, prefix string) ([]byte, error) {
	u := *c.base
	if prefix != "" {
		u.RawQuery = url.Values{"prefix": {prefix}}.Encode()
	}
	return c.small(ctx, u.String())
}

// small fetches a listing page or checksum file, up to maxPageBytes.
func (c *Client) small(ctx context.Context, rawURL string) ([]byte, error) {
	resp, err := c.open(ctx, c.http, rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes))
	if err != nil {
		return nil, fmt.Errorf("dump: reading %s: %w", rawURL, err)
	}
	return body, nil
}

// open sends a GET for rawURL, which must be on the client's own host. A
// status of 400 or above is returned as an *api.Error, so callers map it to
// the same exit codes as an API failure. The caller closes the body.
//
// It asks for the bytes as stored. Without that, Go's transport would ask for
// gzip and undo any gzip the server added, and a .xml.gz served that way
// would arrive unzipped and fail its checksum.
func (c *Client) open(ctx context.Context, hc *http.Client, rawURL string) (*http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("dump: invalid URL: %w", err)
	}
	if !strings.EqualFold(u.Host, c.base.Host) || u.Scheme != c.base.Scheme {
		return nil, fmt.Errorf("dump: refusing %s, which is not on %s", u.Redacted(), c.base.Redacted())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("dump: building request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dump: %w", err)
	}
	c.log.DebugContext(ctx, "dump response", "method", req.Method, "url", u.String(), "status", resp.StatusCode, "length", resp.ContentLength)
	if resp.StatusCode >= http.StatusBadRequest {
		resp.Body.Close()
		return nil, &api.Error{StatusCode: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
	}
	return resp, nil
}
