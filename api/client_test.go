package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const testAgent = "discogsctl-test/0.0 (+https://hasteful.dev/discogsctl)"

func newTestClient(t *testing.T, srv *httptest.Server, token string) *Client {
	t.Helper()
	c, err := New(Options{BaseURL: srv.URL, UserAgent: testAgent, Token: token, HTTP: srv.Client()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestNewUserAgent(t *testing.T) {
	tests := []struct {
		agent   string
		wantErr bool
	}{
		{"discogsctl/dev (+https://hasteful.dev/discogsctl)", false},
		{"myapp/0.1 (+https://example.com)", false},
		{"LibraryMetadataEnhancer/0.3 +http://example.com/lime", false},
		{"", true},
		{"   ", true},
		{"my app", true},
		{"myapp", true},
		{"curl/8.4.0", true},
		{"Mozilla/5.0 (X11; Linux i686; rv:6.0.2) Gecko/20100101 Firefox/6.0.2", true},
		{"myapp/1.0 Chrome/120.0", true},
		{"Go-http-client/1.1", true},
	}
	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			_, err := New(Options{UserAgent: tt.agent})
			if (err != nil) != tt.wantErr {
				t.Fatalf("New(UserAgent %q) error = %v, wantErr %v", tt.agent, err, tt.wantErr)
			}
		})
	}
}

func TestNewBaseURL(t *testing.T) {
	tests := []struct {
		base    string
		wantErr bool
	}{
		{"", false},
		{"https://api.discogs.com", false},
		{"http://127.0.0.1:8080", false},
		{"http://localhost:8080", false},
		{"http://api.discogs.com", true},
		{"ftp://api.discogs.com", true},
		{"api.discogs.com", true},
	}
	for _, tt := range tests {
		t.Run(tt.base, func(t *testing.T) {
			_, err := New(Options{BaseURL: tt.base, UserAgent: testAgent, Token: "t"})
			if (err != nil) != tt.wantErr {
				t.Fatalf("New(BaseURL %q) error = %v, wantErr %v", tt.base, err, tt.wantErr)
			}
		})
	}
}

func TestNewRejectsMalformedTokenWithoutEchoingIt(t *testing.T) {
	for _, token := range []string{"abc def", "abc\n", "\tabc"} {
		_, err := New(Options{UserAgent: testAgent, Token: token})
		if err == nil {
			t.Fatalf("New(Token %q) succeeded, want error", token)
		}
		if strings.Contains(err.Error(), strings.TrimSpace(token)) {
			t.Errorf("error %q contains the token", err)
		}
	}
}

func TestNewCurrency(t *testing.T) {
	if _, err := New(Options{UserAgent: testAgent, Currency: "gbp"}); err != nil {
		t.Errorf("New(Currency gbp) error = %v, want nil", err)
	}
	if _, err := New(Options{UserAgent: testAgent, Currency: "XYZ"}); err == nil {
		t.Error("New(Currency XYZ) succeeded, want error")
	}
}

func TestRequestHeaders(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		wantAuth string
	}{
		{"token in header", "secret-token", "Discogs token=secret-token"},
		{"anonymous", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *http.Request
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Clone(r.Context())
				w.Write([]byte(`{"id": 249504, "title": "Never Gonna Give You Up"}`))
			}))
			defer srv.Close()

			c := newTestClient(t, srv, tt.token)
			if _, err := c.Release(t.Context(), 249504); err != nil {
				t.Fatalf("Release: %v", err)
			}

			if got.URL.Path != "/releases/249504" {
				t.Errorf("path = %q, want /releases/249504", got.URL.Path)
			}
			if ua := got.Header.Get("User-Agent"); ua != testAgent {
				t.Errorf("User-Agent = %q, want %q", ua, testAgent)
			}
			if accept := got.Header.Get("Accept"); accept != MediaType {
				t.Errorf("Accept = %q, want %q", accept, MediaType)
			}
			if auth := got.Header.Get("Authorization"); auth != tt.wantAuth {
				t.Errorf("Authorization = %q, want %q", auth, tt.wantAuth)
			}
			if got.URL.RawQuery != "" {
				t.Errorf("query = %q, want none: credentials belong in the header", got.URL.RawQuery)
			}
		})
	}
}

func TestReleaseSendsCurrency(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Write([]byte(`{"id": 1}`))
	}))
	defer srv.Close()

	c, err := New(Options{BaseURL: srv.URL, UserAgent: testAgent, Currency: "eur", HTTP: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Release(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if query != "curr_abbr=EUR" {
		t.Errorf("query = %q, want curr_abbr=EUR", query)
	}
}

func TestErrorResponses(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantMsg string
	}{
		{"discogs message", http.StatusNotFound, `{"message": "Release not found."}`, "Release not found."},
		{"auth required", http.StatusUnauthorized, `{"message": "You must authenticate to access this resource."}`, "You must authenticate to access this resource."},
		{"not json", http.StatusBadGateway, `<html>bad gateway</html>`, "Bad Gateway"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			_, err := newTestClient(t, srv, "").Release(t.Context(), 1)
			apiErr, ok := errors.AsType[*Error](err)
			if !ok {
				t.Fatalf("error = %v, want *Error", err)
			}
			if apiErr.StatusCode != tt.status || apiErr.Message != tt.wantMsg {
				t.Errorf("error = %+v, want status %d message %q", apiErr, tt.status, tt.wantMsg)
			}
		})
	}
}

func TestEmptyBodyIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	_, err := newTestClient(t, srv, "").Release(t.Context(), 1)
	if !errors.Is(err, errEmptyBody) {
		t.Fatalf("error = %v, want errEmptyBody", err)
	}
}

func TestInvalidIDSendsNothing(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()

	if _, err := newTestClient(t, srv, "").Release(t.Context(), 0); err == nil {
		t.Error("Release(0) succeeded, want error")
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("server saw %d requests, want 0", n)
	}
}

func TestUsernameCannotWalkToAnotherRoute(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		w.Write([]byte(`{"folders": []}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv, "")

	for _, name := range []string{"..", ".", ""} {
		if _, err := c.CollectionFolders(t.Context(), name); err == nil {
			t.Errorf("CollectionFolders(%q) succeeded, want error", name)
		}
	}
	if _, err := c.CollectionFolders(t.Context(), "a/../b"); err != nil {
		t.Fatalf("CollectionFolders(a/../b): %v", err)
	}
	if want := "/users/a%2F..%2Fb/collection/folders"; len(paths) != 1 || paths[0] != want {
		t.Errorf("paths = %q, want [%q]", paths, want)
	}
}

func TestMarshalReturnsTheDiscogsBody(t *testing.T) {
	body := `{"title":"Never Gonna Give You Up","id":249504,"not_modelled":{"nested":[1,2]},"labels":[{"name":"R & S Records","catno":"RS 1"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	r, err := newTestClient(t, srv, "").Release(t.Context(), 249504)
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != 249504 || r.Labels[0].Catno != "RS 1" {
		t.Errorf("decoded %+v, want id 249504 and catno RS 1", r)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != body {
		t.Errorf("re-encoded\n %s\nwant\n %s", got, body)
	}
}

func TestMarshalWithoutRawEncodesFields(t *testing.T) {
	b, err := json.Marshal(Folder{ID: 1, Name: "Uncategorized", Count: 3})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":1,"name":"Uncategorized","count":3,"resource_url":""}`
	if string(b) != want {
		t.Errorf("Marshal = %s, want %s", b, want)
	}
}
