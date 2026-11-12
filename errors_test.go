package signwell

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func makeResponse(status int, body []byte, headers ...string) *http.Response {
	h := http.Header{}
	for i := 0; i+1 < len(headers); i += 2 {
		h.Set(headers[i], headers[i+1])
	}
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(bytes.NewReader(body))}
}

func TestAPIErrorIs(t *testing.T) {
	sentinels := []error{ErrBadRequest, ErrUnauthorized, ErrForbidden, ErrNotFound, ErrConflict, ErrUnprocessableEntity, ErrRateLimited, ErrServer}
	tests := []struct {
		status int
		want   error
	}{
		{400, ErrBadRequest},
		{401, ErrUnauthorized},
		{403, ErrForbidden},
		{404, ErrNotFound},
		{409, ErrConflict},
		{422, ErrUnprocessableEntity},
		{429, ErrRateLimited},
		{500, ErrServer},
		{503, ErrServer},
		{418, nil},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.status), func(t *testing.T) {
			err := error(&APIError{StatusCode: tt.status})
			for _, s := range sentinels {
				if got, want := errors.Is(err, s), s == tt.want; got != want {
					t.Errorf("errors.Is(%d, %v) = %v", tt.status, s, got)
				}
			}
			if errors.Is(err, ErrInvalidArgument) {
				t.Error("APIError must not match ErrInvalidArgument")
			}
		})
	}
}

func TestAPIErrorWrapped(t *testing.T) {
	err := fmt.Errorf("sync contract: %w", &APIError{StatusCode: 404})
	if !errors.Is(err, ErrNotFound) {
		t.Error("wrapped APIError should still match ErrNotFound")
	}
}

func TestAPIErrorTemporary(t *testing.T) {
	for status, want := range map[int]bool{400: false, 404: false, 422: false, 429: true, 500: true, 503: true} {
		if got := (&APIError{StatusCode: status}).Temporary(); got != want {
			t.Errorf("Temporary(%d) = %v", status, got)
		}
	}
}

func TestNewAPIErrorFromFixtures(t *testing.T) {
	tests := []struct {
		fixture string
		status  int
		message string
		code    string
		fields  int
	}{
		{"error_not_found.json", 404, "Couldn't find the document requested", "record_not_found", 0},
		{"error_hook_not_found.json", 404, "Couldn't find the hook requested", "record_not_found", 0},
		{"error_unauthorized.json", 401, "Not valid authorization token", "api_key_unauthorized_error", 0},
		{"error_webhook_bad_request.json", 400, "Invalid parameter: callback_url must be present", "invalid_parameter", 0},
		{"error_bad_request.json", 400, "The request contains invalid key values.", "", 2},
		{"error_remind.json", 422, "document: The document is in a status that cannot be reminded", "", 1},
		{"error_conflict.json", 409, "Document in 'completed' status cannot have recipients managed. Only sent, viewed, pending, or bounced documents are eligible.", "", 1},
		{"error_rate_limit.json", 429, "Rate limit exceeded: 100 requests per 60 seconds. Try again in 12 seconds.", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			e := newAPIError(http.MethodGet, "/documents/x", makeResponse(tt.status, fixture(t, tt.fixture)))
			if e.Message != tt.message {
				t.Errorf("Message = %q, want %q", e.Message, tt.message)
			}
			if e.Code != tt.code {
				t.Errorf("Code = %q, want %q", e.Code, tt.code)
			}
			if got := len(e.FieldErrors()); got != tt.fields {
				t.Errorf("%d field errors, want %d: %v", got, tt.fields, e.FieldErrors())
			}
			if len(e.Body) == 0 {
				t.Error("Body not kept")
			}
		})
	}
}

func TestNewAPIErrorUnusualBodies(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		message string
	}{
		{"empty body", "", ""},
		{"plain text", "Service Unavailable", "Service Unavailable"},
		{"html page", "<html><body>Bad Gateway</body></html>", ""},
		{"json without known keys", `{"foo": "bar"}`, ""},
		{"errors as a string", `{"errors": "Document is locked"}`, "Document is locked"},
		{"top level message only", `{"message": "Not found"}`, "Not found"},
		{"meta wins over message", `{"message": "Not found", "meta": {"error": "x", "message": "Couldn't find it"}}`, "Couldn't find it"},
		{"many field errors are shortened", `{"errors": {"a": ["1"], "b": ["2"], "c": ["3"], "d": ["4"], "e": ["5"]}}`, "a: 1; b: 2; c: 3; and 2 more"},
		{"long plain text dropped", strings.Repeat("x", 300), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newAPIError(http.MethodGet, "/x", makeResponse(500, []byte(tt.body)))
			if e.Message != tt.message {
				t.Errorf("Message = %q, want %q", e.Message, tt.message)
			}
		})
	}
}

