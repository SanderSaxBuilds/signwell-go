package signwell

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		opts    []Option
		wantErr error
	}{
		{name: "valid key", key: "k"},
		{name: "empty key", key: "", wantErr: ErrMissingAPIKey},
		{name: "blank key", key: "  \t", wantErr: ErrMissingAPIKey},
		{name: "nil option is skipped", key: "k", opts: []Option{nil}},
		{name: "https base URL", key: "k", opts: []Option{WithBaseURL("https://api.example.test")}},
		{name: "base URL with path", key: "k", opts: []Option{WithBaseURL("http://localhost:8080/proxy/")}},
		{name: "ftp base URL", key: "k", opts: []Option{WithBaseURL("ftp://example.test")}, wantErr: ErrInvalidArgument},
		{name: "relative base URL", key: "k", opts: []Option{WithBaseURL("/api")}, wantErr: ErrInvalidArgument},
		{name: "base URL without host", key: "k", opts: []Option{WithBaseURL("https://")}, wantErr: ErrInvalidArgument},
		{name: "base URL with query", key: "k", opts: []Option{WithBaseURL("https://example.test?x=1")}, wantErr: ErrInvalidArgument},
		{name: "base URL with fragment", key: "k", opts: []Option{WithBaseURL("https://example.test#top")}, wantErr: ErrInvalidArgument},
		{name: "unparsable base URL", key: "k", opts: []Option{WithBaseURL("http://[::1")}, wantErr: ErrInvalidArgument},
		{name: "nil http client", key: "k", opts: []Option{WithHTTPClient(nil)}, wantErr: ErrInvalidArgument},
		{name: "custom http client", key: "k", opts: []Option{WithHTTPClient(&http.Client{})}},
		{name: "negative retries", key: "k", opts: []Option{WithMaxRetries(-1)}, wantErr: ErrInvalidArgument},
		{name: "zero retries", key: "k", opts: []Option{WithMaxRetries(0)}},
		{name: "zero initial backoff", key: "k", opts: []Option{WithRetryBackoff(0, time.Second)}, wantErr: ErrInvalidArgument},
		{name: "max below initial", key: "k", opts: []Option{WithRetryBackoff(time.Second, time.Millisecond)}, wantErr: ErrInvalidArgument},
		{name: "valid backoff", key: "k", opts: []Option{WithRetryBackoff(time.Millisecond, time.Second)}},
		{name: "negative max Retry-After", key: "k", opts: []Option{WithMaxRetryAfter(-time.Second)}, wantErr: ErrInvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(tt.key, tt.opts...)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				if c != nil {
					t.Fatal("client should be nil on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.Documents == nil || c.Templates == nil || c.Webhooks == nil {
				t.Fatal("services not set")
			}
		})
	}
}

func TestNewDefaults(t *testing.T) {
	c, err := New("k")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.BaseURL(); got != DefaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", got, DefaultBaseURL)
	}
	if c.retry.maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", c.retry.maxRetries)
	}
	if c.httpClient.Timeout != 60*time.Second {
		t.Errorf("timeout = %s, want 60s", c.httpClient.Timeout)
	}
	if c.testMode != nil {
		t.Error("test mode should be unset by default")
	}
}

func TestRequestHeaders(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusOK, "document.json"))
	if _, err := c.Documents.Get(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	h := api.last(t).Header
	checks := map[string]string{
		"X-Api-Key":  "test-api-key",
		"User-Agent": "signwell-go/" + Version,
		"Accept":     "application/json",
	}
	for k, want := range checks {
		if got := h.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if got := h.Get("Content-Type"); got != "" {
		t.Errorf("GET without body sent Content-Type %q", got)
	}
}

func TestRequestHeadersWithBody(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusCreated, "webhook_created.json"), WithUserAgent("acme-crm/2.1"))
	if _, err := c.Webhooks.Create(context.Background(), &CreateWebhookRequest{CallbackURL: "https://example.com/hook"}); err != nil {
		t.Fatal(err)
	}
	h := api.last(t).Header
	if got := h.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if got, want := h.Get("User-Agent"), "acme-crm/2.1 signwell-go/"+Version; got != want {
		t.Errorf("User-Agent = %q, want %q", got, want)
	}
}

func TestWithUserAgentBlankKeepsDefault(t *testing.T) {
	c, err := New("k", WithUserAgent("   "))
	if err != nil {
		t.Fatal(err)
	}
	if c.userAgent != "signwell-go/"+Version {
		t.Errorf("userAgent = %q", c.userAgent)
	}
}

