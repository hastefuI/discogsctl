package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.hasteful.org/discogsctl/internal/cli"
)

// setOAuth sets the four OAuth variables, so a value in the developer's own
// environment cannot leak into a test.
func setOAuth(t *testing.T, key, secret, token, tokenSecret string) {
	t.Helper()
	t.Setenv(envConsumerKey, key)
	t.Setenv(envConsumerSecret, secret)
	t.Setenv(envOAuthToken, token)
	t.Setenv(envOAuthTokenSecret, tokenSecret)
}

// runWithInput is run with stdin.
func runWithInput(t *testing.T, srv *httptest.Server, stdin string, args ...string) (string, string, error) {
	t.Helper()
	root, cfg := newRootCmd(VersionInfo{Version: "test"})
	cfg.baseURL = srv.URL
	var stdout, stderr bytes.Buffer
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.ExecuteContext(t.Context())
	return stdout.String(), stderr.String(), err
}

// authServer is Discogs for auth verify and auth exchange: the OAuth flow, an
// identity that answers only the good token and the access token, and a search
// that answers only the good consumer key and secret.
func authServer(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = append(*seen, r.Method+" "+r.URL.Path)
		auth := r.Header.Get("Authorization")
		unauthorized := func() {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"message": "You must authenticate to access this resource."}`)
		}
		switch r.URL.Path {
		case "/oauth/request_token":
			fmt.Fprint(w, "oauth_token=rt&oauth_token_secret=rts&oauth_callback_confirmed=true")
		case "/oauth/access_token":
			if !strings.Contains(auth, `oauth_verifier="v123"`) || !strings.Contains(auth, `oauth_token="rt"`) {
				t.Errorf("access token Authorization = %q", auth)
			}
			fmt.Fprint(w, "oauth_token=at&oauth_token_secret=ats")
		case "/oauth/identity":
			if auth != "Discogs token=good-token" && !strings.Contains(auth, `oauth_token="at"`) {
				unauthorized()
				return
			}
			fmt.Fprint(w, `{"id": 1001, "username": "user"}`)
		case "/database/search":
			if auth != "Discogs key=good-consumer-key, secret=good-consumer-secret" {
				unauthorized()
				return
			}
			fmt.Fprint(w, `{"pagination": {"page": 1, "pages": 1, "urls": {}}, "results": []}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAuthExchange(t *testing.T) {
	t.Setenv(envToken, "")
	setOAuth(t, "ck", "cs", "", "")
	var seen []string
	srv := authServer(t, &seen)
	want := exchanged{Username: "user", OAuthToken: "at", OAuthTokenSecret: "ats"}

	// Text by default: a labelled block, never NAME=value shell.
	stdout, stderr, err := runWithInput(t, srv, "v123\n", "auth", "exchange")
	if err != nil {
		t.Fatal(err)
	}
	if want := "Username:     user\nToken:        at\nToken secret: ats\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if strings.Contains(stdout, "=") {
		t.Errorf("stdout looks like shell assignments: %q", stdout)
	}
	for _, w := range []string{"https://www.discogs.com/oauth/authorize?oauth_token=rt", "belongs to user", "shown once and not stored"} {
		if !strings.Contains(stderr, w) {
			t.Errorf("stderr = %q, want %q", stderr, w)
		}
	}

	stdout, stderr, err = runWithInput(t, srv, "https://example.com/cb?oauth_token=rt&oauth_verifier=v123", "auth", "exchange", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var prompt map[string]string
	if err := json.Unmarshal([]byte(stderr), &prompt); err != nil || len(prompt) != 1 || prompt["authorize_url"] != "https://www.discogs.com/oauth/authorize?oauth_token=rt" {
		t.Errorf("JSON stderr = %q (%v), want only the authorize URL as an object", stderr, err)
	}
	var got exchanged
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || got != want {
		t.Errorf("JSON stdout = %q (%v), want %+v", stdout, err, want)
	}
	wantSeen := "GET /oauth/request_token POST /oauth/access_token GET /oauth/identity GET /oauth/request_token POST /oauth/access_token GET /oauth/identity"
	if got := strings.Join(seen, " "); got != wantSeen {
		t.Errorf("requests = %q\nwant       %q", got, wantSeen)
	}
}

func TestAuthVerify(t *testing.T) {
	// Configured credentials that every other command refuses do not stop
	// verify, which checks only what is entered.
	t.Setenv(envToken, "configured-token")
	setOAuth(t, "", "", "at", "ats")
	var seen []string
	srv := authServer(t, &seen)
	const pat, consumer = "good-token\n", "good-consumer-key\ngood-consumer-secret\n"

	// noEcho fails the test if any output holds a value that was entered.
	noEcho := func(typ string, outputs ...string) {
		t.Helper()
		for _, out := range outputs {
			for _, secret := range []string{"good-token", "good-consumer-key", "good-consumer-secret", "bad-token", "wrong"} {
				if strings.Contains(out, secret) {
					t.Errorf("verify %s printed the entered %q: %q", typ, secret, out)
				}
			}
		}
	}

	stdout, stderr, err := runWithInput(t, srv, pat, "auth", "verify", "pat")
	if err != nil || !strings.Contains(stdout, "Verified: the token belongs to user.") || !strings.Contains(stdout, "To use it, set "+envToken+".") {
		t.Errorf("verify pat gave stdout %q, %v", stdout, err)
	}
	noEcho("pat", stdout, stderr)
	stdout, stderr, err = runWithInput(t, srv, consumer, "auth", "verify", "consumer")
	if err != nil || !strings.Contains(stdout, "Verified: Discogs accepted the consumer key and secret.") {
		t.Errorf("verify consumer gave stdout %q, %v", stdout, err)
	}
	noEcho("consumer", stdout, stderr)
	if got := strings.Join(seen, " "); got != "GET /oauth/identity GET /database/search" {
		t.Errorf("requests = %q, want the identity then a search", got)
	}

	// JSON mode writes only the result object.
	for _, tt := range []struct {
		typ, input string
		want       verifyResult
	}{
		{"pat", pat, verifyResult{Type: "pat", Verified: true, Username: "user"}},
		{"consumer", consumer, verifyResult{Type: "consumer", Verified: true}},
	} {
		stdout, stderr, err := runWithInput(t, srv, tt.input, "auth", "verify", tt.typ, "-o", "json")
		var got verifyResult
		if err != nil || stderr != "" || json.Unmarshal([]byte(stdout), &got) != nil || got != tt.want {
			t.Errorf("verify %s -o json gave stdout %q, stderr %q, %v; want only %+v", tt.typ, stdout, stderr, err, tt.want)
		}
		noEcho(tt.typ, stdout, stderr)
	}

	for _, tt := range []struct{ typ, input string }{{"pat", "bad-token\n"}, {"consumer", "good-consumer-key\nwrong\n"}} {
		stdout, stderr, err := runWithInput(t, srv, tt.input, "auth", "verify", tt.typ)
		if err == nil || cli.ExitCode(err) != cli.ExitUnauthorized || stdout != "" {
			t.Errorf("verify %s with a bad credential gave stdout %q, %v; want nothing printed and exit 3", tt.typ, stdout, err)
		}
		noEcho(tt.typ, stdout, stderr, fmt.Sprint(err))
	}
}

func TestAuthVerifyAndExchangeRefusals(t *testing.T) {
	t.Setenv(envToken, "")
	var seen []string
	srv := authServer(t, &seen)

	setOAuth(t, "", "", "", "")
	for _, args := range [][]string{{}, {"token"}, {"oauth1"}, {"pat", "consumer"}} {
		_, _, err := runWithInput(t, srv, "", append([]string{"auth", "verify"}, args...)...)
		if err == nil || !strings.Contains(err.Error(), "pat, consumer") {
			t.Errorf("verify %q gave %v, want an error listing the types", args, err)
		}
	}
	if _, _, err := runWithInput(t, srv, "", "auth", "exchange", "extra"); err == nil {
		t.Error("exchange with an argument succeeded, want an error")
	}
	if _, _, err := runWithInput(t, srv, "", "auth", "exchange"); err == nil || !strings.Contains(err.Error(), envConsumerKey) {
		t.Errorf("exchange without a consumer gave %v, want an error naming %s", err, envConsumerKey)
	}
	for _, typ := range []string{"pat", "consumer"} {
		if _, _, err := runWithInput(t, srv, "\n", "auth", "verify", typ); err == nil || !strings.Contains(err.Error(), "was entered") {
			t.Errorf("verify %s with nothing entered gave %v", typ, err)
		}
	}
	if len(seen) != 0 {
		t.Errorf("requests = %q, want none", seen)
	}

	setOAuth(t, "ck", "cs", "", "")
	if _, _, err := runWithInput(t, srv, "\n", "auth", "exchange"); err == nil || !strings.Contains(err.Error(), "verification code") {
		t.Errorf("an empty code gave %v", err)
	}
	if got := strings.Join(seen, " "); got != "GET /oauth/request_token" {
		t.Errorf("requests = %q, want only the request token", got)
	}
}

func TestOAuthCredentialsSignRequests(t *testing.T) {
	const secret = "very-secret-oauth-value"
	t.Setenv(envToken, "")
	setOAuth(t, "ck", secret, "at", secret+"-2")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "OAuth ") || !strings.Contains(auth, `oauth_token="at"`) {
			t.Errorf("Authorization = %q, want an OAuth header with the access token", auth)
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message": "Release not found."}`)
	}))
	defer srv.Close()

	stdout, stderr, err := run(t, srv, "release", "get", "1", "--verbose")
	if err == nil {
		t.Fatal("want a 404 error")
	}
	for _, s := range []string{stdout, stderr, err.Error()} {
		if strings.Contains(s, secret) {
			t.Errorf("output contains a secret: %q", s)
		}
	}
}

