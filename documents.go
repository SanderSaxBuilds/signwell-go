package signwell

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Document is a document as the API returns it. Documents created from a
// template have the same shape, with TemplateID or TemplateIDs set.
type Document struct {
	ID                           string                     `json:"id"`
	Name                         string                     `json:"name"`
	Subject                      string                     `json:"subject"`
	Message                      string                     `json:"message"`
	Status                       string                     `json:"status"`
	TestMode                     bool                       `json:"test_mode"`
	Archived                     bool                       `json:"archived"`
	CFRPart11                    bool                       `json:"cfr_part11"`
	CFRPart11SigningMode         string                     `json:"cfr_part11_signing_mode"`
	APIApplicationID             string                     `json:"api_application_id"`
	RequesterEmailAddress        string                     `json:"requester_email_address"`
	CustomRequesterName          string                     `json:"custom_requester_name"`
	CustomRequesterEmail         string                     `json:"custom_requester_email"`
	Metadata                     map[string]string          `json:"metadata"`
	CreatedAt                    time.Time                  `json:"created_at"`
	UpdatedAt                    time.Time                  `json:"updated_at"`
	Recipients                   []Recipient                `json:"recipients"`
	Reminders                    bool                       `json:"reminders"`
	Embedded                     *EmbeddedBranding          `json:"embedded"`
	EmbeddedSigning              bool                       `json:"embedded_signing"`
	EmbeddedSigningNotifications bool                       `json:"embedded_signing_notifications"`
	EmbeddedEditURL              string                     `json:"embedded_edit_url"`
	EmbeddedPreviewURL           string                     `json:"embedded_preview_url"`
	ApplySigningOrder            bool                       `json:"apply_signing_order"`
	RedirectURL                  string                     `json:"redirect_url"`
	DeclineRedirectURL           string                     `json:"decline_redirect_url"`
	Language                     string                     `json:"language"`
	ExpiresIn                    *int                       `json:"expires_in"`
	DeclineMessage               string                     `json:"decline_message"`
	ErrorMessage                 string                     `json:"error_message"`
	TemplateID                   string                     `json:"template_id"`
	TemplateIDs                  []string                   `json:"template_ids"`
	AttachmentRequests           []AttachmentRequest        `json:"attachment_requests"`
	Files                        []FileInfo                 `json:"files"`
	CopiedContacts               []CopiedContact            `json:"copied_contacts"`
	Fields                       [][]Field                  `json:"fields"`
	AllowDecline                 *bool                      `json:"allow_decline"`
	AllowReassign                *bool                      `json:"allow_reassign"`
	Labels                       []Label                    `json:"labels"`
	CheckboxGroups               []CheckboxGroup            `json:"checkbox_groups"`
	ConditionalRules             []ConditionalRule          `json:"conditional_rules"`
	ConditionalLogicDecisions    []ConditionalLogicDecision `json:"conditional_logic_decisions"`
}

// Recipient is a person who completes or signs a document.
type Recipient struct {
	ID                 string                `json:"id"`
	Name               string                `json:"name"`
	Email              string                `json:"email"`
	Role               string                `json:"role,omitempty"`
	Subject            string                `json:"subject"`
	Message            string                `json:"message"`
	Status             string                `json:"status"`
	SigningOrder       int                   `json:"signing_order"`
	SigningURL         string                `json:"signing_url"`
	EmbeddedSigningURL string                `json:"embedded_signing_url"`
	SendEmail          *bool                 `json:"send_email"`
	SendEmailDelay     *int                  `json:"send_email_delay"`
	DeliveryMethod     DeliveryMethod        `json:"delivery_method"`
	SMS                *RecipientSMS         `json:"sms,omitempty"`
	Bounced            *bool                 `json:"bounced"`
	BouncedDetails     string                `json:"bounced_details"`
	Passcode           string                `json:"passcode"`
	PasscodeDelivery   *PasscodeDelivery     `json:"passcode_delivery"`
	AttachmentRequests []RecipientAttachment `json:"attachment_requests"`
}