func TestBaseURLWithPath(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusOK, "document.json"))
	c2, err := New("k", WithBaseURL(c.BaseURL()+"/proxy/"), WithHTTPClient(c.httpClient))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c2.Documents.Get(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	if got, want := api.last(t).Path, "/proxy/api/v1/documents/abc"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

func TestTestModeHeader(t *testing.T) {
	ctx := context.Background()
	calls := []struct {
		name       string
		fixture    string
		status     int
		call       func(c *Client) error
		wantHeader bool
	}{
		{"create", "document_created.json", 201, func(c *Client) error {
			_, err := c.Documents.Create(ctx, &CreateDocumentRequest{Files: []File{FileFromURL("a.pdf", "https://example.com/a.pdf")}, Recipients: []RecipientRequest{{ID: "1", Email: "a@example.com"}}})
			return err
		}, true},
		{"create from template", "document_from_template.json", 201, func(c *Client) error {
			_, err := c.Documents.CreateFromTemplate(ctx, &CreateDocumentFromTemplateRequest{TemplateID: "t", Recipients: []RecipientRequest{{ID: "1", PlaceholderName: "Client"}}})
			return err
		}, true},
		{"send", "document_sent.json", 201, func(c *Client) error {
			_, err := c.Documents.Send(ctx, "d", nil)
			return err
		}, true},
		{"get", "document.json", 200, func(c *Client) error {
			_, err := c.Documents.Get(ctx, "d")
			return err
		}, false},
		{"remind", "document_reminded.json", 201, func(c *Client) error {
			_, err := c.Documents.Remind(ctx, "d", nil)
			return err
		}, false},
		{"list", "documents_empty.json", 200, func(c *Client) error {
			_, err := c.Documents.List(ctx, nil)
			return err
		}, false},
	}
	for _, mode := range []bool{true, false} {
		for _, tc := range calls {
			name := tc.name
			if mode {
				name += "/on"
			} else {
				name += "/off"
			}
			t.Run(name, func(t *testing.T) {
				c, api := newTestClient(t, respond(t, tc.status, tc.fixture), WithTestMode(mode))
				if err := tc.call(c); err != nil {
					t.Fatal(err)
				}
				got := api.last(t).Header.Get(headerTestMode)
				want := ""
				if tc.wantHeader {
					want = "false"
					if mode {
						want = "true"
					}
				}
				if got != want {
					t.Errorf("%s = %q, want %q", headerTestMode, got, want)
				}
			})
		}
	}
}

func TestNoTestModeHeaderByDefault(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusCreated, "document_sent.json"))
	if _, err := c.Documents.Send(context.Background(), "d", nil); err != nil {
		t.Fatal(err)
	}
	if h := api.last(t).Header.Values(headerTestMode); len(h) != 0 {
		t.Errorf("unexpected header %v", h)
	}
}

func TestPathEscaping(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusOK, "document.json"))
	if _, err := c.Documents.Get(context.Background(), "a/b c?d"); err != nil {
		t.Fatal(err)
	}
	if got, want := api.last(t).Path, "/api/v1/documents/a%2Fb%20c%3Fd"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

func TestContextCanceledBeforeCall(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusOK, "document.json"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Documents.Get(ctx, "abc")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if n := len(api.Calls()); n != 0 {
		t.Errorf("%d requests sent with a canceled context", n)
	}
}

func TestContextDeadline(t *testing.T) {
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := c.Documents.Get(ctx, "abc")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestNilContext(t *testing.T) {
	c, _ := newTestClient(t, respond(t, http.StatusOK, "document.json"))
	_, err := c.Documents.Get(nilContext(), "abc")
	if err == nil || !strings.Contains(err.Error(), "nil context") {
		t.Fatalf("err = %v", err)
	}
}

// nilContext hides the nil from vet.
func nilContext() context.Context { return nil }

func TestDecodeErrorOnBadJSON(t *testing.T) {
	c, _ := newTestClient(t, respondBytes(http.StatusOK, []byte(`{"id": 12`)))
	_, err := c.Documents.Get(context.Background(), "abc")
	var de *DecodeError
	if !errors.As(err, &de) {
		t.Fatalf("err = %T %v, want *DecodeError", err, err)
	}
	if de.StatusCode != http.StatusOK || string(de.Body) != `{"id": 12` {
		t.Errorf("decode error = %+v", de)
	}
	if errors.Unwrap(de) == nil {
		t.Error("DecodeError should unwrap to the json error")
	}
}

func TestEncodeError(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusCreated, "document_created.json"))
	_, err := c.Documents.Create(context.Background(), &CreateDocumentRequest{
		Files:      []File{FileFromURL("a.pdf", "https://example.com/a.pdf")},
		Recipients: []RecipientRequest{{ID: "1", Email: "a@example.com"}},
		Fields:     [][]FieldRequest{{{Type: FieldText, Value: make(chan int)}}},
	})
	if err == nil || !strings.Contains(err.Error(), "encode request") {
		t.Fatalf("err = %v", err)
	}
	if len(api.Calls()) != 0 {
		t.Error("request sent despite encode failure")
	}
}