func TestConsumerKeyAndSecretAloneActAsNoUser(t *testing.T) {
	t.Setenv(envToken, "")
	setOAuth(t, "ck", "cs", "", "")
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		if auth := r.Header.Get("Authorization"); auth != "Discogs key=ck, secret=cs" {
			t.Errorf("Authorization = %q, want the consumer key and secret", auth)
		}
		fmt.Fprint(w, `{"id": 249504, "title": "Never Gonna Give You Up"}`)
	}))
	defer srv.Close()

	if _, _, err := run(t, srv, "release", "get", "249504"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, srv, "wantlist", "list"); err == nil || !strings.Contains(err.Error(), "--username") {
		t.Errorf("wantlist list gave %v, want one asking for --username", err)
	}
	if got := strings.Join(seen, " "); got != "/releases/249504" {
		t.Errorf("requests = %q, want only the release", got)
	}
}

func TestTokenAndOAuthTogetherAreRefused(t *testing.T) {
	t.Setenv(envToken, "test-token")
	setOAuth(t, "ck", "cs", "at", "ats")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL)
	}))
	defer srv.Close()

	if _, _, err := run(t, srv, "auth", "whoami"); err == nil || !strings.Contains(err.Error(), "one or the other") {
		t.Errorf("both credentials gave %v, want an error", err)
	}
}

