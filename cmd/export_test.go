package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// exportHost answers like Discogs for one user with two pages of collection
// and one of wantlist. With failWants set, the wantlist answers 500.
func exportHost(t *testing.T, failWants bool) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		switch {
		case r.URL.Path == "/oauth/identity":
			fmt.Fprint(w, `{"id": 7, "username": "user"}`)
		case r.URL.Path == "/users/user/collection/folders":
			fmt.Fprint(w, `{"folders": [{"id": 0, "name": "All", "count": 3}, {"id": 1, "name": "Uncategorized", "count": 3}]}`)
		case r.URL.Path == "/users/user/collection/folders/0/releases" && r.URL.Query().Get("page") == "1":
			fmt.Fprintf(w, `{"pagination": {"page": 1, "pages": 2, "urls": {"next": "http://%s/users/user/collection/folders/0/releases?page=2&per_page=100"}},
				"releases": [{"id": 1, "instance_id": 10, "folder_id": 1}, {"id": 2, "instance_id": 20, "folder_id": 1}]}`, r.Host)
		case r.URL.Path == "/users/user/collection/folders/0/releases":
			fmt.Fprint(w, `{"pagination": {"page": 2, "pages": 2, "urls": {}}, "releases": [{"id": 3, "instance_id": 30, "folder_id": 1, "notes": [{"field_id": 3, "value": "signed"}]}]}`)
		case r.URL.Path == "/users/user/wants" && failWants:
			w.WriteHeader(http.StatusInternalServerError)
		case r.URL.Path == "/users/user/wants":
			fmt.Fprint(w, `{"pagination": {"page": 1, "pages": 1, "urls": {}}, "wants": [{"id": 182213, "notes": "first press"}]}`)
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestExport(t *testing.T) {
	t.Setenv(envToken, "test-token")
	srv, seen := exportHost(t, false)

	stdout, _, err := run(t, srv, "export")
	if err != nil {
		t.Fatal(err)
	}
	var e struct {
		Username   string    `json:"username"`
		ExportedAt time.Time `json:"exported_at"`
		Collection struct {
			Folders []struct {
				Name string `json:"name"`
			} `json:"folders"`
			Releases []struct {
				InstanceID int `json:"instance_id"`
				FolderID   int `json:"folder_id"`
				Notes      []struct {
					Value string `json:"value"`
				} `json:"notes"`
			} `json:"releases"`
		} `json:"collection"`
		Wantlist []struct {
			ID    int    `json:"id"`
			Notes string `json:"notes"`
		} `json:"wantlist"`
	}
	if err := json.Unmarshal([]byte(stdout), &e); err != nil {
		t.Fatalf("stdout is not the JSON export, even without --output json: %v\n%s", err, stdout)
	}
	if e.Username != "user" || e.ExportedAt.IsZero() {
		t.Errorf("username %q, exported_at %v", e.Username, e.ExportedAt)
	}
	if len(e.Collection.Folders) != 2 || len(e.Collection.Releases) != 3 || e.Collection.Releases[2].Notes[0].Value != "signed" {
		t.Errorf("collection = %+v, want 2 folders and 3 releases with the notes", e.Collection)
	}
	if len(e.Wantlist) != 1 || e.Wantlist[0].Notes != "first press" {
		t.Errorf("wantlist = %+v", e.Wantlist)
	}
	want := []string{
		"/oauth/identity",
		"/users/user/collection/folders",
		"/users/user/collection/folders/0/releases?page=1&per_page=100",
		"/users/user/collection/folders/0/releases?page=2&per_page=100",
		"/users/user/wants?page=1&per_page=100",
	}
	if strings.Join(*seen, " ") != strings.Join(want, " ") {
		t.Errorf("requests = %q\nwant       %q", *seen, want)
	}
}

func TestExportFailurePrintsNothing(t *testing.T) {
	t.Setenv(envToken, "test-token")
	srv, _ := exportHost(t, true)

	stdout, _, err := run(t, srv, "export")
	if err == nil || !strings.Contains(err.Error(), "exporting wantlist") {
		t.Errorf("error = %v, want one naming the wantlist", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q after a failure, want nothing", stdout)
	}
}
