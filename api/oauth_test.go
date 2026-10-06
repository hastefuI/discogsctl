package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// oauthParams parses an OAuth Authorization header into its unescaped
// parameters, or fails the test if it is not one.
func oauthParams(t *testing.T, header string) map[string]string {
	t.Helper()
	rest, ok := strings.CutPrefix(header, "OAuth ")
	if !ok {
		t.Fatalf("Authorization = %q, want an OAuth header", header)
	}
	params := map[string]string{}
	for pair := range strings.SplitSeq(rest, ", ") {
		k, quoted, ok := strings.Cut(pair, "=")
		v, err := url.PathUnescape(strings.Trim(quoted, `"`))
		if !ok || err != nil || !strings.HasPrefix(quoted, `"`) || !strings.HasSuffix(quoted, `"`) {
			t.Fatalf("malformed parameter %q in %q", pair, header)
		}
		params[k] = v
	}
	return params
}

func TestNewOAuthCredentials(t *testing.T) {
	full := OAuth{ConsumerKey: "ck", ConsumerSecret: "csecret-1", AccessToken: "at", AccessSecret: "asecret-2"}
	tests := []struct {
		name     string
		token    string
		oauth    OAuth
		wantErr  bool
		wantAuth bool
	}{
		{"none", "", OAuth{}, false, false},
		{"consumer only", "", OAuth{ConsumerKey: "ck", ConsumerSecret: "csecret-1"}, false, false},
		{"consumer and access token", "", full, false, true},
		{"personal token and consumer", "pat", OAuth{ConsumerKey: "ck", ConsumerSecret: "csecret-1"}, false, true},
		{"personal token and access token", "pat", full, true, false},
		{"key without secret", "", OAuth{ConsumerKey: "ck"}, true, false},
		{"access token without secret", "", OAuth{ConsumerKey: "ck", ConsumerSecret: "csecret-1", AccessToken: "at"}, true, false},
		{"access token without consumer", "", OAuth{AccessToken: "at", AccessSecret: "asecret-2"}, true, false},
		{"secret with a newline", "", OAuth{ConsumerKey: "ck", ConsumerSecret: "c\nsecret-3"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(Options{UserAgent: testAgent, Token: tt.token, OAuth: tt.oauth})
			if (err != nil) != tt.wantErr {
				t.Fatalf("New error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				for _, secret := range []string{"csecret-1", "asecret-2", "secret-3"} {
					if strings.Contains(err.Error(), secret) {
						t.Errorf("error %q contains a secret", err)
					}
				}
				return
			}
			if c.Authenticated() != tt.wantAuth {
				t.Errorf("Authenticated = %v, want %v", c.Authenticated(), tt.wantAuth)
			}
		})
	}
}

func TestOAuthSignsRequests(t *testing.T) {
	var headers []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = append(headers, r.Header.Get("Authorization"))
		if r.URL.RawQuery != "" {
			t.Errorf("query = %q, want none: credentials belong in the header", r.URL.RawQuery)
		}
		w.Write([]byte(`{"id": 1}`))
	}))
	defer srv.Close()

	c, err := New(Options{BaseURL: srv.URL, UserAgent: testAgent, HTTP: srv.Client(), OAuth: OAuth{
		ConsumerKey: "ck", ConsumerSecret: "c/s", AccessToken: "at", AccessSecret: "a&s",
	}})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := c.Release(t.Context(), 1); err != nil {
			t.Fatal(err)
		}
	}

	first, second := oauthParams(t, headers[0]), oauthParams(t, headers[1])
	want := map[string]string{
		"oauth_consumer_key":     "ck",
		"oauth_token":            "at",
		"oauth_signature":        "c%2Fs&a%26s",
		"oauth_signature_method": "PLAINTEXT",
		"oauth_version":          "1.0",
	}
	for k, v := range want {
		if first[k] != v {
			t.Errorf("%s = %q, want %q", k, first[k], v)
		}
	}
	if _, err := strconv.ParseInt(first["oauth_timestamp"], 10, 64); err != nil {
		t.Errorf("oauth_timestamp = %q, want seconds", first["oauth_timestamp"])
	}
	if first["oauth_nonce"] == "" || first["oauth_nonce"] == second["oauth_nonce"] {
		t.Errorf("nonces %q and %q, want two different ones", first["oauth_nonce"], second["oauth_nonce"])
	}
	for _, k := range []string{"oauth_callback", "oauth_verifier"} {
		if _, ok := first[k]; ok {
			t.Errorf("%s sent outside the flow", k)
		}
	}
}

