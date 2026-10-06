package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

// TestNoTokenFlag guards the decision to read credentials from the environment
// only: a flag shows up in ps and shell history.
func TestNoTokenFlag(t *testing.T) {
	const token = "very-secret-flag-value"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL)
	}))
	defer srv.Close()

	stdout, stderr, err := run(t, srv, "--token", token, "release", "get", "1")
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --token") {
		t.Fatalf("error = %v, want unknown flag", err)
	}
	for _, s := range []string{stdout, stderr, err.Error()} {
		if strings.Contains(s, token) {
			t.Errorf("output contains the value: %q", s)
		}
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

func TestWantlistWrites(t *testing.T) {
	t.Setenv(envToken, "test-token")
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		switch {
		case r.URL.Path == "/oauth/identity":
			fmt.Fprint(w, `{"id": 7, "username": "hasteful"}`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			fmt.Fprint(w, `{"id": 8191071, "notes": "n", "basic_information": {"title": "Flygod"}}`)
		}
	}))
	defer srv.Close()

	stdout, _, err := run(t, srv, "wantlist", "add", "8191071", "--notes", "n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Title:") || !strings.Contains(stdout, "Notes:") {
		t.Errorf("stdout = %q, want the entry", stdout)
	}
	stdout, stderr, err := run(t, srv, "wantlist", "remove", "8191071")
	if err != nil || stdout != "" || !strings.Contains(stderr, "Removed release 8191071") {
		t.Errorf("remove gave stdout %q, stderr %q, %v; want only a note on stderr", stdout, stderr, err)
	}
	want := "GET /oauth/identity PUT /users/hasteful/wants/8191071 POST /users/hasteful/wants/8191071 GET /oauth/identity DELETE /users/hasteful/wants/8191071"
	if got := strings.Join(seen, " "); got != want {
		t.Errorf("requests = %q\nwant       %q", got, want)
	}

	seen = nil
	if _, _, err := run(t, srv, "wantlist", "edit", "8191071"); err == nil {
		t.Error("edit without --notes succeeded, want error")
	}
	t.Setenv(envToken, "")
	if _, _, err := run(t, srv, "wantlist", "add", "8191071"); err == nil || !strings.Contains(err.Error(), "needs a token") {
		t.Errorf("add without a token gave %v, want an error saying it needs one", err)
	}
	if len(seen) != 0 {
		t.Errorf("requests = %q, want none", seen)
	}
}

func TestReleaseRateAndUnrate(t *testing.T) {
	t.Setenv(envToken, "test-token")
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		switch {
		case r.URL.Path == "/oauth/identity":
			fmt.Fprint(w, `{"id": 7, "username": "hasteful"}`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			fmt.Fprint(w, `{"release_id": 8191071, "rating": 5, "username": "hasteful"}`)
		}
	}))
	defer srv.Close()

	stdout, _, err := run(t, srv, "release", "rate", "8191071", "--rating", "5")
	if err != nil || !strings.Contains(stdout, "Rating:   5") {
		t.Errorf("rate gave %q, %v", stdout, err)
	}
	stdout, stderr, err := run(t, srv, "release", "unrate", "8191071")
	if err != nil || stdout != "" || !strings.Contains(stderr, "Removed the rating of release 8191071") {
		t.Errorf("unrate gave stdout %q, stderr %q, %v", stdout, stderr, err)
	}
	want := "GET /oauth/identity PUT /releases/8191071/rating/hasteful GET /oauth/identity DELETE /releases/8191071/rating/hasteful"
	if got := strings.Join(seen, " "); got != want {
		t.Errorf("requests = %q\nwant       %q", got, want)
	}

	seen = nil
	for _, args := range [][]string{{"release", "rate", "8191071"}, {"release", "rate", "8191071", "--rating", "6"}, {"release", "rate", "8191071", "--rating", "0"}} {
		if _, _, err := run(t, srv, args...); err == nil {
			t.Errorf("%q succeeded, want error", args)
		}
	}
	t.Setenv(envToken, "")
	if _, _, err := run(t, srv, "release", "unrate", "8191071"); err == nil || !strings.Contains(err.Error(), "needs a token") {
		t.Errorf("unrate without a token gave %v", err)
	}
	if len(seen) != 0 {
		t.Errorf("requests = %q, want none", seen)
	}
}

