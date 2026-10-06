package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.hasteful.org/discogsctl/internal/cli"
)

// collectionHost answers like Discogs for a collection that holds the copies
// given, as "instance:folder" pairs, of release 5077187.
func collectionHost(t *testing.T, copies ...string) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		switch {
		case r.URL.Path == "/oauth/identity":
			fmt.Fprint(w, `{"id": 7, "username": "user"}`)
		case r.Method == http.MethodGet:
			items := make([]string, len(copies))
			for i, c := range copies {
				instance, folder, _ := strings.Cut(c, ":")
				items[i] = fmt.Sprintf(`{"id": 5077187, "instance_id": %s, "folder_id": %s}`, instance, folder)
			}
			fmt.Fprintf(w, `{"pagination": {"page": 1, "pages": 1, "urls": {}}, "releases": [%s]}`, strings.Join(items, ","))
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"instance_id": 99, "resource_url": "https://api.discogs.com/x"}`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestCollectionAdd(t *testing.T) {
	t.Setenv(envToken, "test-token")
	srv, seen := collectionHost(t)

	stdout, _, err := run(t, srv, "collection", "add", "5077187")
	if err != nil || !strings.Contains(stdout, "Instance: 99") {
		t.Fatalf("add gave %q, %v", stdout, err)
	}
	if got := strings.Join(*seen, " "); got != "GET /oauth/identity POST /users/user/collection/folders/1/releases/5077187" {
		t.Errorf("requests = %q, want the add to Uncategorized", got)
	}

	*seen = nil
	for _, args := range [][]string{{"--folder", "0"}, {"--username", "someone"}} {
		if _, _, err := run(t, srv, append([]string{"collection", "add", "5077187"}, args...)...); err == nil {
			t.Errorf("add %q succeeded, want error", args)
		}
	}
	if len(*seen) != 0 {
		t.Errorf("refused adds sent %q", *seen)
	}
}

func TestCollectionRemove(t *testing.T) {
	t.Setenv(envToken, "test-token")
	for _, tt := range []struct {
		name     string
		copies   []string
		args     []string
		deleted  string
		wantErr  string
		wantExit int
	}{
		{name: "one copy", copies: []string{"11:3"}, deleted: "/users/user/collection/folders/3/releases/5077187/instances/11"},
		{name: "chosen copy", copies: []string{"11:1", "22:3"}, args: []string{"--instance", "22"}, deleted: "/users/user/collection/folders/3/releases/5077187/instances/22"},
		{name: "two copies, none chosen", copies: []string{"11:1", "22:3"}, wantErr: "instance 11 in folder 1, instance 22 in folder 3", wantExit: cli.ExitFailure},
		{name: "not in collection", wantErr: "not in your collection", wantExit: cli.ExitNotFound},
		{name: "unknown instance", copies: []string{"11:1"}, args: []string{"--instance", "5"}, wantErr: "no instance 5", wantExit: cli.ExitNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv, seen := collectionHost(t, tt.copies...)
			_, stderr, err := run(t, srv, append([]string{"collection", "remove", "5077187"}, tt.args...)...)
			var deleted string
			for _, s := range *seen {
				if path, ok := strings.CutPrefix(s, "DELETE "); ok {
					deleted = path
				}
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || cli.ExitCode(err) != tt.wantExit {
					t.Errorf("error = %v, exit %d; want %q, exit %d", err, cli.ExitCode(err), tt.wantErr, tt.wantExit)
				}
				if deleted != "" {
					t.Errorf("deleted %s after an error", deleted)
				}
				return
			}
			if err != nil || deleted != tt.deleted || !strings.Contains(stderr, "Removed instance") {
				t.Errorf("deleted %q, stderr %q, %v; want %q", deleted, stderr, err, tt.deleted)
			}
		})
	}
}