func TestOAuthFlow(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("%s Content-Type = %q, want a form", r.URL.Path, ct)
		}
		p := oauthParams(t, r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/oauth/request_token":
			if p["oauth_consumer_key"] != "ck" || p["oauth_signature"] != "cs&" || p["oauth_callback"] != "https://example.com/cb" {
				t.Errorf("request token params = %v", p)
			}
			if _, ok := p["oauth_token"]; ok {
				t.Error("request token request carries an oauth_token")
			}
			fmt.Fprint(w, "oauth_token=rt&oauth_token_secret=rts&oauth_callback_confirmed=true")
		case "/oauth/access_token":
			if p["oauth_token"] != "rt" || p["oauth_signature"] != "cs&rts" || p["oauth_verifier"] != "v123" {
				t.Errorf("access token params = %v", p)
			}
			fmt.Fprint(w, "oauth_token=at&oauth_token_secret=ats")
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c, err := New(Options{BaseURL: srv.URL, UserAgent: testAgent, HTTP: srv.Client(), OAuth: OAuth{ConsumerKey: "ck", ConsumerSecret: "cs"}})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := c.OAuthRequestToken(t.Context(), "https://example.com/cb")
	if err != nil {
		t.Fatal(err)
	}
	if *rt != (OAuthToken{Token: "rt", Secret: "rts"}) {
		t.Errorf("request token = %+v", rt)
	}
	if got, want := rt.AuthorizeURL(), "https://www.discogs.com/oauth/authorize?oauth_token=rt"; got != want {
		t.Errorf("AuthorizeURL = %q, want %q", got, want)
	}
	at, err := c.OAuthAccessToken(t.Context(), *rt, "v123")
	if err != nil {
		t.Fatal(err)
	}
	if *at != (OAuthToken{Token: "at", Secret: "ats"}) {
		t.Errorf("access token = %+v", at)
	}
	if got := strings.Join(seen, " "); got != "GET /oauth/request_token POST /oauth/access_token" {
		t.Errorf("requests = %q", got)
	}
}

func TestOAuthFlowErrors(t *testing.T) {
	var calls atomic.Int32
	status, body := http.StatusOK, ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	defer srv.Close()
	consumer := OAuth{ConsumerKey: "ck", ConsumerSecret: "cs"}
	newClient := func(o OAuth) *Client {
		c, err := New(Options{BaseURL: srv.URL, UserAgent: testAgent, HTTP: srv.Client(), OAuth: o})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	if _, err := newClient(OAuth{}).OAuthRequestToken(t.Context(), ""); !errors.Is(err, errNoConsumer) {
		t.Errorf("without a consumer: %v, want errNoConsumer", err)
	}
	c := newClient(consumer)
	for _, verifier := range []string{"", "a b"} {
		if _, err := c.OAuthAccessToken(t.Context(), OAuthToken{Token: "rt", Secret: "rts"}, verifier); err == nil {
			t.Errorf("verifier %q accepted", verifier)
		}
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("refused calls sent %d requests", n)
	}

	status, body = http.StatusUnauthorized, `{"message": "Invalid consumer."}`
	if _, err := c.OAuthRequestToken(t.Context(), ""); err == nil {
		t.Error("401 accepted")
	} else if apiErr, ok := errors.AsType[*Error](err); !ok || apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("401 gave %v, want *Error", err)
	}

	status, body = http.StatusOK, "oauth_token_secret=leaked-secret"
	_, err := c.OAuthRequestToken(t.Context(), "")
	if err == nil || strings.Contains(err.Error(), "leaked-secret") {
		t.Errorf("incomplete body gave %v, want an error without the body", err)
	}

	status, body = http.StatusOK, ""
	if _, err := c.OAuthRequestToken(t.Context(), ""); !errors.Is(err, errEmptyBody) {
		t.Errorf("empty body gave %v, want errEmptyBody", err)
	}
}

func TestOAuthEscape(t *testing.T) {
	if got, want := oauthEscape("aZ09-._~ /&=+%é"), "aZ09-._~%20%2F%26%3D%2B%25%C3%A9"; got != want {
		t.Errorf("oauthEscape = %q, want %q", got, want)
	}
}
