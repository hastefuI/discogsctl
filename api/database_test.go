package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRateRelease(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen = append(seen, strings.TrimSpace(r.Method+" "+r.URL.RequestURI()+" "+string(body)))
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"release_id": 8191071, "rating": 3, "username": "hasteful"}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv, "test-token")

	r, err := c.RateRelease(t.Context(), 8191071, "hasteful", 3)
	if err != nil || r.Rating != 3 || r.ReleaseID != 8191071 {
		t.Fatalf("RateRelease = %+v, %v", r, err)
	}
	if err := c.UnrateRelease(t.Context(), 8191071, "hasteful"); err != nil {
		t.Fatal(err)
	}
	// The rating must be a string: Discogs answers a JSON number with 422.
	want := `PUT /releases/8191071/rating/hasteful {"rating":"3"} | DELETE /releases/8191071/rating/hasteful`
	if got := strings.Join(seen, " | "); got != want {
		t.Errorf("requests = %q\nwant       %q", got, want)
	}

	seen = nil
	for _, bad := range []int{0, 6, -1} {
		if _, err := c.RateRelease(t.Context(), 8191071, "hasteful", bad); err == nil {
			t.Errorf("RateRelease(%d) succeeded, want error", bad)
		}
	}
	if len(seen) != 0 {
		t.Errorf("out of range ratings sent %q", seen)
	}
}
