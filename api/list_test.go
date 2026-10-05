package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLists(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		if r.URL.Path == "/lists/100" {
			w.Write([]byte(`{"id": 100, "name": "Example List", "public": true, "date_changed": "2009-06-23T03:02:05-07:00",
				"user": {"id": 1, "username": "user"},
				"items": [{"id": 26694, "type": "release", "display_title": "Paolo Zerletti - Power", "comment": "opener", "stats": {}}]}`))
			return
		}
		w.Write([]byte(`{"pagination": {"page": 1, "pages": 1, "urls": {}}, "lists": [{"id": 94, "name": "Another List", "public": true}]}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv, "")

	lists, err := c.UserLists(t.Context(), "user", Page{PerPage: 100})
	if err != nil || len(lists.Items) != 1 || lists.Items[0].ID != 94 || len(lists.Items[0].Items) != 0 {
		t.Fatalf("UserLists = %+v, %v", lists, err)
	}
	l, err := c.List(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if l.User.Username != "user" || len(l.Items) != 1 || l.Items[0].Comment != "opener" || l.Items[0].Type != "release" {
		t.Errorf("list = %+v", l)
	}
	if want := "/users/user/lists?per_page=100 /lists/100"; strings.Join(seen, " ") != want {
		t.Errorf("requests = %q, want %q", seen, want)
	}
	if _, err := c.List(t.Context(), 0); err == nil {
		t.Error("List(0) succeeded, want error")
	}
}
