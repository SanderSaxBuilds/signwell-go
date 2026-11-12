package signwell

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Option configures a [Client]. Pass options to [New].
type Option func(*Client) error

// WithBaseURL sends requests to another host, for example an httptest server.
// The URL must be absolute and must not carry a query string. The /api/v1
// prefix is added by the client.
func WithBaseURL(raw string) Option {
	return func(c *Client) error {
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("%w: base URL: %v", ErrInvalidArgument, err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("%w: base URL %q must start with http:// or https://", ErrInvalidArgument, raw)
		}
		if u.Host == "" {
			return fmt.Errorf("%w: base URL %q has no host", ErrInvalidArgument, raw)
		}
		if u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("%w: base URL %q must not have a query or fragment", ErrInvalidArgument, raw)
		}
		u.Path = strings.TrimRight(u.Path, "/")
		c.baseURL = u
		return nil
	}
}

// WithHTTPClient replaces the default *http.Client, which has a 60 second
// timeout. Use it to add a proxy, custom transport or tracing.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) error {
		if hc == nil {
			return fmt.Errorf("%w: nil *http.Client", ErrInvalidArgument)
		}
		c.httpClient = hc
		return nil
	}
}

// WithUserAgent puts your application name in front of the default
// User-Agent, so requests read like "myapp/1.2 signwell-go/0.1.0".
func WithUserAgent(ua string) Option {
	return func(c *Client) error {
		ua = strings.TrimSpace(ua)
		if ua != "" {
			c.userAgent = ua + " signwell-go/" + Version
		}
		return nil
	}
}

// WithTestMode sends the X-TestMode-Enabled header on the calls that accept
// it: create document, create document from template and send document.
// Test mode documents are free and not legally binding. When both the header
// and the test_mode body field are set, SignWell uses the header.
func WithTestMode(enabled bool) Option {
	return func(c *Client) error {
		c.testMode = &enabled
		return nil
	}
}

// WithMaxRetries sets how many times a failed request is retried. The
// default is 3. Zero turns retries off.
func WithMaxRetries(n int) Option {
	return func(c *Client) error {
		if n < 0 {
			return fmt.Errorf("%w: max retries must not be negative", ErrInvalidArgument)
		}
		c.retry.maxRetries = n
		return nil
	}
}

// WithRetryBackoff sets the first and the largest wait between retries. The
// wait doubles on each attempt, with jitter, between these bounds. The
// defaults are 500ms and 30s. A Retry-After header from the API takes
// precedence.
func WithRetryBackoff(initial, max time.Duration) Option {
	return func(c *Client) error {
		if initial <= 0 || max < initial {
			return fmt.Errorf("%w: retry backoff needs 0 < initial <= max, got %s and %s", ErrInvalidArgument, initial, max)
		}
		c.retry.initial = initial
		c.retry.max = max
		return nil
	}
}

// WithMaxRetryAfter sets the longest Retry-After wait the client accepts. If
// the API asks for a longer wait, the *APIError is returned at once instead.
// The default is 60s.
func WithMaxRetryAfter(d time.Duration) Option {
	return func(c *Client) error {
		if d < 0 {
			return fmt.Errorf("%w: max Retry-After must not be negative", ErrInvalidArgument)
		}
		c.retry.maxRetryAfter = d
		return nil
	}
}

// WithRetryNonIdempotent also retries POST and PATCH requests after a 5xx
// response or a network error. It is off by default, because a create call
// that failed with a 5xx may still have created the document, and a retry
// would create a second one. HTTP 429 is always retried, since the API
// rejected that request before doing any work.
func WithRetryNonIdempotent(enabled bool) Option {
	return func(c *Client) error {
		c.retry.nonIdempotent = enabled
		return nil
	}
}
