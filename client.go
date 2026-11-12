package signwell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the SignWell API host. Paths under /api/v1 are added to it.
const DefaultBaseURL = "https://www.signwell.com"

const (
	apiPrefix      = "/api/v1"
	headerAPIKey   = "X-Api-Key"
	headerTestMode = "X-TestMode-Enabled"
)

// Client talks to the SignWell API. It is safe for concurrent use.
//
// The zero value is not usable, create one with [New].
type Client struct {
	// Documents creates, sends and manages documents.
	Documents *DocumentsService
	// Templates lists, reads and creates templates.
	Templates *TemplatesService
	// Webhooks registers and removes webhook callback URLs.
	Webhooks *WebhooksService

	apiKey     string
	baseURL    *url.URL
	httpClient *http.Client
	userAgent  string
	testMode   *bool
	retry      retryPolicy

	// sleep waits between retries. Tests replace it to avoid real waits.
	sleep func(ctx context.Context, d time.Duration) error
}

// New returns a client that authenticates with apiKey.
//
// It returns [ErrMissingAPIKey] when apiKey is empty, and an error when an
// option is invalid, for example a base URL that does not parse.
func New(apiKey string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, ErrMissingAPIKey
	}
	base, _ := url.Parse(DefaultBaseURL)
	c := &Client{
		apiKey:     apiKey,
		baseURL:    base,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		userAgent:  "signwell-go/" + Version,
		retry:      defaultRetryPolicy(),
		sleep:      sleepContext,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	c.Documents = &DocumentsService{client: c}
	c.Templates = &TemplatesService{client: c}
	c.Webhooks = &WebhooksService{client: c}
	return c, nil
}

// BaseURL returns the API host the client sends requests to.
func (c *Client) BaseURL() string { return c.baseURL.String() }

// request describes one API call before it is turned into an *http.Request.
type request struct {
	method string
	path   string
	query  url.Values
	body   any
	accept string
	// testModeHeader marks endpoints that document the X-TestMode-Enabled header.
	testModeHeader bool
}

// do sends the request and decodes a JSON response into out. A nil out
// discards the body.
func (c *Client) do(ctx context.Context, r *request, out any) error {
	resp, err := c.send(ctx, r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("signwell: read response: %w", err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return &DecodeError{StatusCode: resp.StatusCode, Body: data, Err: err}
	}
	return nil
}

// send performs the request with retries. On success the caller owns the
// response body. Any non-2xx response is returned as an *APIError.
func (c *Client) send(ctx context.Context, r *request) (*http.Response, error) {
	if ctx == nil {
		return nil, errors.New("signwell: nil context")
	}
	var payload []byte
	if r.body != nil {
		var err error
		payload, err = json.Marshal(r.body)
		if err != nil {
			return nil, fmt.Errorf("signwell: encode request: %w", err)
		}
	}

	for attempt := 0; ; attempt++ {
		req, err := c.newHTTPRequest(ctx, r, payload)
		if err != nil {
			return nil, err
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if attempt < c.retry.maxRetries && c.retry.retryTransport(r.method) {
				if err := c.sleep(ctx, c.retry.backoff(attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("signwell: %s %s: %w", r.method, r.path, err)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}

		apiErr := newAPIError(r.method, r.path, resp)
		if attempt < c.retry.maxRetries && c.retry.retryStatus(r.method, resp.StatusCode) {
			wait := c.retry.backoff(attempt)
			if apiErr.RetryAfter > 0 {
				if apiErr.RetryAfter > c.retry.maxRetryAfter {
					return nil, apiErr
				}
				wait = apiErr.RetryAfter
			}
			if err := c.sleep(ctx, wait); err != nil {
				return nil, err
			}
			continue
		}
		return nil, apiErr
	}
}

func (c *Client) newHTTPRequest(ctx context.Context, r *request, payload []byte) (*http.Request, error) {
	// r.path is already escaped segment by segment, see pathID.
	target := strings.TrimRight(c.baseURL.String(), "/") + apiPrefix + r.path
	if len(r.query) > 0 {
		target += "?" + r.query.Encode()
	}

	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, r.method, target, body)
	if err != nil {
		return nil, fmt.Errorf("signwell: build request: %w", err)
	}
	req.Header.Set(headerAPIKey, c.apiKey)
	req.Header.Set("User-Agent", c.userAgent)
	accept := r.accept
	if accept == "" {
		accept = "application/json"
	}
	req.Header.Set("Accept", accept)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.testModeHeader && c.testMode != nil {
		req.Header.Set(headerTestMode, strconv.FormatBool(*c.testMode))
	}
	return req, nil
}

// pathID escapes an identifier for use as one path segment.
func pathID(kind, id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", fmt.Errorf("%w: %s id is empty", ErrInvalidArgument, kind)
	}
	return url.PathEscape(id), nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