func TestEmptyIDs(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusOK, "document.json"))
	ctx := context.Background()
	calls := map[string]func() error{
		"Documents.Get":     func() error { _, err := c.Documents.Get(ctx, ""); return err },
		"Documents.Send":    func() error { _, err := c.Documents.Send(ctx, " ", nil); return err },
		"Documents.Remind":  func() error { _, err := c.Documents.Remind(ctx, "", nil); return err },
		"Documents.Delete":  func() error { return c.Documents.Delete(ctx, "") },
		"Documents.PDF":     func() error { _, err := c.Documents.DownloadCompletedPDF(ctx, "", nil); return err },
		"Documents.PDFURL":  func() error { _, err := c.Documents.CompletedPDFURL(ctx, "", nil); return err },
		"Templates.Get":     func() error { _, err := c.Templates.Get(ctx, ""); return err },
		"Webhooks.Delete":   func() error { return c.Webhooks.Delete(ctx, "") },
		"Documents.Get tab": func() error { _, err := c.Documents.Get(ctx, "\t"); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err = %v, want ErrInvalidArgument", err)
			}
		})
	}
	if n := len(api.Calls()); n != 0 {
		t.Errorf("%d requests sent for empty ids", n)
	}
}

func TestInvalidRequests(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusOK, "document.json"))
	ctx := context.Background()
	file := []File{FileFromURL("a.pdf", "https://example.com/a.pdf")}
	rcpt := []RecipientRequest{{ID: "1", Email: "a@example.com"}}
	calls := []struct {
		name string
		call func() error
	}{
		{"nil create", func() error { _, err := c.Documents.Create(ctx, nil); return err }},
		{"create without files", func() error {
			_, err := c.Documents.Create(ctx, &CreateDocumentRequest{Recipients: rcpt})
			return err
		}},
		{"create without recipients", func() error {
			_, err := c.Documents.Create(ctx, &CreateDocumentRequest{Files: file})
			return err
		}},
		{"nil create from template", func() error { _, err := c.Documents.CreateFromTemplate(ctx, nil); return err }},
		{"template id missing", func() error {
			_, err := c.Documents.CreateFromTemplate(ctx, &CreateDocumentFromTemplateRequest{Recipients: rcpt})
			return err
		}},
		{"template recipients missing", func() error {
			_, err := c.Documents.CreateFromTemplate(ctx, &CreateDocumentFromTemplateRequest{TemplateID: "t"})
			return err
		}},
		{"nil template create", func() error { _, err := c.Templates.Create(ctx, nil); return err }},
		{"template without files", func() error {
			_, err := c.Templates.Create(ctx, &CreateTemplateRequest{Placeholders: []PlaceholderRequest{{ID: "1", Name: "Client"}}})
			return err
		}},
		{"template without placeholders", func() error {
			_, err := c.Templates.Create(ctx, &CreateTemplateRequest{Files: file})
			return err
		}},
		{"nil webhook create", func() error { _, err := c.Webhooks.Create(ctx, nil); return err }},
		{"empty callback", func() error {
			_, err := c.Webhooks.Create(ctx, &CreateWebhookRequest{})
			return err
		}},
		{"relative callback", func() error {
			_, err := c.Webhooks.Create(ctx, &CreateWebhookRequest{CallbackURL: "/hooks/signwell"})
			return err
		}},
		{"non-http callback", func() error {
			_, err := c.Webhooks.Create(ctx, &CreateWebhookRequest{CallbackURL: "ftp://example.com/x"})
			return err
		}},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err = %v, want ErrInvalidArgument", err)
			}
		})
	}
	if n := len(api.Calls()); n != 0 {
		t.Errorf("%d requests sent for invalid input", n)
	}
}

func TestSleepContext(t *testing.T) {
	if err := sleepContext(context.Background(), 0); err != nil {
		t.Errorf("zero wait: %v", err)
	}
	if err := sleepContext(context.Background(), time.Millisecond); err != nil {
		t.Errorf("short wait: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepContext(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled wait: %v", err)
	}
	if err := sleepContext(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled zero wait: %v", err)
	}
}
