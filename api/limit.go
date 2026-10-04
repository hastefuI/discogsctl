package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	// window is the span Discogs averages requests over. A window with no
	// requests in it resets.
	window = time.Minute

	anonymousBudget     = 25
	authenticatedBudget = 60
)

// limiter throttles requests to the Discogs window locally, so the client
// does not run into 429s. It starts from the documented budget for the
// credentials in use and switches to the X-Discogs-Ratelimit headers once a
// response arrives, since Discogs says the numbers can change. It always
// keeps one request in reserve.
type limiter struct {
	mu sync.Mutex
	// limit is the number of requests allowed per window.
	limit int
	// remaining is the server's remaining count, kept current between
	// responses by subtracting each send and adding back each send that
	// leaves the window. It is -1 when unknown or older than a window.
	remaining int
	// observed is when remaining was last read from a response.
	observed time.Time
	// blockedUntil holds every request after a 429 until the window passes.
	blockedUntil time.Time
	// sent holds the send times within the last window, oldest first.
	sent []time.Time
	log  *slog.Logger
}

func newLimiter(authenticated bool, log *slog.Logger) *limiter {
	limit := anonymousBudget
	if authenticated {
		limit = authenticatedBudget
	}
	return &limiter{limit: limit, remaining: -1, log: log}
}

// wait blocks until a request may be sent without leaving the window with
// fewer than one request remaining, then records the send.
func (l *limiter) wait(ctx context.Context) error {
	for {
		delay := l.reserve(time.Now())
		if delay <= 0 {
			return nil
		}
		l.log.DebugContext(ctx, "waiting for rate limit", "delay", delay.Round(time.Millisecond))
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return context.Cause(ctx)
		case <-timer.C:
		}
	}
}

// reserve records a send at now and returns zero when a request may go, or
// returns how long to wait before asking again.
func (l *limiter) reserve(now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.expire(now)

	if d := l.blockedUntil.Sub(now); d > 0 {
		return d
	}
	if len(l.sent) >= max(l.limit-1, 1) {
		return l.sent[0].Add(window).Sub(now)
	}
	if l.remaining >= 0 && l.remaining <= 1 {
		if len(l.sent) > 0 {
			return l.sent[0].Add(window).Sub(now)
		}
		// Another process used the window. Wait for it to go stale.
		return l.observed.Add(window).Sub(now)
	}

	l.sent = append(l.sent, now)
	if l.remaining > 0 {
		l.remaining--
	}
	return 0
}

// expire drops sends that have left the window, crediting each back to the
// remaining count, and forgets a remaining count older than the window.
func (l *limiter) expire(now time.Time) {
	cutoff := now.Add(-window)
	n := 0
	for n < len(l.sent) && !l.sent[n].After(cutoff) {
		n++
	}
	l.sent = l.sent[n:]
	if l.remaining < 0 {
		return
	}
	if !l.observed.After(cutoff) {
		l.remaining = -1
		return
	}
	l.remaining = min(l.remaining+n, l.limit)
}

// observe reads the rate limit headers from a response. Values that are
// missing or malformed leave the current state alone.
func (l *limiter) observe(h http.Header, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if v, err := strconv.Atoi(h.Get("X-Discogs-Ratelimit")); err == nil && v > 0 {
		l.limit = v
	}
	if v, err := strconv.Atoi(h.Get("X-Discogs-Ratelimit-Remaining")); err == nil && v >= 0 {
		l.remaining = min(v, l.limit)
		l.observed = now
	}
}

// backoff holds every request for a full window after a 429.
func (l *limiter) backoff(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.blockedUntil = now.Add(window)
}
