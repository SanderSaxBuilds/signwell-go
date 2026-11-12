package signwell

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Event types SignWell posts to webhooks.
const (
	EventDocumentCreated           = "document_created"
	EventDocumentSent              = "document_sent"
	EventDocumentViewed            = "document_viewed"
	EventDocumentInProgress        = "document_in_progress"
	EventDocumentRecipientsUpdated = "document_recipients_updated"
	EventDocumentSigned            = "document_signed"
	EventDocumentCompleted         = "document_completed"
	EventDocumentExpired           = "document_expired"
	EventDocumentCanceled          = "document_canceled"
	EventDocumentDeclined          = "document_declined"
	EventDocumentBounced           = "document_bounced"
	EventDocumentError             = "document_error"
	EventDocumentSMSFailed         = "document_sms_failed"
	EventDocumentSMSOptedOut       = "document_sms_opted_out"
	EventTemplateCreated           = "template_created"
	EventTemplateError             = "template_error"
	EventTemplateAvailable         = "template_available"
	EventTemplateUnavailable       = "template_unavailable"
)

// Webhook verification errors.
var (
	// ErrMissingWebhookID is returned when the signing key is empty.
	ErrMissingWebhookID = errors.New("signwell: webhook id is empty")
	// ErrInvalidEvent is returned when the event lacks type, time or hash, or
	// when the payload is not valid JSON.
	ErrInvalidEvent = errors.New("signwell: invalid webhook event")
	// ErrInvalidSignature is returned when the hash does not match.
	ErrInvalidSignature = errors.New("signwell: webhook signature does not match")
	// ErrEventExpired is returned when the event time is outside the tolerance
	// set with WithTolerance.
	ErrEventExpired = errors.New("signwell: webhook event time is outside the tolerance")
)

// EventTime is the event timestamp exactly as SignWell sent it. The
// signature covers this text, so it is kept as is instead of being parsed.
type EventTime string

// UnmarshalJSON keeps a JSON number or string as its text.
func (t *EventTime) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	switch {
	case bytes.Equal(data, []byte("null")):
		*t = ""
	case len(data) > 0 && data[0] == '"':
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*t = EventTime(s)
	default:
		*t = EventTime(data)
	}
	return nil
}

// MarshalJSON writes a numeric time as a number and anything else as a string.
func (t EventTime) MarshalJSON() ([]byte, error) {
	if _, err := strconv.ParseFloat(string(t), 64); err == nil {
		return []byte(t), nil
	}
	return json.Marshal(string(t))
}

// Time parses the timestamp as Unix seconds.
func (t EventTime) Time() (time.Time, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(string(t)), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return time.Time{}, fmt.Errorf("%w: time %q is not a Unix timestamp", ErrInvalidEvent, string(t))
	}
	sec, frac := math.Modf(f)
	return time.Unix(int64(sec), int64(frac*1e9)).UTC(), nil
}

// WebhookEvent is the event block of a webhook payload.
type WebhookEvent struct {
	Type string    `json:"type"`
	Time EventTime `json:"time"`
	Hash string    `json:"hash"`
	// RelatedSigner is set on document_viewed, document_declined and
	// document_signed.
	RelatedSigner *EventSigner `json:"related_signer,omitempty"`
	// RelatedRecipient is set on SMS delivery events.
	RelatedRecipient *EventRecipient `json:"related_recipient,omitempty"`
}

// EventSigner is the signer an event is about.
type EventSigner struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// EventRecipient is the recipient an SMS event is about.
type EventRecipient struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	Email             string         `json:"email"`
	PhoneNumberMasked string         `json:"phone_number_masked"`
	DeliveryMethod    DeliveryMethod `json:"delivery_method"`
}

// WebhookPayload is the JSON body SignWell posts to a webhook.
type WebhookPayload struct {
	Event WebhookEvent `json:"event"`
	Data  WebhookData  `json:"data"`
}

// WebhookData carries the document or template the event is about.
type WebhookData struct {
	// Object is the document or template, in the same shape Get returns.
	// Decode it with WebhookPayload.Document or WebhookPayload.Template.
	Object      json.RawMessage `json:"object"`
	AccountID   string          `json:"account_id"`
	WorkspaceID string          `json:"workspace_id,omitempty"`
}

// IsTemplateEvent reports whether the event is about a template.
func (p *WebhookPayload) IsTemplateEvent() bool {
	return strings.HasPrefix(p.Event.Type, "template_")
}

