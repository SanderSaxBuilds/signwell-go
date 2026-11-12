package signwell

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestWebhooksList(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusOK, "webhooks.json"))
	hooks, err := c.Webhooks.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if call := api.last(t); call.Method != http.MethodGet || call.Path != "/api/v1/hooks" {
		t.Errorf("request = %s %s", call.Method, call.Path)
	}
	if len(hooks) != 8 {
		t.Fatalf("%d hooks, want 8", len(hooks))
	}
	if hooks[0].ID != testWebhookID || hooks[0].APIApplicationID != "" {
		t.Errorf("first hook = %+v", hooks[0])
	}
	if hooks[3].APIApplicationID != "3fa54438-5bd9-405a-9d7f-89dd47c80dc4" {
		t.Errorf("fourth hook = %+v", hooks[3])
	}
}

func TestWebhooksListEmpty(t *testing.T) {
	c, _ := newTestClient(t, respondBytes(http.StatusOK, []byte(`[]`)))
	hooks, err := c.Webhooks.List(context.Background())
	if err != nil || len(hooks) != 0 {
		t.Fatalf("hooks = %v, err = %v", hooks, err)
	}
}

func TestWebhooksCreate(t *testing.T) {
	tests := []struct {
		name string
		req  CreateWebhookRequest
		want string
	}{
		{"callback only", CreateWebhookRequest{CallbackURL: "http://von.test/vance"}, `{"callback_url":"http://von.test/vance"}`},
		{"with application", CreateWebhookRequest{CallbackURL: "https://example.com/hook", APIApplicationID: "app-1"}, `{"callback_url":"https://example.com/hook","api_application_id":"app-1"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, api := newTestClient(t, respond(t, http.StatusCreated, "webhook_created.json"))
			hook, err := c.Webhooks.Create(context.Background(), &tt.req)
			if err != nil {
				t.Fatal(err)
			}
			call := api.last(t)
			if call.Method != http.MethodPost || call.Path != "/api/v1/hooks" || string(call.Body) != tt.want {
				t.Errorf("request = %s %s %s", call.Method, call.Path, call.Body)
			}
			if hook.ID != "3ca1189b-ae29-4a9e-878d-5c53dee86987" || hook.CallbackURL != "http://von.test/vance" {
				t.Errorf("hook = %+v", hook)
			}
		})
	}
}

func TestWebhooksCreateBadRequest(t *testing.T) {
	c, _ := newTestClient(t, respond(t, http.StatusBadRequest, "error_webhook_bad_request.json"))
	_, err := c.Webhooks.Create(context.Background(), &CreateWebhookRequest{CallbackURL: "https://example.com/hook"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !errors.Is(err, ErrBadRequest) {
		t.Fatalf("err = %v", err)
	}
	if apiErr.Code != "invalid_parameter" || apiErr.Message != "Invalid parameter: callback_url must be present" {
		t.Errorf("api error = %q %q", apiErr.Code, apiErr.Message)
	}
	if len(apiErr.Messages) != 1 {
		t.Errorf("messages = %v", apiErr.Messages)
	}
}

func TestWebhooksDelete(t *testing.T) {
	tests := []struct {
		name    string
		handler func(t *testing.T) http.HandlerFunc
		wantErr error
	}{
		{"deleted", func(t *testing.T) http.HandlerFunc { return respondBytes(http.StatusNoContent, nil) }, nil},
		{"not found", func(t *testing.T) http.HandlerFunc {
			return respond(t, http.StatusNotFound, "error_hook_not_found.json")
		}, ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, api := newTestClient(t, tt.handler(t))
			err := c.Webhooks.Delete(context.Background(), testWebhookID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if call := api.last(t); call.Method != http.MethodDelete || call.Path != "/api/v1/hooks/"+testWebhookID {
				t.Errorf("request = %s %s", call.Method, call.Path)
			}
		})
	}
}