// RecipientSMS is the SMS delivery state of a recipient.
type RecipientSMS struct {
	PhoneNumberMasked string `json:"phone_number_masked"`
	SendSMS           *bool  `json:"send_sms"`
	SendSMSDelay      *int   `json:"send_sms_delay"`
	DeliveryStatus    string `json:"delivery_status"`
	MessageType       string `json:"message_type"`
	Failed            *bool  `json:"failed"`
	FailedDetails     string `json:"failed_details"`
}

// Field is a field placed on a document or template, as returned by the API.
// Fields are grouped per file: Document.Fields[0] holds the fields of the
// first file.
type Field struct {
	APIID                       string           `json:"api_id"`
	Name                        string           `json:"name"`
	Type                        FieldType        `json:"type"`
	Page                        int              `json:"page"`
	X                           Number           `json:"x"`
	Y                           Number           `json:"y"`
	Width                       Number           `json:"width"`
	Height                      Number           `json:"height"`
	Required                    bool             `json:"required"`
	Label                       string           `json:"label"`
	Value                       any              `json:"value"`
	RecipientID                 string           `json:"recipient_id"`
	Recipient                   *FieldRecipient  `json:"recipient,omitempty"`
	PlaceholderName             string           `json:"placeholder_name,omitempty"`
	SigningElementsGroupID      string           `json:"signing_elements_group_id,omitempty"`
	DateFormat                  DateFormat       `json:"date_format,omitempty"`
	FixedWidth                  bool             `json:"fixed_width"`
	LockSignDate                bool             `json:"lock_sign_date"`
	Formula                     string           `json:"formula,omitempty"`
	Validation                  TextValidation   `json:"validation,omitempty"`
	ValidationRegex             string           `json:"validation_regex,omitempty"`
	ValidationRegexErrorMessage string           `json:"validation_regex_error_message,omitempty"`
	Options                     []DropdownOption `json:"options,omitempty"`
	DefaultOption               string           `json:"default_option,omitempty"`
	AllowOther                  bool             `json:"allow_other"`
}

// FieldRecipient names the recipient a field belongs to.
type FieldRecipient struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role,omitempty"`
}

// RecipientRequest is a recipient in a create request. For documents made
// from a template, set PlaceholderName to the template placeholder this
// person fills.
type RecipientRequest struct {
	// ID is your own identifier, used by fields to point at this recipient.
	// Numbering from "1" works well.
	ID               string            `json:"id"`
	Name             string            `json:"name,omitempty"`
	Email            string            `json:"email,omitempty"`
	PlaceholderName  string            `json:"placeholder_name,omitempty"`
	Passcode         string            `json:"passcode,omitempty"`
	PasscodeDelivery *PasscodeDelivery `json:"passcode_delivery,omitempty"`
	Subject          string            `json:"subject,omitempty"`
	Message          string            `json:"message,omitempty"`
	SendEmail        *bool             `json:"send_email,omitempty"`
	SendEmailDelay   *int              `json:"send_email_delay,omitempty"`
	DeliveryMethod   DeliveryMethod    `json:"delivery_method,omitempty"`
	PhoneNumber      string            `json:"phone_number,omitempty"`
	SendSMS          *bool             `json:"send_sms,omitempty"`
	SendSMSDelay     *int              `json:"send_sms_delay,omitempty"`
}

// FieldRequest places a field in a create request. Use RecipientID on
// documents and PlaceholderID on templates.
type FieldRequest struct {
	X                           float64          `json:"x"`
	Y                           float64          `json:"y"`
	Page                        int              `json:"page"`
	Type                        FieldType        `json:"type"`
	RecipientID                 string           `json:"recipient_id,omitempty"`
	PlaceholderID               string           `json:"placeholder_id,omitempty"`
	Required                    *bool            `json:"required,omitempty"`
	Label                       string           `json:"label,omitempty"`
	Value                       any              `json:"value,omitempty"`
	APIID                       string           `json:"api_id,omitempty"`
	Name                        string           `json:"name,omitempty"`
	Validation                  TextValidation   `json:"validation,omitempty"`
	ValidationRegex             string           `json:"validation_regex,omitempty"`
	ValidationRegexErrorMessage string           `json:"validation_regex_error_message,omitempty"`
	FixedWidth                  bool             `json:"fixed_width,omitempty"`
	LockSignDate                bool             `json:"lock_sign_date,omitempty"`
	DateFormat                  DateFormat       `json:"date_format,omitempty"`
	Height                      float64          `json:"height,omitempty"`
	Width                       float64          `json:"width,omitempty"`
	Options                     []DropdownOption `json:"options,omitempty"`
	DefaultOption               string           `json:"default_option,omitempty"`
	AllowOther                  bool             `json:"allow_other,omitempty"`
}

