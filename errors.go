package signwell

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Errors that a call can return before any request is sent.
var (
	// ErrMissingAPIKey is returned by New when the API key is empty.
	ErrMissingAPIKey = errors.New("signwell: API key is empty")
	// ErrInvalidArgument marks a bad argument, such as an empty id or a nil request.
	ErrInvalidArgument = errors.New("signwell: invalid argument")
)

// Sentinel errors for API responses. An *APIError matches the one for its
// status code through errors.Is.
var (
	ErrBadRequest          = errors.New("signwell: bad request")          // 400
	ErrUnauthorized        = errors.New("signwell: unauthorized")         // 401
	ErrForbidden           = errors.New("signwell: forbidden")            // 403
	ErrNotFound            = errors.New("signwell: not found")            // 404
	ErrConflict            = errors.New("signwell: conflict")             // 409
	ErrUnprocessableEntity = errors.New("signwell: unprocessable entity") // 422
	ErrRateLimited         = errors.New("signwell: rate limited")         // 429
	ErrServer              = errors.New("signwell: server error")         // 5xx
)

// maxErrorBody caps how much of an error response is kept in memory.
const maxErrorBody = 1 << 20

// APIError is returned when SignWell answers with a status outside 2xx.
//
// SignWell uses three error shapes. Not found and auth errors carry a
// message and a meta object, validation errors (400 and 422) carry an errors
// object keyed by field name, and rate limit errors (429) carry one error
// string. APIError reads all three.
type APIError struct {
	// StatusCode is the HTTP status, for example 422.
	StatusCode int
	// Method and Path identify the call, for example "GET" and "/documents/abc".
	Method string
	Path   string

	// Message is the most specific human readable message found in the body.
	Message string
	// Code is the machine readable code from meta.error, for example
	// "record_not_found". It is empty for validation and rate limit errors.
	Code string
	// Messages lists meta.messages when the API sends them.
	Messages []string
	// Errors holds the raw errors object of a validation error. Values are
	// strings, lists of strings or nested objects. FieldErrors flattens it.
	Errors map[string]any

	// RetryAfter is the wait the API asked for in the Retry-After header.
	RetryAfter time.Duration
	// RateLimit holds the X-RateLimit-* headers when the response has them.
	RateLimit *RateLimit

	// Header and Body are the raw response, kept for logging.
	Header http.Header
	Body   []byte
}

// RateLimit is read from the X-RateLimit-Limit, X-RateLimit-Remaining and
// X-RateLimit-Reset response headers.
type RateLimit struct {
	Limit     int
	Remaining int
	// Reset is when the window resets. It is zero when the header is absent.
	Reset time.Time
}

// Error implements the error interface.
func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "signwell: %s %s: %d %s", e.Method, e.Path, e.StatusCode, http.StatusText(e.StatusCode))
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	if e.Code != "" {
		fmt.Fprintf(&b, " (%s)", e.Code)
	}
	return b.String()
}

// Is lets errors.Is match an *APIError against the status sentinels.
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrBadRequest:
		return e.StatusCode == http.StatusBadRequest
	case ErrUnauthorized:
		return e.StatusCode == http.StatusUnauthorized
	case ErrForbidden:
		return e.StatusCode == http.StatusForbidden
	case ErrNotFound:
		return e.StatusCode == http.StatusNotFound
	case ErrConflict:
		return e.StatusCode == http.StatusConflict
	case ErrUnprocessableEntity:
		return e.StatusCode == http.StatusUnprocessableEntity
	case ErrRateLimited:
		return e.StatusCode == http.StatusTooManyRequests
	case ErrServer:
		return e.StatusCode >= 500 && e.StatusCode <= 599
	}
	return false
}

// Temporary reports whether the same request may succeed later: rate limits
// and server errors.
func (e *APIError) Temporary() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

// FieldErrors flattens the validation errors into one list of messages per
// field. Nested keys are joined with dots, so a bad recipient email reads as
// "recipients.recipient_1.email". It returns nil when there are none.
func (e *APIError) FieldErrors() map[string][]string {
	if len(e.Errors) == 0 {
		return nil
	}
	out := make(map[string][]string)
	flattenErrors("", e.Errors, out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func flattenErrors(prefix string, v any, out map[string][]string) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flattenErrors(key, child, out)
		}
	case []any:
		for _, item := range t {
			switch it := item.(type) {
			case map[string]any, []any:
				flattenErrors(prefix, it, out)
			default:
				out[prefix] = append(out[prefix], fmt.Sprint(it))
			}
		}
	case nil:
	default:
		out[prefix] = append(out[prefix], fmt.Sprint(t))
	}
}

