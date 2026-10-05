package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// fakeDiscogs is an in-memory Discogs that enforces a moving 60 second window
// the way the docs describe, and answers 429 once a request would exceed it.
// Inside a synctest bubble its clock is the bubble's, so minutes pass
// instantly.
type fakeDiscogs struct {
	mu         sync.Mutex
	limit      int
	usedByAny  int // requests made by another process, counted in the window
	alwaysBusy bool
	times      []time.Time
	sent       []time.Time
	throttled  int
}

func (f *fakeDiscogs) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	f.sent = append(f.sent, now)
	cutoff := now.Add(-window)
	for len(f.times) > 0 && !f.times[0].After(cutoff) {
		f.times = f.times[1:]
	}

	status := http.StatusOK
	body := `{"id": 1}`
	if f.alwaysBusy || len(f.times)+f.usedByAny >= f.limit {
		status = http.StatusTooManyRequests
		body = `{"message": "You are making requests too quickly."}`
		f.throttled++
	} else {
		f.times = append(f.times, now)
	}
	used := min(len(f.times)+f.usedByAny, f.limit)

	h := http.Header{}
	h.Set("X-Discogs-Ratelimit", strconv.Itoa(f.limit))
	h.Set("X-Discogs-Ratelimit-Used", strconv.Itoa(used))
	h.Set("X-Discogs-Ratelimit-Remaining", strconv.Itoa(f.limit-used))
	return &http.Response{
		StatusCode: status,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

// maxInWindow is the most requests the server saw inside any 60 second span.
func (f *fakeDiscogs) maxInWindow() int {
	most := 0
	for i, start := range f.sent {
		n := 0
		for _, t := range f.sent[i:] {
			if t.Sub(start) < window {
				n++
			}
		}
		most = max(most, n)
	}
	return most
}

func newFakeClient(t *testing.T, f *fakeDiscogs, token string) *Client {
	t.Helper()
	c, err := New(Options{UserAgent: testAgent, Token: token, HTTP: &http.Client{Transport: f}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLimiterWaitsInsteadOfBursting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeDiscogs{limit: authenticatedBudget}
		c := newFakeClient(t, f, "tok")

		start := time.Now()
		const requests = 150
		for i := range requests {
			if _, err := c.Release(t.Context(), 1); err != nil {
				t.Fatalf("request %d: %v", i+1, err)
			}
		}
		elapsed := time.Since(start)

		if f.throttled != 0 {
			t.Errorf("server answered 429 %d times, want 0", f.throttled)
		}
		if got := len(f.sent); got != requests {
			t.Errorf("server saw %d requests, want %d", got, requests)
		}
		// One in reserve leaves 59 a window: 150 requests span three windows.
		if most := f.maxInWindow(); most > authenticatedBudget-1 {
			t.Errorf("server saw %d requests in one window, want at most %d", most, authenticatedBudget-1)
		}
		if elapsed < 2*window {
			t.Errorf("150 requests took %v, want at least %v of waiting", elapsed, 2*window)
		}
	})
}

func TestLimiterStartsAtTheAnonymousBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// The server allows 60, but the client has no token and must not
		// assume more than 25 before the first response says otherwise.
		f := &fakeDiscogs{limit: authenticatedBudget}
		c := newFakeClient(t, f, "")
		if c.limit.limit != anonymousBudget {
			t.Fatalf("anonymous limit = %d, want %d", c.limit.limit, anonymousBudget)
		}
		c2 := newFakeClient(t, f, "tok")
		if c2.limit.limit != authenticatedBudget {
			t.Fatalf("authenticated limit = %d, want %d", c2.limit.limit, authenticatedBudget)
		}
	})
}

func TestLimiterFollowsTheServerLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// The server has lowered the limit below the documented 60.
		f := &fakeDiscogs{limit: 10}
		c := newFakeClient(t, f, "tok")

		for i := range 30 {
			if _, err := c.Release(t.Context(), 1); err != nil {
				t.Fatalf("request %d: %v", i+1, err)
			}
		}
		if f.throttled != 0 {
			t.Errorf("server answered 429 %d times, want 0", f.throttled)
		}
		if most := f.maxInWindow(); most > 9 {
			t.Errorf("server saw %d requests in one window, want at most 9", most)
		}
	})
}