func TestAuthStatus(t *testing.T) {
	const secret = "very-secret-status-value"
	tests := []struct {
		name                           string
		token, key, keySecret, at, ats string
		args                           []string
		want                           []string
		wantErr                        bool
	}{
		{name: "nothing", want: []string{"pat:not configured", "consumer:not configured", "oauth1:not configured", "Requests are anonymous, at up to 25"}},
		{name: "personal token", token: secret, key: "ck", keySecret: secret,
			want: []string{"pat:in use", "consumer:configured", "oauth1:not configured", "act as a user, at up to 60"}},
		{name: "consumer alone", key: "ck", keySecret: secret,
			want: []string{"pat:not configured", "consumer:in use", "oauth1:not configured", "act as no user, at up to 60"}},
		{name: "oauth", key: "ck", keySecret: secret, at: "at", ats: secret,
			want: []string{"consumer:in use", "oauth1:in use", "act as a user"}},
		{name: "key without secret", key: "ck", wantErr: true,
			want: []string{"consumer:incomplete"}},
		{name: "access token without consumer", at: "at", ats: secret, wantErr: true,
			want: []string{"oauth1:incomplete"}},
		{name: "token and oauth together", token: secret, key: "ck", keySecret: secret, at: "at", ats: secret, wantErr: true,
			want: []string{"pat:configured", "oauth1:configured"}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("auth status sent %s", r.URL)
	}))
	defer srv.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envToken, tt.token)
			setOAuth(t, tt.key, tt.keySecret, tt.at, tt.ats)

			stdout, stderr, err := run(t, srv, append([]string{"auth", "status"}, tt.args...)...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			jsonOut, _, _ := run(t, srv, append([]string{"auth", "status", "-o", "json"}, tt.args...)...)
			var st authStatus
			if err := json.Unmarshal([]byte(jsonOut), &st); err != nil {
				t.Fatalf("JSON stdout: %v\n%s", err, jsonOut)
			}
			got := map[string]string{}
			for _, m := range st.Methods {
				got[m.Type] = m.Status
			}
			// A want is type:status, checked in both views, or text the text
			// view prints.
			for _, w := range tt.want {
				typ, status, ok := strings.Cut(w, ":")
				if _, known := got[typ]; !ok || !known {
					if !strings.Contains(stdout, w) {
						t.Errorf("stdout has no %q:\n%s", w, stdout)
					}
					continue
				}
				if got[typ] != status {
					t.Errorf("JSON %s = %q, want %q", typ, got[typ], status)
				}
				if !regexp.MustCompile(`(?m)^\S.*\s` + typ + `\s+` + status + `\s+DISCOGSCTL_[\w, ]+$`).MatchString(stdout) {
					t.Errorf("text has no %s row with status %q:\n%s", typ, status, stdout)
				}
			}
			if strings.Contains(jsonOut, `"detail"`) {
				t.Errorf("JSON has a detail field:\n%s", jsonOut)
			}
			if header := strings.Fields(strings.SplitN(stdout, "\n", 2)[0]); strings.Join(header, " ") != "PRIORITY METHOD TYPE STATUS VARIABLES" {
				t.Errorf("text header = %q, want PRIORITY METHOD TYPE STATUS VARIABLES", header)
			}
			wantVars := map[string][]string{
				typePAT:      {envToken},
				typeOAuth1:   {envOAuthToken, envOAuthTokenSecret},
				typeConsumer: {envConsumerKey, envConsumerSecret},
			}
			for _, m := range st.Methods {
				if !slices.Equal(m.Variables, wantVars[m.Type]) {
					t.Errorf("JSON %s variables = %q, want %q", m.Type, m.Variables, wantVars[m.Type])
				}
				if want := m.Type + `\s+[a-z ]+\s+` + strings.Join(m.Variables, ", ") + `$`; !regexp.MustCompile(`(?m)\s` + want).MatchString(stdout) {
					t.Errorf("text has no %s row ending in its variables:\n%s", m.Type, stdout)
				}
			}
			for i, m := range st.Methods {
				if m.Priority != i+1 {
					t.Errorf("methods[%d] = %s with priority %d, want %d", i, m.Type, m.Priority, i+1)
				}
			}
			if got := []string{st.Methods[0].Type, st.Methods[1].Type, st.Methods[2].Type}; strings.Join(got, " ") != "pat oauth1 consumer" {
				t.Errorf("order = %q, want pat oauth1 consumer", got)
			}
			for _, s := range []string{stdout, stderr, jsonOut} {
				if strings.Contains(s, secret) {
					t.Errorf("output contains a secret: %q", s)
				}
			}
		})
	}
}