func TestFieldErrorsFlatten(t *testing.T) {
	e := newAPIError(http.MethodPost, "/documents", makeResponse(422, fixture(t, "error_validation.json")))
	fe := e.FieldErrors()
	want := map[string][]string{
		"custom_requester_email":       {"is invalid"},
		"recipients.recipient_1.email": {"format is invalid."},
		"recipients.recipient_2.email": {"format is invalid."},
		"recipients.duplicated_ids":    {"These ids are duplicated: recipient_id_1."},
	}
	for k, v := range want {
		if !reflect.DeepEqual(fe[k], v) {
			t.Errorf("FieldErrors[%q] = %v, want %v", k, fe[k], v)
		}
	}
	if got := len(fe["metadata"]); got != 3 {
		t.Errorf("metadata has %d messages, want 3", got)
	}
	if !strings.Contains(e.Message, "; and ") {
		t.Errorf("Message = %q, want a shortened summary", e.Message)
	}
}

func TestFieldErrorsEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		errors map[string]any
		want   map[string][]string
	}{
		{"nil", nil, nil},
		{"empty", map[string]any{}, nil},
		{"null value", map[string]any{"name": nil}, nil},
		{"number", map[string]any{"count": float64(3)}, map[string][]string{"count": {"3"}}},
		{"list of objects", map[string]any{"files": []any{map[string]any{"name": []any{"is required"}}}}, map[string][]string{"files.name": {"is required"}}},
		{"nested list", map[string]any{"fields": []any{[]any{"bad x"}}}, map[string][]string{"fields": {"bad x"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (&APIError{Errors: tt.errors}).FieldErrors()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FieldErrors = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAPIErrorString(t *testing.T) {
	tests := []struct {
		err  *APIError
		want string
	}{
		{&APIError{StatusCode: 404, Method: "GET", Path: "/documents/x", Message: "Couldn't find the document requested", Code: "record_not_found"},
			"signwell: GET /documents/x: 404 Not Found: Couldn't find the document requested (record_not_found)"},
		{&APIError{StatusCode: 503, Method: "GET", Path: "/hooks"}, "signwell: GET /hooks: 503 Service Unavailable"},
		{&APIError{StatusCode: 422, Method: "POST", Path: "/documents", Message: "expires_in: must be less than or equal to 365"},
			"signwell: POST /documents: 422 Unprocessable Entity: expires_in: must be less than or equal to 365"},
	}
	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("Error() = %q\nwant      %q", got, tt.want)
		}
	}
}

func TestRateLimitHeaders(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name    string
		headers []string
		want    *RateLimit
		reset   time.Duration // expected reset distance from now, when not a timestamp
	}{
		{"absent", nil, nil, 0},
		{"limit and remaining", []string{"X-RateLimit-Limit", "100", "X-RateLimit-Remaining", "0"}, &RateLimit{Limit: 100, Remaining: 0}, 0},
		{"unix reset", []string{"X-RateLimit-Limit", "20", "X-RateLimit-Reset", "1799999999"}, &RateLimit{Limit: 20, Reset: time.Unix(1799999999, 0).UTC()}, 0},
		{"seconds reset", []string{"X-RateLimit-Reset", "30"}, &RateLimit{}, 30 * time.Second},
		{"garbage values", []string{"X-RateLimit-Limit", "lots", "X-RateLimit-Remaining", ""}, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newAPIError(http.MethodGet, "/x", makeResponse(429, fixture(t, "error_rate_limit.json"), tt.headers...))
			got := e.RateLimit
			if tt.want == nil {
				if got != nil {
					t.Fatalf("RateLimit = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("RateLimit is nil")
			}
			if got.Limit != tt.want.Limit || got.Remaining != tt.want.Remaining {
				t.Errorf("RateLimit = %+v, want %+v", got, tt.want)
			}
			if tt.reset > 0 {
				if d := got.Reset.Sub(now); d < tt.reset-2*time.Second || d > tt.reset+2*time.Second {
					t.Errorf("Reset is %s from now, want about %s", d, tt.reset)
				}
			} else if !got.Reset.Equal(tt.want.Reset) {
				t.Errorf("Reset = %s, want %s", got.Reset, tt.want.Reset)
			}
		})
	}
}

func TestDecodeErrorString(t *testing.T) {
	inner := errors.New("unexpected end of JSON input")
	err := error(&DecodeError{StatusCode: 200, Body: []byte("{"), Err: inner})
	if got, want := err.Error(), "signwell: decode 200 response: unexpected end of JSON input"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, inner) {
		t.Error("DecodeError should unwrap to its cause")
	}
}

func TestRetryAfterOnAPIError(t *testing.T) {
	e := newAPIError(http.MethodGet, "/x", makeResponse(429, nil, "Retry-After", "9"))
	if e.RetryAfter != 9*time.Second {
		t.Errorf("RetryAfter = %s", e.RetryAfter)
	}
}
