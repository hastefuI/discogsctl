package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCollectionWrites(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen = append(seen, strings.TrimSpace(r.Method+" "+r.URL.Path+" "+string(body)))
		switch r.Method {
		case http.MethodGet:
			w.Write([]byte(`{"pagination": {"page": 1, "pages": 1, "urls": {}}, "releases": [
				{"id": 5077187, "instance_id": 11, "folder_id": 1}, {"id": 5077187, "instance_id": 22, "folder_id": 3}]}`))
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"instance_id": 33, "resource_url": "https://api.discogs.com/users/user/collection/folders/1/release/5077187/instance/33"}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	c := newTestClient(t, srv, "test-token")

	copies, err := c.CollectionInstances(t.Context(), "user", 5077187)
	if err != nil || len(copies) != 2 || copies[1].InstanceID != 22 || copies[1].FolderID != 3 {
		t.Fatalf("CollectionInstances = %+v, %v", copies, err)
	}
	i, err := c.AddToCollection(t.Context(), "user", UncategorizedFolder, 5077187)
	if err != nil || i.InstanceID != 33 {
		t.Fatalf("AddToCollection = %+v, %v", i, err)
	}
	if err := c.RemoveFromCollection(t.Context(), "user", 3, 5077187, 22); err != nil {
		t.Fatal(err)
	}
	want := "GET /users/user/collection/releases/5077187 | " +
		"POST /users/user/collection/folders/1/releases/5077187 | " +
		"DELETE /users/user/collection/folders/3/releases/5077187/instances/22"
	if got := strings.Join(seen, " | "); got != want {
		t.Errorf("requests = %q\nwant       %q", got, want)
	}

	seen = nil
	if _, err := c.AddToCollection(t.Context(), "user", AllFolder, 5077187); err == nil {
		t.Error("AddToCollection to folder 0 succeeded, want error")
	}
	if err := c.RemoveFromCollection(t.Context(), "user", 1, 5077187, 0); err == nil {
		t.Error("RemoveFromCollection of instance 0 succeeded, want error")
	}
	if _, err := c.CollectionInstances(t.Context(), "user", 0); err == nil {
		t.Error("CollectionInstances(0) succeeded, want error")
	}
	if len(seen) != 0 {
		t.Errorf("invalid arguments sent %q", seen)
	}
}
