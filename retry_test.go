package signwell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryStatus(t *testing.T) {
	tests := []struct {
		method        string
		status        int
		nonIdempotent bool
		want          bool
	}{
		{http.MethodGet, 429, false, true},
		{http.MethodPost, 429, false, true},
		{http.MethodPatch, 429, false, true},
		{http.MethodGet, 500, false, true},
		{http.MethodGet, 502, false, true},
		{http.MethodGet, 503, false, true},
		{http.MethodGet, 504, false, true},
		{http.MethodDelete, 503, false, true},
		{http.MethodPut, 500, false, true},
		{http.MethodGet, 501, false, false},
		{http.MethodPost, 500, false, false},
		{http.MethodPost, 503, false, false},
		{http.MethodPatch, 502, false, false},
		{http.MethodPost, 503, true, true},
		{http.MethodPatch, 500, true, true},
		{http.MethodGet, 400, false, false},
		{http.MethodGet, 401, false, false},
		{http.MethodGet, 404, false, false},
		{http.MethodGet, 409, false, false},
		{http.MethodPost, 422, true, false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%d_%v", tt.method, tt.status, tt.nonIdempotent), func(t *testing.T) {
			p := defaultRetryPolicy()
			p.nonIdempotent = tt.nonIdempotent
			if got := p.retryStatus(tt.method, tt.status); got != tt.want {
				t.Errorf("retryStatus = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBackoff(t *testing.T) {
	p := defaultRetryPolicy()
	p.initial = 100 * time.Millisecond
	p.max = time.Second
	tests := []struct {
		attempt int
		jitter  float64
		want    time.Duration
	}{
		{0, 0, 50 * time.Millisecond},
		{0, 0.999999, 100 * time.Millisecond},
		{1, 0, 100 * time.Millisecond},
		{2, 0, 200 * time.Millisecond},
		{3, 0, 400 * time.Millisecond},
		{4, 0, 500 * time.Millisecond},
		{5, 0, 500 * time.Millisecond},
		{30, 0.5, 750 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt%d_jitter%.1f", tt.attempt, tt.jitter), func(t *testing.T) {
			p.jitter = func() float64 { return tt.jitter }
			got := p.backoff(tt.attempt)
			if diff := got - tt.want; diff < -time.Millisecond || diff > time.Millisecond {
				t.Errorf("backoff = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestBackoffDefaultJitterInRange(t *testing.T) {
	p := defaultRetryPolicy()
	for i := 0; i < 100; i++ {
		d := p.backoff(1)
		if d < 500*time.Millisecond || d > time.Second {
			t.Fatalf("backoff(1) = %s, outside [500ms, 1s]", d)
		}
	}
	p.jitter = nil
	if d := p.backoff(0); d != 375*time.Millisecond {
		t.Errorf("backoff without jitter func = %s", d)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{"absent", "", 0},
		{"seconds", "5", 5 * time.Second},
		{"padded", "  7 ", 7 * time.Second},
		{"fraction", "1.5", 1500 * time.Millisecond},
		{"zero", "0", 0},
		{"negative", "-3", 0},
		{"http date ahead", now.Add(30 * time.Second).Format(http.TimeFormat), 30 * time.Second},
		{"http date passed", now.Add(-time.Minute).Format(http.TimeFormat), 0},
		{"garbage", "soon", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRetryAfter(tt.value, now); got != tt.want {
				t.Errorf("parseRetryAfter(%q) = %s, want %s", tt.value, got, tt.want)
			}
		})
	}
}

func TestRetryOn429ThenSuccess(t *testing.T) {
	c, api := newTestClient(t, sequence(
		respond(t, http.StatusTooManyRequests, "error_rate_limit.json", "Retry-After", "2"),
		respond(t, http.StatusTooManyRequests, "error_rate_limit.json"),
		respond(t, http.StatusOK, "document.json"),
	))
	doc, err := c.Documents.Get(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if doc.ID == "" {
		t.Error("empty document")
	}
	if n := len(api.Calls()); n != 3 {
		t.Errorf("%d requests, want 3", n)
	}
	// First wait honours Retry-After, the second falls back to backoff(1).
	want := []time.Duration{2 * time.Second, 750 * time.Millisecond}
	if got := api.Waits(); !reflect.DeepEqual(got, want) {
		t.Errorf("waits = %v, want %v", got, want)
	}
}

func TestRetryOn429ForPost(t *testing.T) {
	c, api := newTestClient(t, sequence(
		respond(t, http.StatusTooManyRequests, "error_rate_limit.json", "Retry-After", "1"),
		respond(t, http.StatusCreated, "document_created.json"),
	))
	_, err := c.Documents.Create(context.Background(), &CreateDocumentRequest{
		Files:      []File{FileFromURL("a.pdf", "https://example.com/a.pdf")},
		Recipients: []RecipientRequest{{ID: "1", Email: "a@example.com"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := api.Calls()
	if len(calls) != 2 {
		t.Fatalf("%d requests, want 2", len(calls))
	}
	if !bytes.Equal(calls[0].Body, calls[1].Body) || len(calls[1].Body) == 0 {
		t.Errorf("retried body differs:\n%s\n%s", calls[0].Body, calls[1].Body)
	}
}

func TestRetryOn5xxForGet(t *testing.T) {
	for _, status := range []int{500, 502, 503, 504} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, api := newTestClient(t, sequence(
				respondBytes(status, []byte("upstream error")),
				respond(t, http.StatusOK, "webhooks.json"),
			))
			if _, err := c.Webhooks.List(context.Background()); err != nil {
				t.Fatal(err)
			}
			if n := len(api.Calls()); n != 2 {
				t.Errorf("%d requests, want 2", n)
			}
		})
	}
}

func TestNoRetryOn5xxForPostByDefault(t *testing.T) {
	c, api := newTestClient(t, sequence(
		respondBytes(http.StatusServiceUnavailable, nil),
		respond(t, http.StatusCreated, "webhook_created.json"),
	))
	_, err := c.Webhooks.Create(context.Background(), &CreateWebhookRequest{CallbackURL: "https://example.com/h"})
	if !errors.Is(err, ErrServer) {
		t.Fatalf("err = %v, want ErrServer", err)
	}
	if n := len(api.Calls()); n != 1 {
		t.Errorf("%d requests, POST must not be retried on 5xx", n)
	}
}

func TestRetryNonIdempotentOption(t *testing.T) {
	c, api := newTestClient(t, sequence(
		respondBytes(http.StatusServiceUnavailable, nil),
		respond(t, http.StatusCreated, "webhook_created.json"),
	), WithRetryNonIdempotent(true))
	if _, err := c.Webhooks.Create(context.Background(), &CreateWebhookRequest{CallbackURL: "https://example.com/h"}); err != nil {
		t.Fatal(err)
	}
	if n := len(api.Calls()); n != 2 {
		t.Errorf("%d requests, want 2", n)
	}
}

func TestRetriesExhausted(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries int
		wantCalls  int
	}{
		{"default", -1, 4},
		{"off", 0, 1},
		{"one", 1, 2},
		{"five", 5, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []Option
			if tt.maxRetries >= 0 {
				opts = append(opts, WithMaxRetries(tt.maxRetries))
			}
			c, api := newTestClient(t, respondBytes(http.StatusBadGateway, []byte("bad gateway")), opts...)
			_, err := c.Documents.Get(context.Background(), "abc")
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadGateway {
				t.Fatalf("err = %v", err)
			}
			if apiErr.Message != "bad gateway" {
				t.Errorf("message = %q", apiErr.Message)
			}
			if n := len(api.Calls()); n != tt.wantCalls {
				t.Errorf("%d requests, want %d", n, tt.wantCalls)
			}
			if n := len(api.Waits()); n != tt.wantCalls-1 {
				t.Errorf("%d waits, want %d", n, tt.wantCalls-1)
			}
		})
	}
}

func TestRetryAfterAboveCap(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusTooManyRequests, "error_rate_limit.json", "Retry-After", "120"))
	_, err := c.Documents.Get(context.Background(), "abc")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v", err)
	}
	if apiErr.RetryAfter != 120*time.Second {
		t.Errorf("RetryAfter = %s", apiErr.RetryAfter)
	}
	if !strings.HasPrefix(apiErr.Message, "Rate limit exceeded") {
		t.Errorf("message = %q", apiErr.Message)
	}
	if n := len(api.Calls()); n != 1 {
		t.Errorf("%d requests, a wait above the cap must not be retried", n)
	}
}

func TestMaxRetryAfterOption(t *testing.T) {
	c, api := newTestClient(t, sequence(
		respond(t, http.StatusTooManyRequests, "error_rate_limit.json", "Retry-After", "120"),
		respond(t, http.StatusOK, "document.json"),
	), WithMaxRetryAfter(5*time.Minute))
	if _, err := c.Documents.Get(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	if got := api.Waits(); len(got) != 1 || got[0] != 120*time.Second {
		t.Errorf("waits = %v", got)
	}
}

func TestRetryWaitCanceled(t *testing.T) {
	c, api := newTestClient(t, respondBytes(http.StatusServiceUnavailable, nil))
	c.sleep = sleepContext
	c.retry.initial = time.Hour
	c.retry.max = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for len(api.Calls()) == 0 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()
	_, err := c.Documents.Get(ctx, "abc")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// flakyTransport fails the first n round trips with a network error.
type flakyTransport struct {
	fails atomic.Int64
	next  http.RoundTripper
	seen  atomic.Int64
}

func (f *flakyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.seen.Add(1)
	if f.fails.Add(-1) >= 0 {
		if r.Body != nil {
			_, _ = io.Copy(io.Discard, r.Body)
			r.Body.Close()
		}
		return nil, errors.New("connection reset by peer")
	}
	return f.next.RoundTrip(r)
}

func TestRetryTransportErrors(t *testing.T) {
	tests := []struct {
		name          string
		post          bool
		nonIdempotent bool
		wantErr       bool
		wantSeen      int64
	}{
		{"GET is retried", false, false, false, 3},
		{"POST is not retried", true, false, true, 1},
		{"POST is retried when allowed", true, true, false, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft := &flakyTransport{next: http.DefaultTransport}
			ft.fails.Store(2)
			fixtureName, status := "document.json", http.StatusOK
			if tt.post {
				fixtureName, status = "webhook_created.json", http.StatusCreated
			}
			c, _ := newTestClient(t, respond(t, status, fixtureName),
				WithHTTPClient(&http.Client{Transport: ft}),
				WithRetryNonIdempotent(tt.nonIdempotent))
			var err error
			if tt.post {
				_, err = c.Webhooks.Create(context.Background(), &CreateWebhookRequest{CallbackURL: "https://example.com/h"})
			} else {
				_, err = c.Documents.Get(context.Background(), "abc")
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "connection reset") {
				t.Errorf("err = %v", err)
			}
			if got := ft.seen.Load(); got != tt.wantSeen {
				t.Errorf("round trips = %d, want %d", got, tt.wantSeen)
			}
		})
	}
}
