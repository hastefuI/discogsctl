package api

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"

	"go.hasteful.org/discogsctl/internal/useragent"
)

const (
	// DefaultBaseURL is the host used unless Options.BaseURL says otherwise.
	DefaultBaseURL = "https://api.discogs.com"
	// DefaultTimeout is the HTTP timeout used unless Options.HTTP says otherwise.
	DefaultTimeout = 30 * time.Second
	// MediaType is sent as the Accept header on every request. It pins API v2
	// and leaves text fields in Discogs markup.
	MediaType = "application/vnd.discogs.v2.discogs+json"

	maxBodyBytes = 32 << 20
)

// Currencies are the values Discogs accepts for Options.Currency.
var Currencies = []string{"USD", "GBP", "EUR", "CAD", "AUD", "JPY", "CHF", "MXN", "BRL", "NZD", "SEK", "ZAR"}

// Options configures New. UserAgent is required.
type Options struct {
	// BaseURL defaults to DefaultBaseURL. It must use https, except for a
	// loopback host, so a token is never sent in cleartext.
	BaseURL string
	// UserAgent names the calling application, not this library, preferably
	// as product/version followed by a contact URL. Discogs answers a missing
	// or generic agent with an empty body, so New rejects one.
	UserAgent string
	// Token is a personal access token from
	// https://www.discogs.com/settings/developers. It is optional. Without
	// one, requests are unauthenticated: 25 a minute and no image URLs.
	Token string
	// Currency is sent as curr_abbr on the requests that accept it, and must
	// be one of Currencies. Empty leaves it to Discogs, which uses the
	// authenticated user's currency.
	Currency string
	// HTTP defaults to a client with DefaultTimeout.
	HTTP *http.Client
	// Logger receives one debug record per response, with the rate limit
	// headers, and one per rate limit wait. It defaults to discarding them.
	// Credentials are never logged.
	Logger *slog.Logger
}

// Client calls the Discogs API. Create one with New. A Client is safe for
// concurrent use, and every call shares one rate limiter.
type Client struct {
	base      *url.URL
	userAgent string
	auth      auth
	currency  string
	http      *http.Client
	limit     *limiter
	log       *slog.Logger
}

// auth sets credentials on an outgoing request. It is the one place a new
// scheme, such as OAuth 1.0a, plugs in.
type auth interface {
	authorize(req *http.Request)
	authenticated() bool
}

type noAuth struct{}

func (noAuth) authorize(*http.Request) {}
func (noAuth) authenticated() bool     { return false }

// tokenAuth sends a personal access token in the Authorization header. Discogs
// also accepts the token as a query parameter, which this client never uses,
// so the token stays out of URLs and logs.
type tokenAuth string

func (t tokenAuth) authorize(req *http.Request) {
	req.Header.Set("Authorization", "Discogs token="+string(t))
}
func (tokenAuth) authenticated() bool { return true }

// New returns a Client, or an error if UserAgent is missing or generic, the
// base URL is not https, the token is malformed, or Currency is not one of
// Currencies.
func New(opts Options) (*Client, error) {
	if err := useragent.Check(opts.UserAgent); err != nil {
		return nil, fmt.Errorf("api: %w", err)
	}

	base, err := url.Parse(cmp.Or(opts.BaseURL, DefaultBaseURL))
	if err != nil {
		return nil, fmt.Errorf("api: invalid BaseURL: %w", err)
	}
	if base.Host == "" || (base.Scheme != "https" && !(base.Scheme == "http" && isLoopback(base.Hostname()))) {
		return nil, fmt.Errorf("api: BaseURL %q must be an https URL", base.Redacted())
	}
	if base.Path == "" {
		base.Path = "/"
	}

	var a auth = noAuth{}
	if opts.Token != "" {
		if strings.ContainsFunc(opts.Token, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
			return nil, errors.New("api: Token contains whitespace or control characters")
		}
		a = tokenAuth(opts.Token)
	}

	currency := strings.ToUpper(opts.Currency)
	if currency != "" && !slices.Contains(Currencies, currency) {
		return nil, fmt.Errorf("api: unsupported currency %q, want one of %s", opts.Currency, strings.Join(Currencies, ", "))
	}

	httpClient := opts.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	return &Client{
		base:      base,
		userAgent: opts.UserAgent,
		auth:      a,
		currency:  currency,
		http:      httpClient,
		limit:     newLimiter(a.authenticated(), logger),
		log:       logger,
	}, nil
}

