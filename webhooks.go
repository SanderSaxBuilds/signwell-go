package signwell

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Webhook is a registered callback URL. Its ID is also the key that signs
// the events sent to it, see VerifyEvent.
type Webhook struct {
	ID               string `json:"id"`
	CallbackURL      string `json:"callback_url"`
	APIApplicationID string `json:"api_application_id,omitempty"`
}

// CreateWebhookRequest is the body of Webhooks.Create.
type CreateWebhookRequest struct {
	// CallbackURL receives a POST for every document and template event.
	CallbackURL string `json:"callback_url"`
	// APIApplicationID limits the webhook to one API application.
	APIApplicationID string `json:"api_application_id,omitempty"`
}

// WebhooksService wraps the webhook endpoints. Get it from Client.Webhooks.
type WebhooksService struct {
	client *Client
}

// List returns every webhook on the account. The endpoint is not paged.
//
// GET /api/v1/hooks
func (s *WebhooksService) List(ctx context.Context) ([]Webhook, error) {
	var out []Webhook
	if err := s.client.do(ctx, &request{method: http.MethodGet, path: "/hooks"}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Create registers a callback URL.
//
// POST /api/v1/hooks
func (s *WebhooksService) Create(ctx context.Context, req *CreateWebhookRequest) (*Webhook, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: nil CreateWebhookRequest", ErrInvalidArgument)
	}
	u, err := url.Parse(strings.TrimSpace(req.CallbackURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%w: callback URL %q must be an absolute http or https URL", ErrInvalidArgument, req.CallbackURL)
	}
	var w Webhook
	if err := s.client.do(ctx, &request{method: http.MethodPost, path: "/hooks", body: req}, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

// Delete removes a webhook.
//
// DELETE /api/v1/hooks/{id}
func (s *WebhooksService) Delete(ctx context.Context, id string) error {
	pid, err := pathID("webhook", id)
	if err != nil {
		return err
	}
	return s.client.do(ctx, &request{method: http.MethodDelete, path: "/hooks/" + pid}, nil)
}
