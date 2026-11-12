package signwell

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testWebhookID = "6ff542d5-590c-44b2-8131-7ae998e78c5b"

// apiCall is one request as the fake API saw it.
type apiCall struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   []byte
}

func (c apiCall) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(c.Body, &m); err != nil {
		t.Fatalf("request body is not a JSON object: %v\n%s", err, c.Body)
	}
	return m
}

// testAPI records the requests a client sends and the waits it would sleep.
type testAPI struct {
	mu    sync.Mutex
	calls []apiCall
	waits []time.Duration
}

func (a *testAPI) Calls() []apiCall {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]apiCall(nil), a.calls...)
}

func (a *testAPI) Waits() []time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]time.Duration(nil), a.waits...)
}

func (a *testAPI) last(t *testing.T) apiCall {
	t.Helper()
	calls := a.Calls()
	if len(calls) == 0 {
		t.Fatal("no request reached the server")
	}
	return calls[len(calls)-1]
}

// newTestClient starts an httptest server running h and returns a client
// pointed at it. Retry waits are recorded instead of slept.
func newTestClient(t *testing.T, h http.HandlerFunc, opts ...Option) (*Client, *testAPI) {
	t.Helper()
	api := &testAPI{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		api.mu.Lock()
		api.calls = append(api.calls, apiCall{
			Method: r.Method,
			Path:   r.URL.EscapedPath(),
			Query:  r.URL.RawQuery,
			Header: r.Header.Clone(),
			Body:   body,
		})
		api.mu.Unlock()
		r.Body = io.NopCloser(bytes.NewReader(body))
		h(w, r)
	}))
	t.Cleanup(srv.Close)

	all := append([]Option{WithBaseURL(srv.URL)}, opts...)
	c, err := New("test-api-key", all...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.sleep = func(ctx context.Context, d time.Duration) error {
		api.mu.Lock()
		api.waits = append(api.waits, d)
		api.mu.Unlock()
		return ctx.Err()
	}
	c.retry.jitter = func() float64 { return 0.5 }
	return c, api
}

// fixture reads a file from testdata.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

// respond answers with a status and a testdata JSON file. An empty name
// sends no body.
func respond(t *testing.T, status int, name string, headers ...string) http.HandlerFunc {
	t.Helper()
	var body []byte
	if name != "" {
		body = fixture(t, name)
	}
	return respondBytes(status, body, headers...)
}

// respondBytes answers with a status, a body and header pairs.
func respondBytes(status int, body []byte, headers ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(body) > 0 {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		for i := 0; i+1 < len(headers); i += 2 {
			w.Header().Set(headers[i], headers[i+1])
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}
}

// sequence runs the nth handler on the nth request and repeats the last one.
func sequence(handlers ...http.HandlerFunc) http.HandlerFunc {
	var n atomic.Int64
	return func(w http.ResponseWriter, r *http.Request) {
		i := int(n.Add(1)) - 1
		if i >= len(handlers) {
			i = len(handlers) - 1
		}
		handlers[i](w, r)
	}
}

// byPage answers list requests from the page query parameter.
func byPage(pages map[string]http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("page")
		if p == "" {
			p = "1"
		}
		h, ok := pages[p]
		if !ok {
			http.Error(w, "unexpected page "+p, http.StatusTeapot)
			return
		}
		h(w, r)
	}
}