// RateLimit returns the rate limit Discogs reported on the most recent response
// that carried the X-Discogs-Ratelimit headers. It sends no request, and is
// the zero value until a response arrives. The client already throttles
// itself to stay within the limit; this is for showing the budget or deciding
// when to start a long walk.
func (c *Client) RateLimit() RateLimit {
	return c.limit.report()
}

// Authenticated reports whether the client sends credentials.
func (c *Client) Authenticated() bool {
	return c.auth.authenticated()
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}

// endpoint returns the URL for a path made of segments, escaping each one. It
// rejects empty and dot segments so a username cannot walk to another route.
func (c *Client) endpoint(segments ...string) (*url.URL, error) {
	escaped := make([]string, len(segments))
	for i, s := range segments {
		if s == "" || s == "." || s == ".." {
			return nil, fmt.Errorf("api: invalid path segment %q", s)
		}
		escaped[i] = url.PathEscape(s)
	}
	return c.base.JoinPath(escaped...), nil
}

// follow returns the URL of a pagination link on the client's own scheme and
// host. Discogs has sent links with an http scheme, and following one as given
// would send the token in cleartext.
func (c *Client) follow(link string) (*url.URL, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("api: invalid pagination link: %w", err)
	}
	if !strings.EqualFold(u.Host, c.base.Host) {
		return nil, fmt.Errorf("api: pagination link host %q is not %q", u.Host, c.base.Host)
	}
	u.Scheme, u.Host, u.User = c.base.Scheme, c.base.Host, nil
	return u, nil
}

// get fetches u and decodes the JSON body into v.
func (c *Client) get(ctx context.Context, u *url.URL, v any) error {
	return c.do(ctx, http.MethodGet, u, nil, v)
}

// do sends method to u, with in encoded as a JSON body unless it is nil, and
// decodes the JSON response into out, or expects no body when out is nil, as
// for a DELETE. A 429 is retried once, after the limiter has waited out the
// window, and returned if it persists. Retrying a write is safe because
// Discogs did not act on a request it rate limited.
func (c *Client) do(ctx context.Context, method string, u *url.URL, in, out any) error {
	var payload []byte
	if in != nil {
		var err error
		if payload, err = json.Marshal(in); err != nil {
			return fmt.Errorf("api: encoding request: %w", err)
		}
	}
	status, body, err := c.send(ctx, method, u, payload)
	if err == nil && status == http.StatusTooManyRequests {
		c.limit.backoff(time.Now())
		status, body, err = c.send(ctx, method, u, payload)
	}
	if err != nil {
		return err
	}
	if status >= http.StatusBadRequest {
		return newError(status, body)
	}
	if out == nil {
		return nil
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return errEmptyBody
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("discogs: decoding %s: %w", u.Path, err)
	}
	return nil
}

// send is the transport. Every request waits on the limiter and carries the
// User-Agent, the Accept header and the credentials. A non-nil payload is sent
// as a JSON body.
func (c *Client) send(ctx context.Context, method string, u *url.URL, payload []byte) (int, []byte, error) {
	if c.userAgent == "" {
		return 0, nil, errors.New("api: refusing to send a request without a User-Agent")
	}
	if err := c.limit.wait(ctx); err != nil {
		return 0, nil, err
	}

	var reqBody io.Reader
	if payload != nil {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reqBody)
	if err != nil {
		return 0, nil, fmt.Errorf("api: building request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", MediaType)
	c.auth.authorize(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("discogs: %w", err)
	}
	defer resp.Body.Close()

	c.limit.observe(resp.Header, time.Now())
	c.log.DebugContext(ctx, "discogs response",
		"method", req.Method,
		"path", u.RequestURI(),
		"status", resp.StatusCode,
		"ratelimit", resp.Header.Get("X-Discogs-Ratelimit"),
		"used", resp.Header.Get("X-Discogs-Ratelimit-Used"),
		"remaining", resp.Header.Get("X-Discogs-Ratelimit-Remaining"),
	)

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return 0, nil, fmt.Errorf("discogs: reading response: %w", err)
	}
	return resp.StatusCode, body, nil
}
