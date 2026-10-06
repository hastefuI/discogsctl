package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// authorizeURL is the Discogs page where a user approves a request token. It
// is on the website, not the API host.
const authorizeURL = "https://www.discogs.com/oauth/authorize"

// OAuth holds OAuth 1.0a credentials, as an alternative to Options.Token. The
// consumer key and secret identify an application registered at
// https://www.discogs.com/settings/developers. The access token and secret
// identify a user who authorized that application, and come from
// Client.OAuthAccessToken.
//
// With only the consumer key and secret, requests carry them as Discogs Auth,
// which gets 60 requests a minute and image URLs but acts as no user, and the
// client can run the authorization flow to get an access token.
type OAuth struct {
	ConsumerKey    string
	ConsumerSecret string
	AccessToken    string
	AccessSecret   string
}

// OAuthToken is a token and its secret. Client.OAuthRequestToken returns a
// temporary request token, and Client.OAuthAccessToken exchanges it for an
// access token, which goes in OAuth.AccessToken and OAuth.AccessSecret.
type OAuthToken struct {
	Token  string `json:"token"`
	Secret string `json:"secret"`
}

// AuthorizeURL returns the Discogs page where a user approves the request
// token t. Discogs then shows a verification code, or redirects to the
// application's callback URL with the code in its oauth_verifier parameter.
func (t OAuthToken) AuthorizeURL() string {
	return authorizeURL + "?" + url.Values{"oauth_token": {t.Token}}.Encode()
}

// oauthAuth signs a request with OAuth 1.0a in the Authorization header. It
// uses PLAINTEXT, the method the Discogs docs use, which sends the secrets
// rather than a digest of the request. That is safe only over TLS, and New
// refuses a base URL that is not https.
type oauthAuth struct {
	consumerKey    string
	consumerSecret string
	token          string
	tokenSecret    string
	// callback and verifier are only set on the requests of the flow.
	callback string
	verifier string
}

func (o oauthAuth) authorize(req *http.Request) {
	params := map[string]string{
		"oauth_consumer_key":     o.consumerKey,
		"oauth_nonce":            rand.Text(),
		"oauth_signature":        oauthEscape(o.consumerSecret) + "&" + oauthEscape(o.tokenSecret),
		"oauth_signature_method": "PLAINTEXT",
		"oauth_timestamp":        strconv.FormatInt(time.Now().Unix(), 10),
		"oauth_version":          "1.0",
	}
	for k, v := range map[string]string{"oauth_token": o.token, "oauth_callback": o.callback, "oauth_verifier": o.verifier} {
		if v != "" {
			params[k] = v
		}
	}
	pairs := make([]string, 0, len(params))
	for k, v := range params {
		pairs = append(pairs, oauthEscape(k)+`="`+oauthEscape(v)+`"`)
	}
	slices.Sort(pairs)
	req.Header.Set("Authorization", "OAuth "+strings.Join(pairs, ", "))
}

func (oauthAuth) highTier() bool { return true }
func (o oauthAuth) asUser() bool { return o.token != "" }

// oauthFlow signs the requests that get a request token and an access token,
// which Discogs documents as form requests.
type oauthFlow struct{ oauthAuth }

func (f oauthFlow) authorize(req *http.Request) {
	f.oauthAuth.authorize(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
}

// oauthEscape percent-encodes s as RFC 5849 section 3.6 requires: everything
// but letters, digits and -._~ is escaped, with upper case hex.
func oauthEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || strings.IndexByte("-._~", c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// errNoConsumer is returned by the flow when the client has no consumer key.
var errNoConsumer = errors.New("api: the OAuth flow needs OAuth.ConsumerKey and OAuth.ConsumerSecret")

// OAuthRequestToken starts the OAuth flow. It returns a request token for the
// user to approve at its AuthorizeURL. callback is where Discogs sends the
// user afterwards; empty leaves it to the application's settings, and with no
// callback URL there Discogs shows the user a verification code instead.
func (c *Client) OAuthRequestToken(ctx context.Context, callback string) (*OAuthToken, error) {
	if c.consumer.consumerKey == "" {
		return nil, errNoConsumer
	}
	u, err := c.endpoint("oauth", "request_token")
	if err != nil {
		return nil, err
	}
	a := c.consumer
	a.callback = callback
	return c.oauthToken(ctx, oauthFlow{a}, http.MethodGet, u)
}

// OAuthAccessToken finishes the OAuth flow. It exchanges the request token,
// once the user has approved it, and the verification code Discogs gave them
// for an access token that acts as that user.
func (c *Client) OAuthAccessToken(ctx context.Context, request OAuthToken, verifier string) (*OAuthToken, error) {
	if c.consumer.consumerKey == "" {
		return nil, errNoConsumer
	}
	if request.Token == "" || request.Secret == "" {
		return nil, errors.New("api: the request token and its secret are required")
	}
	if verifier == "" || malformed(verifier) {
		return nil, errors.New("api: the verification code is empty or contains whitespace")
	}
	u, err := c.endpoint("oauth", "access_token")
	if err != nil {
		return nil, err
	}
	a := c.consumer
	a.token, a.tokenSecret, a.verifier = request.Token, request.Secret, verifier
	return c.oauthToken(ctx, oauthFlow{a}, http.MethodPost, u)
}

// oauthToken sends one request of the flow and reads the token and secret
// from the form encoded body. The body is never put in an error, since it
// holds the secret.
func (c *Client) oauthToken(ctx context.Context, a auth, method string, u *url.URL) (*OAuthToken, error) {
	body, err := c.call(ctx, a, method, u, nil)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errEmptyBody
	}
	values, err := url.ParseQuery(string(bytes.TrimSpace(body)))
	if err != nil {
		return nil, fmt.Errorf("discogs: decoding %s: malformed form body", u.Path)
	}
	t := &OAuthToken{Token: values.Get("oauth_token"), Secret: values.Get("oauth_token_secret")}
	if t.Token == "" || t.Secret == "" {
		return nil, fmt.Errorf("discogs: %s returned no oauth_token and oauth_token_secret", u.Path)
	}
	return t, nil
}
