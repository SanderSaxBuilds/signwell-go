package signwell

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Template is a reusable document with placeholders in place of people.
type Template struct {
	ID                    string              `json:"id"`
	Name                  string              `json:"name"`
	Subject               string              `json:"subject"`
	Message               string              `json:"message"`
	Status                string              `json:"status"`
	Archived              bool                `json:"archived"`
	APIApplicationID      string              `json:"api_application_id"`
	RequesterEmailAddress string              `json:"requester_email_address"`
	CustomRequesterName   string              `json:"custom_requester_name"`
	CustomRequesterEmail  string              `json:"custom_requester_email"`
	Metadata              map[string]string   `json:"metadata"`
	CreatedAt             time.Time           `json:"created_at"`
	UpdatedAt             time.Time           `json:"updated_at"`
	Placeholders          []Placeholder       `json:"placeholders"`
	CopiedPlaceholders    []CopiedPlaceholder `json:"copied_placeholders"`
	Reminders             *bool               `json:"reminders"`
	EmbeddedEditURL       string              `json:"embedded_edit_url"`
	TemplateLink          string              `json:"template_link"`
	TemplateID            string              `json:"template_id"`
	ApplySigningOrder     bool                `json:"apply_signing_order"`
	RedirectURL           string              `json:"redirect_url"`
	DeclineRedirectURL    string              `json:"decline_redirect_url"`
	Language              string              `json:"language"`
	ExpiresIn             *int                `json:"expires_in"`
	Files                 []FileInfo          `json:"files"`
	Fields                [][]Field           `json:"fields"`
	AllowDecline          *bool               `json:"allow_decline"`
	AllowReassign         *bool               `json:"allow_reassign"`
	Labels                []Label             `json:"labels"`
	CheckboxGroups        []CheckboxGroup     `json:"checkbox_groups"`
	ConditionalRules      []ConditionalRule   `json:"conditional_rules"`
}

// Placeholder is a role on a template, such as "Client", that a real person
// fills when a document is created from it.
type Placeholder struct {
	ID                        string                `json:"id"`
	Name                      string                `json:"name"`
	Subject                   string                `json:"subject"`
	Message                   string                `json:"message"`
	PreassignedRecipientName  string                `json:"preassigned_recipient_name"`
	PreassignedRecipientEmail string                `json:"preassigned_recipient_email"`
	SigningOrder              int                   `json:"signing_order"`
	AttachmentRequests        []RecipientAttachment `json:"attachment_requests"`
	PasscodeDelivery          *PasscodeDelivery     `json:"passcode_delivery"`
}

// CopiedPlaceholder is a role that receives the final document. In a
// create request only Name and the preassigned recipient are used.
type CopiedPlaceholder struct {
	ID                        string `json:"id,omitempty"`
	PlaceholderID             string `json:"placeholder_id,omitempty"`
	Name                      string `json:"name"`
	Subject                   string `json:"subject,omitempty"`
	Message                   string `json:"message,omitempty"`
	PreassignedRecipientName  string `json:"preassigned_recipient_name,omitempty"`
	PreassignedRecipientEmail string `json:"preassigned_recipient_email,omitempty"`
}

// PlaceholderRequest defines a placeholder in Templates.Create.
type PlaceholderRequest struct {
	// ID is your own identifier, used by fields to point at this placeholder.
	ID                        string            `json:"id"`
	Name                      string            `json:"name"`
	PreassignedRecipientName  string            `json:"preassigned_recipient_name,omitempty"`
	PreassignedRecipientEmail string            `json:"preassigned_recipient_email,omitempty"`
	PasscodeDelivery          *PasscodeDelivery `json:"passcode_delivery,omitempty"`
}

// CreateTemplateRequest is the body of Templates.Create. Files and
// Placeholders are required.
type CreateTemplateRequest struct {
	Files              []File               `json:"files"`
	Name               string               `json:"name,omitempty"`
	Subject            string               `json:"subject,omitempty"`
	Message            string               `json:"message,omitempty"`
	Placeholders       []PlaceholderRequest `json:"placeholders"`
	CopiedPlaceholders []CopiedPlaceholder  `json:"copied_placeholders,omitempty"`
	Draft              bool                 `json:"draft,omitempty"`
	ExpiresIn          int                  `json:"expires_in,omitempty"`
	Reminders          *bool                `json:"reminders,omitempty"`
	ApplySigningOrder  bool                 `json:"apply_signing_order,omitempty"`
	APIApplicationID   string               `json:"api_application_id,omitempty"`
	TextTags           bool                 `json:"text_tags,omitempty"`
	RedirectURL        string               `json:"redirect_url,omitempty"`
	AllowDecline       *bool                `json:"allow_decline,omitempty"`
	AllowReassign      *bool                `json:"allow_reassign,omitempty"`
	DeclineRedirectURL string               `json:"decline_redirect_url,omitempty"`
	Language           string               `json:"language,omitempty"`
	ConditionalRules   []ConditionalRule    `json:"conditional_rules,omitempty"`
	Metadata           map[string]string    `json:"metadata,omitempty"`
	// Fields use PlaceholderID, not RecipientID.
	Fields             [][]FieldRequest    `json:"fields,omitempty"`
	AttachmentRequests []AttachmentRequest `json:"attachment_requests,omitempty"`
	Labels             []Label             `json:"labels,omitempty"`
	CheckboxGroups     []CheckboxGroup     `json:"checkbox_groups,omitempty"`
}

// TemplateList is one page of templates.
type TemplateList struct {
	Templates []Template `json:"templates"`
	Pagination
}

// TemplatesService wraps the template endpoints. Get it from Client.Templates.
type TemplatesService struct {
	client *Client
}

// List returns one page of templates. params may be nil.
//
// GET /api/v1/document_templates
func (s *TemplatesService) List(ctx context.Context, params *ListParams) (*TemplateList, error) {
	var out TemplateList
	if err := s.client.do(ctx, &request{method: http.MethodGet, path: "/document_templates", query: listQuery(params)}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListAll returns an iterator over every template that matches params.
func (s *TemplatesService) ListAll(params *ListParams) *Iterator[Template] {
	p := copyParams(params)
	return newIterator(p.Page, func(ctx context.Context, page int) ([]Template, Pagination, error) {
		p.Page = page
		list, err := s.List(ctx, &p)
		if err != nil {
			return nil, Pagination{}, err
		}
		return list.Templates, list.Pagination, nil
	})
}

// Get returns a template with its placeholders and fields.
//
// GET /api/v1/document_templates/{id}
func (s *TemplatesService) Get(ctx context.Context, id string) (*Template, error) {
	pid, err := pathID("template", id)
	if err != nil {
		return nil, err
	}
	var t Template
	if err := s.client.do(ctx, &request{method: http.MethodGet, path: "/document_templates/" + pid}, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// Create creates a template.
//
// POST /api/v1/document_templates
func (s *TemplatesService) Create(ctx context.Context, req *CreateTemplateRequest) (*Template, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: nil CreateTemplateRequest", ErrInvalidArgument)
	}
	if len(req.Files) == 0 {
		return nil, fmt.Errorf("%w: at least one file is required", ErrInvalidArgument)
	}
	if len(req.Placeholders) == 0 {
		return nil, fmt.Errorf("%w: at least one placeholder is required", ErrInvalidArgument)
	}
	var t Template
	if err := s.client.do(ctx, &request{method: http.MethodPost, path: "/document_templates", body: req}, &t); err != nil {
		return nil, err
	}
	return &t, nil
}
