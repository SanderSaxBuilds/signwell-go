package signwell

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func loadPayload(t *testing.T, name string) *WebhookPayload {
	t.Helper()
	var p WebhookPayload
	if err := json.Unmarshal(fixture(t, name), &p); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return &p
}

func TestVerifyEventFixtures(t *testing.T) {
	for _, name := range []string{
		"event_document_viewed.json",
		"event_document_completed.json",
		"event_document_sms_failed.json",
		"event_template_available.json",
	} {
		t.Run(name, func(t *testing.T) {
			p := loadPayload(t, name)
			if err := VerifyEvent(p.Event, testWebhookID); err != nil {
				t.Fatalf("VerifyEvent: %v", err)
			}
		})
	}
}

func TestSignatureMatchesFixture(t *testing.T) {
	// The fixture hash was produced with Python's hmac module, the way the
	// SignWell docs show it, so this pins the Go side to the same bytes.
	p := loadPayload(t, "event_document_completed.json")
	if got := Signature(testWebhookID, p.Event.Type, p.Event.Time); got != p.Event.Hash {
		t.Errorf("Signature = %s, want %s", got, p.Event.Hash)
	}
}

func TestVerifyEventFailures(t *testing.T) {
	good := loadPayload(t, "event_document_completed.json").Event
	tests := []struct {
		name    string
		edit    func(e *WebhookEvent)
		key     string
		wantErr error
	}{
		{"wrong webhook id", func(e *WebhookEvent) {}, "35826895-d91f-49d9-a397-39955b7a5ac9", ErrInvalidSignature},
		{"empty webhook id", func(e *WebhookEvent) {}, "", ErrMissingWebhookID},
		{"type changed", func(e *WebhookEvent) { e.Type = EventDocumentDeclined }, testWebhookID, ErrInvalidSignature},
		{"time changed", func(e *WebhookEvent) { e.Time = "1689332301" }, testWebhookID, ErrInvalidSignature},
		{"time as float text", func(e *WebhookEvent) { e.Time = "1689332300.0" }, testWebhookID, ErrInvalidSignature},
		{"hash not hex", func(e *WebhookEvent) { e.Hash = "102bb628c79c4cca904482add21ad8d9a5dff05f137788156457hubs8afbd83d" }, testWebhookID, ErrInvalidSignature},
		{"hash truncated", func(e *WebhookEvent) { e.Hash = e.Hash[:32] }, testWebhookID, ErrInvalidSignature},
		{"missing hash", func(e *WebhookEvent) { e.Hash = "" }, testWebhookID, ErrInvalidEvent},
		{"missing type", func(e *WebhookEvent) { e.Type = "" }, testWebhookID, ErrInvalidEvent},
		{"missing time", func(e *WebhookEvent) { e.Time = "" }, testWebhookID, ErrInvalidEvent},
		{"uppercase hash is accepted", func(e *WebhookEvent) { e.Hash = strings.ToUpper(e.Hash) }, testWebhookID, nil},
		{"surrounding spaces in hash are accepted", func(e *WebhookEvent) { e.Hash = " " + e.Hash + "\n" }, testWebhookID, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := good
			tt.edit(&e)
			err := VerifyEvent(e, tt.key)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestVerifyEventMissingFieldsMessage(t *testing.T) {
	err := VerifyEvent(WebhookEvent{}, testWebhookID)
	if err == nil || !strings.Contains(err.Error(), "type, time, hash") {
		t.Fatalf("err = %v", err)
	}
}

func TestVerifyEventTolerance(t *testing.T) {
	ev := loadPayload(t, "event_document_completed.json").Event // time 1689332300
	at := time.Unix(1689332300, 0)
	tests := []struct {
		name      string
		now       time.Time
		tolerance time.Duration
		wantErr   error
	}{
		{"no tolerance ignores age", at.Add(1000 * time.Hour), 0, nil},
		{"inside tolerance", at.Add(4 * time.Minute), 5 * time.Minute, nil},
		{"exactly at tolerance", at.Add(5 * time.Minute), 5 * time.Minute, nil},
		{"too old", at.Add(6 * time.Minute), 5 * time.Minute, ErrEventExpired},
		{"from the future", at.Add(-10 * time.Minute), 5 * time.Minute, ErrEventExpired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyEvent(ev, testWebhookID, WithTolerance(tt.tolerance), WithClock(func() time.Time { return tt.now }))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestVerifyEventToleranceNeedsNumericTime(t *testing.T) {
	e := WebhookEvent{Type: EventDocumentSent, Time: "yesterday"}
	e.Hash = Signature(testWebhookID, e.Type, e.Time)
	if err := VerifyEvent(e, testWebhookID); err != nil {
		t.Fatalf("without tolerance: %v", err)
	}
	if err := VerifyEvent(e, testWebhookID, WithTolerance(time.Minute)); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("with tolerance: %v", err)
	}
}

func TestVerifyEventStringTime(t *testing.T) {
	// A time sent as a JSON string is signed as its text, like a number.
	body := []byte(`{"event":{"type":"document_sent","time":"1689332249","hash":"` +
		Signature(testWebhookID, "document_sent", "1689332249") + `"},"data":{"object":{"id":"x"}}}`)
	p, err := ParseWebhook(body, testWebhookID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Event.Time != "1689332249" {
		t.Errorf("time = %q", p.Event.Time)
	}
}

func TestEventTimeJSON(t *testing.T) {
	tests := []struct {
		in       string
		want     EventTime
		marshal  string
		wantUnix int64
		unixErr  bool
	}{
		{`1689332249`, "1689332249", `1689332249`, 1689332249, false},
		{`"1689332249"`, "1689332249", `1689332249`, 1689332249, false},
		{`1689332249.5`, "1689332249.5", `1689332249.5`, 1689332249, false},
		{`"soon"`, "soon", `"soon"`, 0, true},
		{`null`, "", `""`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			var et EventTime
			if err := json.Unmarshal([]byte(tt.in), &et); err != nil {
				t.Fatal(err)
			}
			if et != tt.want {
				t.Errorf("EventTime = %q, want %q", et, tt.want)
			}
			out, err := json.Marshal(et)
			if err != nil || string(out) != tt.marshal {
				t.Errorf("Marshal = %s %v, want %s", out, err, tt.marshal)
			}
			ts, err := et.Time()
			if (err != nil) != tt.unixErr {
				t.Fatalf("Time() err = %v", err)
			}
			if err == nil && ts.Unix() != tt.wantUnix {
				t.Errorf("Time() = %v", ts)
			}
		})
	}
}

func TestEventTimeFraction(t *testing.T) {
	ts, err := EventTime("1689332249.25").Time()
	if err != nil {
		t.Fatal(err)
	}
	if ts.Nanosecond() != 250000000 {
		t.Errorf("nanoseconds = %d", ts.Nanosecond())
	}
}

func TestParseWebhook(t *testing.T) {
	tests := []struct {
		name      string
		body      []byte
		key       string
		wantErr   error
		wantType  string
		template  bool
		wantState string
	}{
		{"viewed", fixture(t, "event_document_viewed.json"), testWebhookID, nil, EventDocumentViewed, false, "Sent"},
		{"completed", fixture(t, "event_document_completed.json"), testWebhookID, nil, EventDocumentCompleted, false, "Completed"},
		{"sms failed", fixture(t, "event_document_sms_failed.json"), testWebhookID, nil, EventDocumentSMSFailed, false, "Sent"},
		{"template available", fixture(t, "event_template_available.json"), testWebhookID, nil, EventTemplateAvailable, true, "Available"},
		{"not json", []byte("event=document_sent"), testWebhookID, ErrInvalidEvent, "", false, ""},
		{"wrong key", fixture(t, "event_document_viewed.json"), "other", ErrInvalidSignature, "", false, ""},
		{"no event block", []byte(`{"data":{}}`), testWebhookID, ErrInvalidEvent, "", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParseWebhook(tt.body, tt.key)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if p != nil {
					t.Error("payload returned with an error")
				}
				return
			}
			if p.Event.Type != tt.wantType || p.IsTemplateEvent() != tt.template {
				t.Errorf("type = %s, template = %v", p.Event.Type, p.IsTemplateEvent())
			}
			if p.Data.AccountID != "7c98753-0acc-46ab-84b0-e96d085983e0" {
				t.Errorf("account = %q", p.Data.AccountID)
			}
			var status string
			if tt.template {
				tpl, err := p.Template()
				if err != nil {
					t.Fatal(err)
				}
				status = tpl.Status
				if len(tpl.CheckboxGroups) != 1 || tpl.Fields[0][1].ValidationRegex != "^[A-Z]{3}-[0-9]{4}$" {
					t.Errorf("template = %+v", tpl.CheckboxGroups)
				}
			} else {
				doc, err := p.Document()
				if err != nil {
					t.Fatal(err)
				}
				status = doc.Status
				if doc.ID != "a692e1ce-9929-4541-adc7-61932bab39d3" {
					t.Errorf("document id = %s", doc.ID)
				}
			}
			if status != tt.wantState {
				t.Errorf("status = %q, want %q", status, tt.wantState)
			}
		})
	}
}

func TestWebhookRelatedPeople(t *testing.T) {
	viewed := loadPayload(t, "event_document_viewed.json")
	if s := viewed.Event.RelatedSigner; s == nil || s.Email != "person@email.com" || s.Name != "Christian Jordan" {
		t.Errorf("related signer = %+v", s)
	}
	sms := loadPayload(t, "event_document_sms_failed.json")
	r := sms.Event.RelatedRecipient
	if r == nil || r.PhoneNumberMasked != "+1 ******1234" || r.DeliveryMethod != DeliveryEmailAndSMS {
		t.Fatalf("related recipient = %+v", r)
	}
	doc, err := sms.Document()
	if err != nil {
		t.Fatal(err)
	}
	s := doc.Recipients[0].SMS
	if s == nil || s.DeliveryStatus != "failed" || s.Failed == nil || !*s.Failed {
		t.Errorf("sms = %+v", s)
	}
}

func TestWebhookEmbeddedFalse(t *testing.T) {
	// The document webhook example sends "embedded": false, not an object.
	doc, err := loadPayload(t, "event_document_viewed.json").Document()
	if err != nil {
		t.Fatal(err)
	}
	if doc.Embedded == nil {
		t.Fatal("Embedded should decode to an empty value")
	}
	if doc.Embedded.Branding.AccentColor != "" {
		t.Errorf("embedded = %+v", doc.Embedded)
	}
}

func TestPayloadWithoutObject(t *testing.T) {
	p := &WebhookPayload{}
	if _, err := p.Document(); !errors.Is(err, ErrInvalidEvent) {
		t.Errorf("Document err = %v", err)
	}
	if _, err := p.Template(); !errors.Is(err, ErrInvalidEvent) {
		t.Errorf("Template err = %v", err)
	}
	p.Data.Object = json.RawMessage(`[1,2]`)
	if _, err := p.Document(); err == nil {
		t.Error("Document should fail on an array")
	}
	if _, err := p.Template(); err == nil {
		t.Error("Template should fail on an array")
	}
}

func TestWebhookHandler(t *testing.T) {
	good := fixture(t, "event_document_completed.json")
	tests := []struct {
		name     string
		method   string
		body     string
		fnErr    error
		maxBody  int64
		want     int
		wantCall bool
	}{
		{"valid event", http.MethodPost, string(good), nil, 0, http.StatusOK, true},
		{"handler fails", http.MethodPost, string(good), errors.New("db down"), 0, http.StatusInternalServerError, true},
		{"bad signature", http.MethodPost, strings.Replace(string(good), "document_completed", "document_declined", 1), nil, 0, http.StatusBadRequest, false},
		{"not json", http.MethodPost, "hello", nil, 0, http.StatusBadRequest, false},
		{"GET", http.MethodGet, "", nil, 0, http.StatusMethodNotAllowed, false},
		{"body too large", http.MethodPost, string(good), nil, 64, http.StatusRequestEntityTooLarge, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			h := WebhookHandler(testWebhookID, func(ctx context.Context, p *WebhookPayload) error {
				called = true
				if p.Event.Type != EventDocumentCompleted {
					t.Errorf("type = %s", p.Event.Type)
				}
				return tt.fnErr
			}, WithMaxBodySize(tt.maxBody))
			req := httptest.NewRequest(tt.method, "/hooks/signwell", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
			if called != tt.wantCall {
				t.Errorf("handler called = %v, want %v", called, tt.wantCall)
			}
			if tt.method == http.MethodGet && rec.Header().Get("Allow") != http.MethodPost {
				t.Errorf("Allow = %q", rec.Header().Get("Allow"))
			}
		})
	}
}

func TestWebhookHandlerTolerance(t *testing.T) {
	h := WebhookHandler(testWebhookID, func(context.Context, *WebhookPayload) error { return nil },
		WithTolerance(time.Minute), WithClock(time.Now), WithClock(nil))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(fixture(t, "event_document_completed.json"))))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, an event from 2023 must fail a one minute tolerance", rec.Code)
	}
}

func TestVerifyOptionNil(t *testing.T) {
	ev := loadPayload(t, "event_document_completed.json").Event
	if err := VerifyEvent(ev, testWebhookID, nil); err != nil {
		t.Fatal(err)
	}
}