// CreateDocumentRequest is the body of Documents.Create. Files and
// Recipients are required. Unless Draft or WithSignaturePage is set, at
// least one field is required too.
type CreateDocumentRequest struct {
	TestMode                     bool                `json:"test_mode,omitempty"`
	Files                        []File              `json:"files"`
	Name                         string              `json:"name,omitempty"`
	Subject                      string              `json:"subject,omitempty"`
	Message                      string              `json:"message,omitempty"`
	Recipients                   []RecipientRequest  `json:"recipients"`
	Draft                        bool                `json:"draft,omitempty"`
	SelfSign                     bool                `json:"self_sign,omitempty"`
	WithSignaturePage            bool                `json:"with_signature_page,omitempty"`
	ExpiresIn                    int                 `json:"expires_in,omitempty"`
	Reminders                    *bool               `json:"reminders,omitempty"`
	ApplySigningOrder            bool                `json:"apply_signing_order,omitempty"`
	APIApplicationID             string              `json:"api_application_id,omitempty"`
	EmbeddedSigning              bool                `json:"embedded_signing,omitempty"`
	EmbeddedSigningNotifications bool                `json:"embedded_signing_notifications,omitempty"`
	Embedded                     *EmbeddedRequest    `json:"embedded,omitempty"`
	TextTags                     bool                `json:"text_tags,omitempty"`
	CustomRequesterName          string              `json:"custom_requester_name,omitempty"`
	CustomRequesterEmail         string              `json:"custom_requester_email,omitempty"`
	RedirectURL                  string              `json:"redirect_url,omitempty"`
	AllowDecline                 *bool               `json:"allow_decline,omitempty"`
	AllowReassign                *bool               `json:"allow_reassign,omitempty"`
	DeclineRedirectURL           string              `json:"decline_redirect_url,omitempty"`
	Language                     string              `json:"language,omitempty"`
	ConditionalRules             []ConditionalRule   `json:"conditional_rules,omitempty"`
	Metadata                     map[string]string   `json:"metadata,omitempty"`
	Fields                       [][]FieldRequest    `json:"fields,omitempty"`
	AttachmentRequests           []AttachmentRequest `json:"attachment_requests,omitempty"`
	CopiedContacts               []CopiedContact     `json:"copied_contacts,omitempty"`
	Labels                       []Label             `json:"labels,omitempty"`
	CheckboxGroups               []CheckboxGroup     `json:"checkbox_groups,omitempty"`
	SMSTestEmail                 bool                `json:"sms_test_email,omitempty"`
}

// TemplateFieldValue fills a template field, found by its API ID.
type TemplateFieldValue struct {
	APIID string `json:"api_id"`
	Value any    `json:"value"`
}