func TestAuthIdentityAndWhoami(t *testing.T) {
	t.Setenv(envToken, "test-token")
	setOAuth(t, "", "", "", "")
	var seen []string
	profileStatus := http.StatusOK
	staff := `, "is_staff": false`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		switch r.URL.Path {
		case "/oauth/identity":
			fmt.Fprint(w, `{"id": 1001, "username": "user", "consumer_name": "Example App", "resource_url": "https://api.discogs.com/users/user"}`)
		case "/users/user":
			w.WriteHeader(profileStatus)
			if profileStatus != http.StatusOK {
				fmt.Fprint(w, `{"message": "User does not exist or may have been deleted."}`)
				return
			}
			fmt.Fprintf(w, `{"id": 1001, "username": "user", "email": "user@example.com", "num_collection": 3, "num_wantlist": 2,
				"uri": "https://www.discogs.com/user/user", "not_modelled": true%s}`, staff)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	stdout, _, err := run(t, srv, "auth", "identity")
	if err != nil || !strings.Contains(stdout, "Application: Example App") || strings.Contains(stdout, "Email:") {
		t.Errorf("auth identity gave %q, %v; want the identity alone", stdout, err)
	}
	if got := strings.Join(seen, " "); got != "/oauth/identity" {
		t.Errorf("auth identity requests = %q, want one", got)
	}

	seen = nil
	stdout, _, err = run(t, srv, "auth", "whoami")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Username:    user", "Application: Example App", "Staff:       No", "Email:       user@example.com", "Collection:  3"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("whoami has no %q:\n%s", want, stdout)
		}
	}
	if n := strings.Count(stdout, "Username:"); n != 1 {
		t.Errorf("whoami prints the username %d times, want once:\n%s", n, stdout)
	}
	if got := strings.Join(seen, " "); got != "/oauth/identity /users/user" {
		t.Errorf("whoami requests = %q, want the identity then the profile", got)
	}

	staff = `, "is_staff": true`
	if stdout, _, err := run(t, srv, "auth", "whoami"); err != nil || !strings.Contains(stdout, "Staff:       Yes") {
		t.Errorf("a staff member gave %q, %v; want Staff: Yes", stdout, err)
	}
	staff = ""
	if stdout, _, err := run(t, srv, "auth", "whoami"); err != nil || strings.Contains(stdout, "Staff:") {
		t.Errorf("a profile without is_staff gave %q, %v; want no Staff line", stdout, err)
	}

	stdout, _, err = run(t, srv, "auth", "whoami", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var both struct {
		Identity struct {
			ConsumerName string `json:"consumer_name"`
		} `json:"identity"`
		User struct {
			Email       string `json:"email"`
			NotModelled bool   `json:"not_modelled"`
		} `json:"user"`
	}
	if err := json.Unmarshal([]byte(stdout), &both); err != nil {
		t.Fatalf("JSON stdout: %v\n%s", err, stdout)
	}
	if both.Identity.ConsumerName != "Example App" || both.User.Email != "user@example.com" || !both.User.NotModelled {
		t.Errorf("JSON = %+v, want both Discogs bodies with every field", both)
	}

	profileStatus = http.StatusNotFound
	_, _, err = run(t, srv, "auth", "whoami")
	if err == nil || cli.ExitCode(err) != cli.ExitNotFound || !strings.Contains(err.Error(), "profile of user") {
		t.Errorf("a missing profile gave %v, want a 404 naming the profile", err)
	}
}

func TestVerifier(t *testing.T) {
	tests := map[string]string{
		"  v123\n": "v123",
		"https://example.com/cb?oauth_token=rt&oauth_verifier=v123": "v123",
		"":         "",
		"no-query": "no-query",
	}
	for in, want := range tests {
		if got := verifier(in); got != want {
			t.Errorf("verifier(%q) = %q, want %q", in, got, want)
		}
	}
}