func TestUserEdit(t *testing.T) {
	t.Setenv(envToken, "test-token")
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/identity" {
			fmt.Fprint(w, `{"id": 7, "username": "hasteful"}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, r.Method+" "+r.URL.Path+" "+string(b))
		fmt.Fprint(w, `{"id": 7, "username": "hasteful", "location": "Anytown", "curr_abbr": "EUR"}`)
	}))
	defer srv.Close()

	stdout, _, err := run(t, srv, "user", "edit", "--location", "Anytown", "--home-page", "", "--account-currency", "eur")
	if err != nil || !strings.Contains(stdout, "Location:") {
		t.Fatalf("edit gave %q, %v", stdout, err)
	}
	if len(bodies) != 1 || !strings.HasPrefix(bodies[0], "POST /users/hasteful ") {
		t.Fatalf("requests = %q", bodies)
	}
	var sent map[string]string
	json.Unmarshal([]byte(strings.SplitN(bodies[0], " ", 3)[2]), &sent)
	if len(sent) != 3 || sent["location"] != "Anytown" || sent["home_page"] != "" || sent["curr_abbr"] != "EUR" {
		t.Errorf("sent %v, want location, an empty home_page and EUR", sent)
	}

	bodies = nil
	for _, args := range [][]string{{}, {"--account-currency", "XYZ"}} {
		if _, _, err := run(t, srv, append([]string{"user", "edit"}, args...)...); err == nil {
			t.Errorf("user edit %q succeeded, want error", args)
		}
	}
	if len(bodies) != 0 {
		t.Errorf("refused edits sent %q", bodies)
	}
}

func TestUserContributions(t *testing.T) {
	t.Setenv(envToken, "test-token")
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		if r.URL.Path == "/oauth/identity" {
			fmt.Fprint(w, `{"id": 7, "username": "hasteful"}`)
			return
		}
		fmt.Fprint(w, `{"pagination": {"page": 1, "pages": 1, "urls": {}}, "contributions": []}`)
	}))
	defer srv.Close()

	if _, _, err := run(t, srv, "user", "contributions", "--sort", "YEAR", "--sort-order", "desc"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, srv, "user", "contributions", "someone"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/oauth/identity",
		"/users/hasteful/contributions?page=1&per_page=50&sort=year&sort_order=desc",
		"/users/someone/contributions?page=1&per_page=50",
	}
	if strings.Join(seen, " ") != strings.Join(want, " ") {
		t.Errorf("requests = %q\nwant       %q", seen, want)
	}

	seen = nil
	for _, args := range [][]string{{"--sort", "name"}, {"--sort-order", "up"}} {
		if _, _, err := run(t, srv, append([]string{"user", "contributions"}, args...)...); err == nil {
			t.Errorf("%q succeeded, want error", args)
		}
	}
	if len(seen) != 0 {
		t.Errorf("bad flags sent %q", seen)
	}
}

func TestUserListsAndListGet(t *testing.T) {
	t.Setenv(envToken, "test-token")
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		switch {
		case r.URL.Path == "/oauth/identity":
			fmt.Fprint(w, `{"id": 7, "username": "hasteful"}`)
		case strings.HasPrefix(r.URL.Path, "/lists/"):
			fmt.Fprint(w, `{"id": 100, "name": "Example List", "items": []}`)
		default:
			fmt.Fprint(w, `{"pagination": {"page": 1, "pages": 1, "urls": {}}, "lists": [{"id": 94, "name": "Another List", "public": true}]}`)
		}
	}))
	defer srv.Close()

	stdout, _, err := run(t, srv, "user", "lists")
	if err != nil || !strings.Contains(stdout, "Another List") {
		t.Fatalf("user lists gave %q, %v", stdout, err)
	}
	if _, _, err := run(t, srv, "user", "lists", "user"); err != nil {
		t.Fatal(err)
	}
	stdout, _, err = run(t, srv, "list", "get", "100")
	if err != nil || !strings.Contains(stdout, "Example List") {
		t.Fatalf("list get gave %q, %v", stdout, err)
	}
	want := []string{"/oauth/identity", "/users/hasteful/lists?page=1&per_page=50", "/users/user/lists?page=1&per_page=50", "/lists/100"}
	if strings.Join(seen, " ") != strings.Join(want, " ") {
		t.Errorf("requests = %q\nwant       %q", seen, want)
	}

	seen = nil
	if _, _, err := run(t, srv, "list", "get", "abc"); err == nil || len(seen) != 0 {
		t.Errorf("list get abc gave %v after %d requests, want an error and none", err, len(seen))
	}
}

func TestUserSubmissions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, `{"pagination": {"page": 2, "pages": 2, "urls": {}}, "submissions": {"releases": [{"id": 3}]}}`)
			return
		}
		fmt.Fprintf(w, `{"pagination": {"page": 1, "pages": 2, "urls": {"next": "http://%s/users/someone/submissions?page=2&per_page=100"}},
			"submissions": {"labels": [{"id": 2}], "releases": [{"id": 1}]}}`, r.Host)
	}))
	defer srv.Close()

	stdout, _, err := run(t, srv, "user", "submissions", "someone", "--all", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Artists  []struct{ ID int } `json:"artists"`
		Labels   []struct{ ID int } `json:"labels"`
		Releases []struct{ ID int } `json:"releases"`
	}
	if err := json.Unmarshal([]byte(stdout), &s); err != nil {
		t.Fatalf("stdout is not the submissions object: %v\n%s", err, stdout)
	}
	if s.Artists == nil || len(s.Labels) != 1 || len(s.Releases) != 2 {
		t.Errorf("submissions = %+v; want artists [], one label and both pages of releases", s)
	}
	if !strings.Contains(stdout, `"artists": []`) {
		t.Errorf("stdout has no empty artists array:\n%s", stdout)
	}
}