// Document decodes data.object as a Document.
func (p *WebhookPayload) Document() (*Document, error) {
	if len(p.Data.Object) == 0 {
		return nil, fmt.Errorf("%w: payload has no data.object", ErrInvalidEvent)
	}
	var d Document
	if err := json.Unmarshal(p.Data.Object, &d); err != nil {
		return nil, fmt.Errorf("signwell: decode event document: %w", err)
	}
	return &d, nil
}

// Template decodes data.object as a Template.
func (p *WebhookPayload) Template() (*Template, error) {
	if len(p.Data.Object) == 0 {
		return nil, fmt.Errorf("%w: payload has no data.object", ErrInvalidEvent)
	}
	var t Template
	if err := json.Unmarshal(p.Data.Object, &t); err != nil {
		return nil, fmt.Errorf("signwell: decode event template: %w", err)
	}
	return &t, nil
}

// VerifyOption tunes webhook verification.
type VerifyOption func(*verifyConfig)

type verifyConfig struct {
	tolerance time.Duration
	now       func() time.Time
	maxBody   int64
}

// WithTolerance rejects events whose time is further than d from now, which
// limits replays of captured events. Off by default.
func WithTolerance(d time.Duration) VerifyOption {
	return func(c *verifyConfig) { c.tolerance = d }
}

// WithClock replaces time.Now for the tolerance check.
func WithClock(now func() time.Time) VerifyOption {
	return func(c *verifyConfig) {
		if now != nil {
			c.now = now
		}
	}
}

// WithMaxBodySize caps the request body WebhookHandler reads. The default
// is 10 MiB.
func WithMaxBodySize(n int64) VerifyOption {
	return func(c *verifyConfig) {
		if n > 0 {
			c.maxBody = n
		}
	}
}

func newVerifyConfig(opts []VerifyOption) verifyConfig {
	c := verifyConfig{now: time.Now, maxBody: 10 << 20}
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	return c
}

// Signature returns the hex HMAC-SHA256 SignWell puts in event.hash: the key
// is the webhook ID and the message is "<type>@<time>".
func Signature(webhookID, eventType string, eventTime EventTime) string {
	mac := hmac.New(sha256.New, []byte(webhookID))
	mac.Write([]byte(eventType + "@" + string(eventTime)))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyEvent checks that event was signed with webhookID, the ID of the
// webhook that received it. Pass payload.Event, not the whole payload.
func VerifyEvent(event WebhookEvent, webhookID string, opts ...VerifyOption) error {
	if webhookID == "" {
		return ErrMissingWebhookID
	}
	var missing []string
	if event.Type == "" {
		missing = append(missing, "type")
	}
	if event.Time == "" {
		missing = append(missing, "time")
	}
	if event.Hash == "" {
		missing = append(missing, "hash")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: missing %s", ErrInvalidEvent, strings.Join(missing, ", "))
	}

	cfg := newVerifyConfig(opts)
	if cfg.tolerance > 0 {
		at, err := event.Time.Time()
		if err != nil {
			return err
		}
		skew := cfg.now().Sub(at)
		if skew < 0 {
			skew = -skew
		}
		if skew > cfg.tolerance {
			return fmt.Errorf("%w: event is %s away from now", ErrEventExpired, skew.Round(time.Second))
		}
	}

	got, err := hex.DecodeString(strings.TrimSpace(event.Hash))
	if err != nil {
		return ErrInvalidSignature
	}
	want, _ := hex.DecodeString(Signature(webhookID, event.Type, event.Time))
	if !hmac.Equal(got, want) {
		return ErrInvalidSignature
	}
	return nil
}

// ParseWebhook decodes a webhook body and verifies its signature. It
// returns the payload only when the signature is valid.
func ParseWebhook(body []byte, webhookID string, opts ...VerifyOption) (*WebhookPayload, error) {
	var p WebhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidEvent, err)
	}
	if err := VerifyEvent(p.Event, webhookID, opts...); err != nil {
		return nil, err
	}
	return &p, nil
}

// WebhookHandler returns an http.Handler that verifies each POST and passes
// the payload to fn. It answers 200 when fn returns nil, 500 when fn fails
// so SignWell can deliver the event again, 400 for a body that is not a
// valid signed event, and 405 for other methods.
func WebhookHandler(webhookID string, fn func(ctx context.Context, p *WebhookPayload) error, opts ...VerifyOption) http.Handler {
	cfg := newVerifyConfig(opts)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, cfg.maxBody))
		if err != nil {
			http.Error(w, "cannot read body", http.StatusRequestEntityTooLarge)
			return
		}
		p, err := ParseWebhook(body, webhookID, opts...)
		if err != nil {
			http.Error(w, "invalid webhook event", http.StatusBadRequest)
			return
		}
		if err := fn(r.Context(), p); err != nil {
			http.Error(w, "webhook handler failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}
