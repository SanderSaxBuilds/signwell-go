package signwell

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
)

func TestTemplatesGet(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusOK, "template.json"))
	tpl, err := c.Templates.Get(context.Background(), "2851644b-8c61-4901-ab97-43fee9106bec")
	if err != nil {
		t.Fatal(err)
	}
	if call := api.last(t); call.Method != http.MethodGet || call.Path != "/api/v1/document_templates/2851644b-8c61-4901-ab97-43fee9106bec" {
		t.Errorf("request = %s %s", call.Method, call.Path)
	}
	if tpl.Status != "Available" || tpl.Name != "satisfaction_twilight/voluptatem.doc" {
		t.Errorf("template = %q %q", tpl.Status, tpl.Name)
	}
	if len(tpl.Placeholders) != 2 || tpl.Placeholders[0].Name == "" {
		t.Errorf("placeholders = %+v", tpl.Placeholders)
	}
	if len(tpl.Files) != 1 || tpl.Files[0].PagesNumber != 2 {
		t.Errorf("files = %+v", tpl.Files)
	}
	if len(tpl.Fields) != 1 || len(tpl.Fields[0]) != 2 {
		t.Errorf("fields = %v", tpl.Fields)
	}
	if tpl.TemplateLink == "" || tpl.CreatedAt.IsZero() {
		t.Errorf("link %q created %s", tpl.TemplateLink, tpl.CreatedAt)
	}
}

func TestTemplatesGetNotFound(t *testing.T) {
	c, _ := newTestClient(t, respond(t, http.StatusNotFound, "error_not_found.json"))
	if _, err := c.Templates.Get(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestTemplatesList(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusOK, "templates_page1.json"))
	list, err := c.Templates.List(context.Background(), &ListParams{Limit: 1, Query: "name:NDA"})
	if err != nil {
		t.Fatal(err)
	}
	call := api.last(t)
	q, _ := url.ParseQuery(call.Query)
	if call.Path != "/api/v1/document_templates" || q.Get("limit") != "1" || q.Get("query") != "name:NDA" {
		t.Errorf("request = %s ?%s", call.Path, call.Query)
	}
	if len(list.Templates) != 1 || list.Templates[0].ID != "2851644b-8c61-4901-ab97-43fee9106bec" || !list.HasNext() {
		t.Errorf("list = %+v", list.Pagination)
	}
}

func TestTemplatesListAll(t *testing.T) {
	c, api := newTestClient(t, byPage(map[string]http.HandlerFunc{
		"1": respond(t, http.StatusOK, "templates_page1.json"),
		"2": respond(t, http.StatusOK, "templates_page2.json"),
	}))
	all, err := c.Templates.ListAll(nil).Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[1].ID != "d8012784-8934-4de3-ac16-ae131a42eab9" {
		t.Errorf("templates = %d", len(all))
	}
	if n := len(api.Calls()); n != 2 {
		t.Errorf("%d requests", n)
	}
}

func TestTemplatesListAllError(t *testing.T) {
	c, _ := newTestClient(t, byPage(map[string]http.HandlerFunc{
		"1": respond(t, http.StatusOK, "templates_page1.json"),
		"2": respond(t, http.StatusUnauthorized, "error_unauthorized.json"),
	}))
	all, err := c.Templates.ListAll(nil).Collect(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v", err)
	}
	if len(all) != 1 {
		t.Errorf("got %d templates before the error, want 1", len(all))
	}
}

func TestTemplatesCreate(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusCreated, "template_created.json"))
	tpl, err := c.Templates.Create(context.Background(), &CreateTemplateRequest{
		Name:  "Agreement",
		Files: []File{FileFromURL("filename1.pdf", "https://example.com/filename1.pdf")},
		Placeholders: []PlaceholderRequest{
			{ID: "1", Name: "Client"},
			{ID: "2", Name: "Company", PreassignedRecipientEmail: "legal@example.com"},
		},
		CopiedPlaceholders: []CopiedPlaceholder{{Name: "Accounting"}},
		Fields: [][]FieldRequest{{
			{X: 10, Y: 10, Page: 1, Type: FieldSignature, PlaceholderID: "1"},
			{X: 10, Y: 40, Page: 1, Type: FieldText, PlaceholderID: "2", Label: "Title", Required: Bool(false)},
		}},
		AttachmentRequests: []AttachmentRequest{{Name: "ID card", PlaceholderID: "1", Required: true}},
		CheckboxGroups:     []CheckboxGroup{{GroupName: "Terms", PlaceholderID: "1", CheckboxIDs: []string{"a", "b"}, Validation: CheckboxMinimum, MinValue: Int(1)}},
		Reminders:          Bool(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	call := api.last(t)
	if call.Method != http.MethodPost || call.Path != "/api/v1/document_templates" {
		t.Errorf("request = %s %s", call.Method, call.Path)
	}
	body := call.json(t)
	ph := body["placeholders"].([]any)
	if len(ph) != 2 || ph[1].(map[string]any)["preassigned_recipient_email"] != "legal@example.com" {
		t.Errorf("placeholders = %v", ph)
	}
	field := body["fields"].([]any)[0].([]any)[1].(map[string]any)
	if field["placeholder_id"] != "2" || field["required"] != false {
		t.Errorf("field = %v", field)
	}
	if _, ok := field["recipient_id"]; ok {
		t.Error("template field sent recipient_id")
	}
	group := body["checkbox_groups"].([]any)[0].(map[string]any)
	if group["validation"] != "minimum" || group["min_value"] != float64(1) {
		t.Errorf("group = %v", group)
	}
	if body["reminders"] != false {
		t.Errorf("reminders = %v", body["reminders"])
	}
	if tpl.ID != "d8012784-8934-4de3-ac16-ae131a42eab9" || len(tpl.Placeholders) != 2 {
		t.Errorf("template = %s %d", tpl.ID, len(tpl.Placeholders))
	}
}

func TestTemplatesCreateValidationError(t *testing.T) {
	c, _ := newTestClient(t, respond(t, http.StatusUnprocessableEntity, "error_template_validation.json"))
	_, err := c.Templates.Create(context.Background(), &CreateTemplateRequest{
		Files:        []File{FileFromURL("a.pdf", "https://example.com/a.pdf")},
		Placeholders: []PlaceholderRequest{{ID: "1", Name: "Client"}},
	})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !errors.Is(err, ErrUnprocessableEntity) {
		t.Fatalf("err = %v", err)
	}
	fe := apiErr.FieldErrors()
	if got := fe["placeholders.placeholder_1.name"]; len(got) != 1 || got[0] != "is too long (maximum is 255 characters)" {
		t.Errorf("placeholder name errors = %v", got)
	}
	if got := fe["placeholders.duplicated_ids"]; len(got) != 1 {
		t.Errorf("duplicated ids = %v", got)
	}
}