// CreateDocumentFromTemplateRequest is the body of
// Documents.CreateFromTemplate. Set TemplateID, or TemplateIDs to combine
// several templates, and give every placeholder a recipient.
type CreateDocumentFromTemplateRequest struct {
	TestMode                     bool                 `json:"test_mode,omitempty"`
	TemplateID                   string               `json:"template_id,omitempty"`
	TemplateIDs                  []string             `json:"template_ids,omitempty"`
	Name                         string               `json:"name,omitempty"`
	Subject                      string               `json:"subject,omitempty"`
	Message                      string               `json:"message,omitempty"`
	Recipients                   []RecipientRequest   `json:"recipients"`
	ExcludePlaceholders          []string             `json:"exclude_placeholders,omitempty"`
	Draft                        bool                 `json:"draft,omitempty"`
	WithSignaturePage            bool                 `json:"with_signature_page,omitempty"`
	ExpiresIn                    int                  `json:"expires_in,omitempty"`
	Reminders                    *bool                `json:"reminders,omitempty"`
	ApplySigningOrder            bool                 `json:"apply_signing_order,omitempty"`
	APIApplicationID             string               `json:"api_application_id,omitempty"`
	EmbeddedSigning              bool                 `json:"embedded_signing,omitempty"`
	EmbeddedSigningNotifications bool                 `json:"embedded_signing_notifications,omitempty"`
	Embedded                     *EmbeddedRequest     `json:"embedded,omitempty"`
	TextTags                     bool                 `json:"text_tags,omitempty"`
	CustomRequesterName          string               `json:"custom_requester_name,omitempty"`
	CustomRequesterEmail         string               `json:"custom_requester_email,omitempty"`
	RedirectURL                  string               `json:"redirect_url,omitempty"`
	AllowDecline                 *bool                `json:"allow_decline,omitempty"`
	AllowReassign                *bool                `json:"allow_reassign,omitempty"`
	DeclineRedirectURL           string               `json:"decline_redirect_url,omitempty"`
	Language                     string               `json:"language,omitempty"`
	ConditionalRules             []ConditionalRule    `json:"conditional_rules,omitempty"`
	ConditionalRulesMode         string               `json:"conditional_rules_mode,omitempty"`
	Metadata                     map[string]string    `json:"metadata,omitempty"`
	TemplateFields               []TemplateFieldValue `json:"template_fields,omitempty"`
	// Files are appended after the template files.
	Files []File `json:"files,omitempty"`
	// Fields go on the appended Files only, one group per file.
	Fields             [][]FieldRequest    `json:"fields,omitempty"`
	AttachmentRequests []AttachmentRequest `json:"attachment_requests,omitempty"`
	CopiedContacts     []CopiedContact     `json:"copied_contacts,omitempty"`
	Labels             []Label             `json:"labels,omitempty"`
	CheckboxGroups     []CheckboxGroup     `json:"checkbox_groups,omitempty"`
	SMSTestEmail       bool                `json:"sms_test_email,omitempty"`
}

// SendDocumentRequest is the body of Documents.Send. Every field is
// optional and changes the draft before it goes out.
type SendDocumentRequest struct {
	TestMode                     bool              `json:"test_mode,omitempty"`
	Name                         string            `json:"name,omitempty"`
	Subject                      string            `json:"subject,omitempty"`
	Message                      string            `json:"message,omitempty"`
	ExpiresIn                    int               `json:"expires_in,omitempty"`
	Reminders                    *bool             `json:"reminders,omitempty"`
	ApplySigningOrder            bool              `json:"apply_signing_order,omitempty"`
	APIApplicationID             string            `json:"api_application_id,omitempty"`
	EmbeddedSigning              bool              `json:"embedded_signing,omitempty"`
	EmbeddedSigningNotifications bool              `json:"embedded_signing_notifications,omitempty"`
	Embedded                     *EmbeddedRequest  `json:"embedded,omitempty"`
	CustomRequesterName          string            `json:"custom_requester_name,omitempty"`
	CustomRequesterEmail         string            `json:"custom_requester_email,omitempty"`
	RedirectURL                  string            `json:"redirect_url,omitempty"`
	AllowDecline                 *bool             `json:"allow_decline,omitempty"`
	AllowReassign                *bool             `json:"allow_reassign,omitempty"`
	DeclineRedirectURL           string            `json:"decline_redirect_url,omitempty"`
	ConditionalRules             []ConditionalRule `json:"conditional_rules,omitempty"`
	Metadata                     map[string]string `json:"metadata,omitempty"`
	Labels                       []Label           `json:"labels,omitempty"`
	CheckboxGroups               []CheckboxGroup   `json:"checkbox_groups,omitempty"`
	SMSTestEmail                 bool              `json:"sms_test_email,omitempty"`
}

