package signwell

import (
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type retryPolicy struct {
	maxRetries    int
	initial       time.Duration
	max           time.Duration
	maxRetryAfter time.Duration
	nonIdempotent bool
	// jitter returns a number in [0, 1). Tests pin it.
	jitter func() float64
}

func defaultRetryPolicy() retryPolicy {
	return retryPolicy{
		maxRetries:    3,
		initial:       500 * time.Millisecond,
		max:           30 * time.Second,
		maxRetryAfter: 60 * time.Second,
		jitter:        rand.Float64,
	}
}

// idempotent reports whether repeating the method cannot cause a second side
// effect on the server.
func idempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

// retryStatus reports whether a response with this status is worth retrying.
func (p retryPolicy) retryStatus(method string, status int) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	if status >= 500 && status <= 599 && status != http.StatusNotImplemented {
		return idempotent(method) || p.nonIdempotent
	}
	return false
}

// retryTransport reports whether a network error is worth retrying.
func (p retryPolicy) retryTransport(method string) bool {
	return idempotent(method) || p.nonIdempotent
}

// backoff returns the wait before retry number attempt+1: the doubled delay,
// capped at max, with the upper half randomised.
func (p retryPolicy) backoff(attempt int) time.Duration {
	d := p.initial
	for i := 0; i < attempt && d < p.max; i++ {
		d *= 2
	}
	if d > p.max {
		d = p.max
	}
	j := 0.5
	if p.jitter != nil {
		j = p.jitter()
	}
	half := d / 2
	return half + time.Duration(float64(half)*j)
}

// parseRetryAfter reads a Retry-After header given in seconds or as an HTTP
// date. It returns zero when the header is absent or unreadable.
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.ParseFloat(v, 64); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs * float64(time.Second))
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}