// DecodeError is returned when a 2xx response body is not the JSON the
// client expected.
type DecodeError struct {
	StatusCode int
	Body       []byte
	Err        error
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("signwell: decode %d response: %v", e.StatusCode, e.Err)
}

func (e *DecodeError) Unwrap() error { return e.Err }

// errorBody covers every documented error shape.
type errorBody struct {
	Message json.RawMessage `json:"message"`
	Error   json.RawMessage `json:"error"`
	Meta    *struct {
		Error    string   `json:"error"`
		Message  string   `json:"message"`
		Messages []string `json:"messages"`
	} `json:"meta"`
	Errors json.RawMessage `json:"errors"`
}

// newAPIError reads and closes the response body.
func newAPIError(method, path string, resp *http.Response) *APIError {
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	_, _ = io.Copy(io.Discard, resp.Body)

	now := time.Now()
	e := &APIError{
		StatusCode: resp.StatusCode,
		Method:     method,
		Path:       path,
		Header:     resp.Header,
		Body:       body,
		RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), now),
		RateLimit:  parseRateLimit(resp.Header, now),
	}

	var eb errorBody
	if json.Unmarshal(body, &eb) == nil {
		top := rawString(eb.Message)
		if eb.Meta != nil {
			e.Code = eb.Meta.Error
			e.Messages = eb.Meta.Messages
			if eb.Meta.Message != "" {
				e.Message = eb.Meta.Message
			}
		}
		if e.Message == "" {
			e.Message = top
		}
		if s := rawString(eb.Error); s != "" && e.Message == "" {
			e.Message = s
		}
		if len(eb.Errors) > 0 {
			var m map[string]any
			if json.Unmarshal(eb.Errors, &m) == nil {
				e.Errors = m
			} else if s := rawString(eb.Errors); s != "" {
				e.Errors = map[string]any{"base": s}
			}
		}
	}
	if e.Message == "" && len(e.Errors) > 0 {
		e.Message = summarizeErrors(e.Errors)
	}
	if e.Message == "" {
		if s := strings.TrimSpace(string(body)); s != "" && !strings.HasPrefix(s, "{") && !strings.HasPrefix(s, "<") && len(s) <= 200 {
			e.Message = s
		}
	}
	return e
}

// summarizeErrors builds a short message from a validation errors object.
// A top level "message" key, as in the documented 400 response, wins.
func summarizeErrors(m map[string]any) string {
	if s, ok := m["message"].(string); ok && s != "" {
		return s
	}
	flat := map[string][]string{}
	flattenErrors("", m, flat)
	keys := make([]string, 0, len(flat))
	for k := range flat {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		msg := strings.Join(flat[k], ", ")
		if k != "" && k != "base" {
			msg = k + ": " + msg
		}
		parts = append(parts, msg)
	}
	const maxParts = 3
	if len(parts) > maxParts {
		return strings.Join(parts[:maxParts], "; ") + fmt.Sprintf("; and %d more", len(parts)-maxParts)
	}
	return strings.Join(parts, "; ")
}

func rawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

func parseRateLimit(h http.Header, now time.Time) *RateLimit {
	limit, okL := headerInt(h, "X-RateLimit-Limit")
	remaining, okR := headerInt(h, "X-RateLimit-Remaining")
	reset, okReset := headerInt(h, "X-RateLimit-Reset")
	if !okL && !okR && !okReset {
		return nil
	}
	rl := &RateLimit{Limit: limit, Remaining: remaining}
	switch {
	case !okReset || reset <= 0:
	case reset > 1_000_000_000:
		// A Unix timestamp.
		rl.Reset = time.Unix(int64(reset), 0).UTC()
	default:
		// Seconds until the window resets.
		rl.Reset = now.Add(time.Duration(reset) * time.Second).UTC()
	}
	return rl
}

func headerInt(h http.Header, name string) (int, bool) {
	v := strings.TrimSpace(h.Get(name))
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}
