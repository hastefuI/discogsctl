package api

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// wantServer records each request as "METHOD URI BODY" and answers like the
// wantlist endpoints.
func wantServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if len(body) > 0 && r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s %s sent a body with Content-Type %q", r.Method, r.URL, r.Header.Get("Content-Type"))
		}
		seen = append(seen, strings.TrimSpace(r.Method+" "+r.URL.RequestURI()+" "+string(body)))
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id": 8191071, "notes": ""}`))
		case http.MethodPost:
			w.Write([]byte(`{"id": 8191071, "notes": "edited"}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestAddWant(t *testing.T) {
	srv, seen := wantServer(t)
	c := newTestClient(t, srv, "test-token")

	w, err := c.AddWant(t.Context(), "hasteful", 8191071, "")
	if err != nil || w.ID != 8191071 {
		t.Fatalf("AddWant = %+v, %v", w, err)
	}
	if got := strings.Join(*seen, " | "); got != "PUT /users/hasteful/wants/8191071" {
		t.Errorf("requests = %q, want one PUT with no body", got)
	}

	*seen = nil
	w, err = c.AddWant(t.Context(), "hasteful", 8191071, "first press")
	if err != nil || w.Notes != "edited" {
		t.Fatalf("AddWant with notes = %+v, %v; want the edited entry", w, err)
	}
	want := `PUT /users/hasteful/wants/8191071 | POST /users/hasteful/wants/8191071 {"notes":"first press"}`
	if got := strings.Join(*seen, " | "); got != want {
		t.Errorf("requests = %q\nwant       %q", got, want)
	}
}

func TestEditAndRemoveWant(t *testing.T) {
	srv, seen := wantServer(t)
	c := newTestClient(t, srv, "test-token")

	if _, err := c.EditWant(t.Context(), "hasteful", 8191071, ""); !errors.Is(err, ErrEmptyNotes) {
		t.Errorf("EditWant with empty notes = %v, want ErrEmptyNotes", err)
	}
	if _, err := c.EditWant(t.Context(), "hasteful", 8191071, "any pressing"); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveWant(t.Context(), "hasteful", 8191071); err != nil {
		t.Fatalf("RemoveWant with a 204 and no body: %v", err)
	}
	want := `POST /users/hasteful/wants/8191071 {"notes":"any pressing"} | DELETE /users/hasteful/wants/8191071`
	if got := strings.Join(*seen, " | "); got != want {
		t.Errorf("requests = %q\nwant       %q", got, want)
	}

	*seen = nil
	if _, err := c.EditWant(t.Context(), "hasteful", 0, "x"); err == nil {
		t.Error("EditWant(0) succeeded, want error")
	}
	if err := c.RemoveWant(t.Context(), "hasteful", -1); err == nil {
		t.Error("RemoveWant(-1) succeeded, want error")
	}
	if len(*seen) != 0 {
		t.Errorf("bad IDs sent %q", *seen)
	}
}