// ReminderRecipient picks one recipient to remind.
type ReminderRecipient struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

// RemindRequest is the body of Documents.Remind. With no recipients, every
// recipient who has not signed yet is reminded.
type RemindRequest struct {
	Recipients []ReminderRecipient `json:"recipients,omitempty"`
}

// DocumentList is one page of documents.
type DocumentList struct {
	Documents []Document `json:"documents"`
	Pagination
}

// CompletedPDFParams tunes the completed document download.
type CompletedPDFParams struct {
	// AuditPage includes the audit page. The API default is true.
	AuditPage *bool
	// FileFormat is FormatPDF (the default) or FormatZIP.
	FileFormat FileFormat
}

func (p *CompletedPDFParams) query(urlOnly bool) url.Values {
	q := url.Values{}
	if urlOnly {
		q.Set("url_only", "true")
	}
	if p == nil {
		return q
	}
	if p.AuditPage != nil {
		q.Set("audit_page", strconv.FormatBool(*p.AuditPage))
	}
	if p.FileFormat != "" {
		q.Set("file_format", string(p.FileFormat))
	}
	return q
}

// Download is a streamed file. The caller must close Body.
type Download struct {
	Body          io.ReadCloser
	ContentType   string
	ContentLength int64
	// Filename comes from the Content-Disposition header when present.
	Filename string
}

// DocumentsService wraps the document endpoints. Get it from Client.Documents.
type DocumentsService struct {
	client *Client
}

