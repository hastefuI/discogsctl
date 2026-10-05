package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// run executes discogsctl with args against srv and returns stdout, stderr and
// the error from the command.
func run(t *testing.T, srv *httptest.Server, args ...string) (string, string, error) {
	t.Helper()
	root, cfg := newRootCmd(VersionInfo{Version: "test"})
	cfg.baseURL = srv.URL
	cfg.dumpURL = srv.URL
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.ExecuteContext(t.Context())
	return stdout.String(), stderr.String(), err
}

func TestUserAgentIsFixed(t *testing.T) {
	if got, want := userAgent("1.2.3"), "discogsctl/1.2.3 (+https://hasteful.dev/discogsctl)"; got != want {
		t.Errorf("userAgent = %q, want %q", got, want)
	}
}

func TestCollectionListAllResolvesTheTokenHolder(t *testing.T) {
	t.Setenv(envToken, "test-token")
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Discogs token=test-token" {
			t.Errorf("%s sent without the token header", r.URL.Path)
		}
		if ua := r.Header.Get("User-Agent"); ua != userAgent("test") {
			t.Errorf("User-Agent = %q, want %q", ua, userAgent("test"))
		}
		seen = append(seen, r.URL.RequestURI())
		switch {
		case r.URL.Path == "/oauth/identity":
			fmt.Fprint(w, `{"id": 7, "username": "hasteful"}`)
		case r.URL.Query().Get("page") == "1":
			fmt.Fprintf(w, `{"pagination": {"page": 1, "pages": 2, "urls": {"next": "http://%s/users/hasteful/collection/folders/0/releases?page=2&per_page=100"}},
				"releases": [{"id": 1, "instance_id": 10}]}`, r.Host)
		default:
			fmt.Fprint(w, `{"pagination": {"page": 2, "pages": 2, "urls": {}}, "releases": [{"id": 2, "instance_id": 20}]}`)
		}
	}))
	defer srv.Close()

	stdout, _, err := run(t, srv, "collection", "list", "--all", "--output", "json")
	if err != nil {
		t.Fatal(err)
	}
	var items []struct {
		ID         int `json:"id"`
		InstanceID int `json:"instance_id"`
	}
	if err := json.Unmarshal([]byte(stdout), &items); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\n%s", err, stdout)
	}
	if len(items) != 2 || items[0].InstanceID != 10 || items[1].InstanceID != 20 {
		t.Errorf("items = %+v, want instances 10 and 20", items)
	}
	want := []string{
		"/oauth/identity",
		"/users/hasteful/collection/folders/0/releases?page=1&per_page=100",
		"/users/hasteful/collection/folders/0/releases?page=2&per_page=100",
	}
	if strings.Join(seen, " ") != strings.Join(want, " ") {
		t.Errorf("requests = %q, want %q", seen, want)
	}
}

func TestTokenNeverReachesOutput(t *testing.T) {
	const token = "very-secret-token-value"
	t.Setenv(envToken, token)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message": "Release not found."}`)
	}))
	defer srv.Close()

	stdout, stderr, err := run(t, srv, "release", "get", "1", "--verbose")
	if err == nil {
		t.Fatal("want a 404 error")
	}
	for _, s := range []string{stdout, stderr, err.Error()} {
		if strings.Contains(s, token) {
			t.Errorf("output contains the token: %q", s)
		}
	}
	if !strings.Contains(stderr, "status=404") {
		t.Errorf("verbose stderr = %q, want the response logged", stderr)
	}
}

func TestUsernameRequiredWithoutToken(t *testing.T) {
	t.Setenv(envToken, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL)
	}))
	defer srv.Close()

	_, _, err := run(t, srv, "wantlist", "list")
	if err == nil || !strings.Contains(err.Error(), "--username") {
		t.Errorf("error = %v, want one asking for --username", err)
	}
}

func TestInvalidOutputFormat(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if _, _, err := run(t, srv, "release", "get", "1", "--output", "yaml"); err == nil {
		t.Error("--output yaml succeeded, want error")
	}
}