func TestLimiterRespectsRequestsFromElsewhere(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Another process has already used most of the window.
		f := &fakeDiscogs{limit: authenticatedBudget, usedByAny: 57}
		c := newFakeClient(t, f, "tok")

		start := time.Now()
		for i := range 3 {
			if _, err := c.Release(t.Context(), 1); err != nil {
				t.Fatalf("request %d: %v", i+1, err)
			}
		}
		if f.throttled != 0 {
			t.Errorf("server answered 429 %d times, want 0", f.throttled)
		}
		// The first response leaves 2, so the second request goes. That
		// leaves 1, and the third waits for the window rather than spend it.
		if elapsed := time.Since(start); elapsed < window {
			t.Errorf("elapsed %v, want a wait for the window", elapsed)
		}
	})
}

func TestTooManyRequestsWaitsOnceThenFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeDiscogs{limit: authenticatedBudget, alwaysBusy: true}
		c := newFakeClient(t, f, "tok")

		start := time.Now()
		_, err := c.Release(t.Context(), 1)
		apiErr, ok := errors.AsType[*Error](err)
		if !ok || apiErr.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("error = %v, want a 429 *Error", err)
		}
		if got := len(f.sent); got != 2 {
			t.Errorf("server saw %d requests, want 2: one, then one retry after the window", got)
		}
		if elapsed := time.Since(start); elapsed < window {
			t.Errorf("retry came after %v, want at least %v", elapsed, window)
		}
	})
}

func TestLimiterWaitHonoursCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newLimiter(true, slog.New(slog.DiscardHandler))
		l.blockedUntil = time.Now().Add(window)

		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			time.Sleep(time.Second)
			cancel()
		}()
		if err := l.wait(ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("wait error = %v, want context.Canceled", err)
		}
	})
}

func TestClientRateLimit(t *testing.T) {
	var headers atomicHeaders
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range headers.get() {
			w.Header().Set(k, v)
		}
		w.Write([]byte(`{"id": 1}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv, "test-token")

	if got := c.RateLimit(); got != (RateLimit{}) {
		t.Errorf("RateLimit before any response = %+v, want the zero value", got)
	}

	headers.set(map[string]string{"X-Discogs-Ratelimit": "60", "X-Discogs-Ratelimit-Used": "2", "X-Discogs-Ratelimit-Remaining": "58"})
	before := time.Now()
	if _, err := c.Release(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	got := c.RateLimit()
	if got.Limit != 60 || got.Used != 2 || got.Remaining != 58 || got.Observed.Before(before) {
		t.Errorf("RateLimit = %+v, want 60, 2, 58 observed after the request", got)
	}

	headers.set(nil)
	if _, err := c.Release(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if again := c.RateLimit(); again != got {
		t.Errorf("RateLimit after a response without headers = %+v, want it unchanged: %+v", again, got)
	}
}

func TestClientRateLimitConcurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Discogs-Ratelimit", "60")
		w.Header().Set("X-Discogs-Ratelimit-Remaining", "50")
		w.Write([]byte(`{"id": 1}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv, "test-token")

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if _, err := c.Release(t.Context(), 1); err != nil {
				t.Error(err)
			}
		})
		wg.Go(func() { _ = c.RateLimit() })
	}
	wg.Wait()
	if got := c.RateLimit(); got.Limit != 60 || got.Remaining != 50 {
		t.Errorf("RateLimit = %+v", got)
	}
}

// atomicHeaders holds the headers a test server sends, changed between
// requests.
type atomicHeaders struct {
	mu sync.Mutex
	h  map[string]string
}

func (a *atomicHeaders) set(h map[string]string) { a.mu.Lock(); a.h = h; a.mu.Unlock() }
func (a *atomicHeaders) get() map[string]string  { a.mu.Lock(); defer a.mu.Unlock(); return a.h }