// Create creates a document and, unless req.Draft is set, sends it for
// signing.
//
// POST /api/v1/documents
func (s *DocumentsService) Create(ctx context.Context, req *CreateDocumentRequest) (*Document, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: nil CreateDocumentRequest", ErrInvalidArgument)
	}
	if len(req.Files) == 0 {
		return nil, fmt.Errorf("%w: at least one file is required", ErrInvalidArgument)
	}
	if len(req.Recipients) == 0 {
		return nil, fmt.Errorf("%w: at least one recipient is required", ErrInvalidArgument)
	}
	var doc Document
	err := s.client.do(ctx, &request{method: http.MethodPost, path: "/documents", body: req, testModeHeader: true}, &doc)
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// CreateFromTemplate creates a document from one or more templates and,
// unless req.Draft is set, sends it for signing.
//
// POST /api/v1/document_templates/documents
func (s *DocumentsService) CreateFromTemplate(ctx context.Context, req *CreateDocumentFromTemplateRequest) (*Document, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: nil CreateDocumentFromTemplateRequest", ErrInvalidArgument)
	}
	if req.TemplateID == "" && len(req.TemplateIDs) == 0 {
		return nil, fmt.Errorf("%w: TemplateID or TemplateIDs is required", ErrInvalidArgument)
	}
	if len(req.Recipients) == 0 {
		return nil, fmt.Errorf("%w: at least one recipient is required", ErrInvalidArgument)
	}
	var doc Document
	err := s.client.do(ctx, &request{method: http.MethodPost, path: "/document_templates/documents", body: req, testModeHeader: true}, &doc)
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// Send updates a draft document and sends it to its recipients. req may be
// nil to send the draft as it is.
//
// POST /api/v1/documents/{id}/send
func (s *DocumentsService) Send(ctx context.Context, id string, req *SendDocumentRequest) (*Document, error) {
	pid, err := pathID("document", id)
	if err != nil {
		return nil, err
	}
	if req == nil {
		req = &SendDocumentRequest{}
	}
	var doc Document
	if err := s.client.do(ctx, &request{method: http.MethodPost, path: "/documents/" + pid + "/send", body: req, testModeHeader: true}, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// Get returns a document with its recipients, fields and status.
//
// GET /api/v1/documents/{id}
func (s *DocumentsService) Get(ctx context.Context, id string) (*Document, error) {
	pid, err := pathID("document", id)
	if err != nil {
		return nil, err
	}
	var doc Document
	if err := s.client.do(ctx, &request{method: http.MethodGet, path: "/documents/" + pid}, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// List returns one page of documents. params may be nil.
//
// GET /api/v1/documents
func (s *DocumentsService) List(ctx context.Context, params *ListParams) (*DocumentList, error) {
	var out DocumentList
	if err := s.client.do(ctx, &request{method: http.MethodGet, path: "/documents", query: listQuery(params)}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListAll returns an iterator over every document that matches params,
// fetching pages as it goes. params.Page sets the first page.
func (s *DocumentsService) ListAll(params *ListParams) *Iterator[Document] {
	p := copyParams(params)
	return newIterator(p.Page, func(ctx context.Context, page int) ([]Document, Pagination, error) {
		p.Page = page
		list, err := s.List(ctx, &p)
		if err != nil {
			return nil, Pagination{}, err
		}
		return list.Documents, list.Pagination, nil
	})
}

// Remind emails or texts the recipients who have not signed yet. req may
// be nil to remind all of them.
//
// POST /api/v1/documents/{id}/remind
func (s *DocumentsService) Remind(ctx context.Context, id string, req *RemindRequest) (*Document, error) {
	pid, err := pathID("document", id)
	if err != nil {
		return nil, err
	}
	if req == nil {
		req = &RemindRequest{}
	}
	var doc Document
	if err := s.client.do(ctx, &request{method: http.MethodPost, path: "/documents/" + pid + "/remind", body: req}, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// Delete deletes a document. A document that is still out for signing is
// canceled.
//
// DELETE /api/v1/documents/{id}
func (s *DocumentsService) Delete(ctx context.Context, id string) error {
	pid, err := pathID("document", id)
	if err != nil {
		return err
	}
	return s.client.do(ctx, &request{method: http.MethodDelete, path: "/documents/" + pid}, nil)
}

// DownloadCompletedPDF streams the completed document as a PDF, or as a ZIP
// when params.FileFormat is FormatZIP. params may be nil. The caller must
// close the returned Body.
//
// GET /api/v1/documents/{id}/completed_pdf
func (s *DocumentsService) DownloadCompletedPDF(ctx context.Context, id string, params *CompletedPDFParams) (*Download, error) {
	pid, err := pathID("document", id)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.send(ctx, &request{
		method: http.MethodGet,
		path:   "/documents/" + pid + "/completed_pdf",
		query:  params.query(false),
		accept: "application/octet-stream",
	})
	if err != nil {
		return nil, err
	}
	d := &Download{
		Body:          resp.Body,
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, p, err := mime.ParseMediaType(cd); err == nil {
			d.Filename = p["filename"]
		}
	}
	return d, nil
}

// CompletedPDFURL returns a link to the completed document instead of its
// bytes. params may be nil.
//
// GET /api/v1/documents/{id}/completed_pdf?url_only=true
func (s *DocumentsService) CompletedPDFURL(ctx context.Context, id string, params *CompletedPDFParams) (string, error) {
	pid, err := pathID("document", id)
	if err != nil {
		return "", err
	}
	var out struct {
		FileURL string `json:"file_url"`
	}
	if err := s.client.do(ctx, &request{
		method: http.MethodGet,
		path:   "/documents/" + pid + "/completed_pdf",
		query:  params.query(true),
	}, &out); err != nil {
		return "", err
	}
	if out.FileURL == "" {
		return "", &DecodeError{StatusCode: http.StatusOK, Err: fmt.Errorf("response has no file_url")}
	}
	return out.FileURL, nil
}

func listQuery(p *ListParams) url.Values {
	q := url.Values{}
	if p == nil {
		return q
	}
	if p.Page > 0 {
		q.Set("page", strconv.Itoa(p.Page))
	}
	if p.Limit > 0 {
		limit := p.Limit
		if limit > MaxPageSize {
			limit = MaxPageSize
		}
		q.Set("limit", strconv.Itoa(limit))
	}
	if p.Query != "" {
		q.Set("query", p.Query)
	}
	return q
}

func copyParams(p *ListParams) ListParams {
	if p == nil {
		return ListParams{}
	}
	return *p
}
