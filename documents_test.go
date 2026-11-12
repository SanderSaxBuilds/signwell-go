package signwell

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"
)

func TestDocumentsCreate(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusCreated, "document_created.json"))
	doc, err := c.Documents.Create(context.Background(), &CreateDocumentRequest{
		TestMode: true,
		Name:     "Agreement",
		Files: []File{
			FileFromURL("filename1.pdf", "https://example.com/filename1.pdf"),
			FileFromBytes("filename2.pdf", []byte("%PDF-1.4")),
		},
		Recipients: []RecipientRequest{
			{ID: "1", Name: "Jane Doe", Email: "jane@example.com"},
			{ID: "2", Name: "Sam Lee", Email: "sam@example.com", DeliveryMethod: DeliveryEmail},
		},
		Fields: [][]FieldRequest{
			{{X: 20, Y: 60, Page: 1, Type: FieldSignature, RecipientID: "1"}},
			{{X: 20, Y: 90, Page: 1, Type: FieldDate, RecipientID: "2", DateFormat: DateDDMMYYYY}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	call := api.last(t)
	if call.Method != http.MethodPost || call.Path != "/api/v1/documents" {
		t.Errorf("request = %s %s", call.Method, call.Path)
	}
	body := call.json(t)
	if body["test_mode"] != true || body["name"] != "Agreement" {
		t.Errorf("body = %v", body)
	}
	files := body["files"].([]any)
	if f := files[1].(map[string]any); f["file_base64"] != base64.StdEncoding.EncodeToString([]byte("%PDF-1.4")) {
		t.Errorf("base64 file = %v", f)
	}
	fields := body["fields"].([]any)
	if len(fields) != 2 || len(fields[0].([]any)) != 1 {
		t.Errorf("fields = %v", fields)
	}
	for _, absent := range []string{"reminders", "allow_decline", "draft", "embedded"} {
		if _, ok := body[absent]; ok {
			t.Errorf("unset %q was sent", absent)
		}
	}

	if doc.ID != "1b9a02e9-2b2d-42f1-81e6-a28e7c239f43" || doc.Name != "Agreement" || doc.Status != "Created" {
		t.Errorf("doc = %s %q %s", doc.ID, doc.Name, doc.Status)
	}
	if len(doc.Recipients) != 2 || len(doc.Files) != 2 {
		t.Errorf("recipients %d files %d", len(doc.Recipients), len(doc.Files))
	}
	if got := []int{len(doc.Fields[0]), len(doc.Fields[1])}; !reflect.DeepEqual(got, []int{9, 18}) {
		t.Errorf("field groups = %v", got)
	}
	if want := time.Date(2026, 9, 29, 15, 21, 41, 0, time.UTC); !doc.CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %s", doc.CreatedAt)
	}
}

func TestCreateDocumentRequestEncoding(t *testing.T) {
	base := func() CreateDocumentRequest {
		return CreateDocumentRequest{
			Files:      []File{FileFromURL("a.pdf", "https://example.com/a.pdf")},
			Recipients: []RecipientRequest{{ID: "1", Email: "a@example.com"}},
		}
	}
	tests := []struct {
		name  string
		edit  func(r *CreateDocumentRequest)
		key   string
		want  any
		unset bool
	}{
		{"reminders unset", func(r *CreateDocumentRequest) {}, "reminders", nil, true},
		{"reminders false", func(r *CreateDocumentRequest) { r.Reminders = Bool(false) }, "reminders", false, false},
		{"reminders true", func(r *CreateDocumentRequest) { r.Reminders = Bool(true) }, "reminders", true, false},
		{"allow decline false", func(r *CreateDocumentRequest) { r.AllowDecline = Bool(false) }, "allow_decline", false, false},
		{"allow reassign false", func(r *CreateDocumentRequest) { r.AllowReassign = Bool(false) }, "allow_reassign", false, false},
		{"draft", func(r *CreateDocumentRequest) { r.Draft = true }, "draft", true, false},
		{"test mode off is omitted", func(r *CreateDocumentRequest) {}, "test_mode", nil, true},
		{"expires in", func(r *CreateDocumentRequest) { r.ExpiresIn = 30 }, "expires_in", float64(30), false},
		{"metadata", func(r *CreateDocumentRequest) { r.Metadata = map[string]string{"crm_id": "42"} }, "metadata", map[string]any{"crm_id": "42"}, false},
		{"labels", func(r *CreateDocumentRequest) { r.Labels = []Label{{Name: "sales"}} }, "labels", []any{map[string]any{"name": "sales"}}, false},
		{"copied contacts", func(r *CreateDocumentRequest) { r.CopiedContacts = []CopiedContact{{Email: "cc@example.com"}} }, "copied_contacts", []any{map[string]any{"email": "cc@example.com"}}, false},
		{"embedded branding", func(r *CreateDocumentRequest) {
			r.Embedded = &EmbeddedRequest{Branding: &BrandingRequest{AccentColor: "#112233", ButtonBorderRadius: Int(4)}}
		}, "embedded", map[string]any{"branding": map[string]any{"accent_color": "#112233", "button_border_radius": float64(4)}}, false},
		{"with signature page", func(r *CreateDocumentRequest) { r.WithSignaturePage = true }, "with_signature_page", true, false},
		{"text tags", func(r *CreateDocumentRequest) { r.TextTags = true }, "text_tags", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := base()
			tt.edit(&r)
			raw, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatal(err)
			}
			got, ok := m[tt.key]
			if tt.unset {
				if ok {
					t.Fatalf("%s = %v, want absent", tt.key, got)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%s = %#v, want %#v", tt.key, got, tt.want)
			}
		})
	}
}

func TestDocumentsCreateFromTemplate(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusCreated, "document_from_template.json"))
	doc, err := c.Documents.CreateFromTemplate(context.Background(), &CreateDocumentFromTemplateRequest{
		TemplateIDs: []string{"8ed515fe-9981-49fb-846e-d25b0f207e73", "dc6eadb2-c0ad-4524-b9ce-e435a3eb27b0"},
		Recipients: []RecipientRequest{
			{ID: "1", PlaceholderName: "Client", Name: "Jane Doe", Email: "jane@example.com"},
		},
		TemplateFields: []TemplateFieldValue{{APIID: "company", Value: "Acme"}, {APIID: "agree", Value: true}},
		Draft:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	call := api.last(t)
	if call.Path != "/api/v1/document_templates/documents" {
		t.Errorf("path = %s", call.Path)
	}
	body := call.json(t)
	if body["draft"] != true {
		t.Errorf("draft = %v", body["draft"])
	}
	rec := body["recipients"].([]any)[0].(map[string]any)
	if rec["placeholder_name"] != "Client" {
		t.Errorf("recipient = %v", rec)
	}
	tf := body["template_fields"].([]any)
	if tf[1].(map[string]any)["value"] != true {
		t.Errorf("template_fields = %v", tf)
	}
	if doc.ID != "95a49223-3955-470a-8da0-9d8f059c8c22" || len(doc.TemplateIDs) != 2 || len(doc.Recipients) != 3 {
		t.Errorf("doc = %s %v %d", doc.ID, doc.TemplateIDs, len(doc.Recipients))
	}
}

func TestDocumentsSend(t *testing.T) {
	tests := []struct {
		name string
		req  *SendDocumentRequest
		want string
	}{
		{"nil request sends empty object", nil, `{}`},
		{"subject and reminders", &SendDocumentRequest{Subject: "Please sign", Reminders: Bool(false)}, `{"subject":"Please sign","reminders":false}`},
		{"test mode", &SendDocumentRequest{TestMode: true}, `{"test_mode":true}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, api := newTestClient(t, respond(t, http.StatusCreated, "document_sent.json"))
			doc, err := c.Documents.Send(context.Background(), "d7955008-c997-466c-957d-117433a84c36", tt.req)
			if err != nil {
				t.Fatal(err)
			}
			call := api.last(t)
			if call.Method != http.MethodPost || call.Path != "/api/v1/documents/d7955008-c997-466c-957d-117433a84c36/send" {
				t.Errorf("request = %s %s", call.Method, call.Path)
			}
			if string(call.Body) != tt.want {
				t.Errorf("body = %s, want %s", call.Body, tt.want)
			}
			if doc.ID != "d7955008-c997-466c-957d-117433a84c36" {
				t.Errorf("id = %s", doc.ID)
			}
		})
	}
}

func TestDocumentsGet(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusOK, "document.json"))
	doc, err := c.Documents.Get(context.Background(), "1234c241-ea39-40a8-94a9-6bc69e83e9d6")
	if err != nil {
		t.Fatal(err)
	}
	if call := api.last(t); call.Method != http.MethodGet || call.Path != "/api/v1/documents/1234c241-ea39-40a8-94a9-6bc69e83e9d6" {
		t.Errorf("request = %s %s", call.Method, call.Path)
	}

	if doc.Status != "Created" || doc.TestMode || doc.Language != "en" {
		t.Errorf("status %q test %v lang %q", doc.Status, doc.TestMode, doc.Language)
	}
	if doc.ExpiresIn == nil || *doc.ExpiresIn != 10 {
		t.Errorf("ExpiresIn = %v", doc.ExpiresIn)
	}
	if doc.AllowDecline == nil || *doc.AllowDecline || doc.AllowReassign == nil || !*doc.AllowReassign {
		t.Errorf("allow decline %v reassign %v", doc.AllowDecline, doc.AllowReassign)
	}
	if doc.Embedded != nil {
		t.Errorf("Embedded = %+v, want nil", doc.Embedded)
	}
	if doc.Metadata["neque"] != "error" {
		t.Errorf("metadata = %v", doc.Metadata)
	}
	if len(doc.Labels) != 2 || doc.Labels[0].Name != "Fr. Adriene Bergstrom" {
		t.Errorf("labels = %v", doc.Labels)
	}
	r := doc.Recipients[0]
	if r.Email != "ailene@schmidt.example" || r.SigningOrder != 1 || r.DeliveryMethod != DeliveryEmail || r.Status != "created" {
		t.Errorf("recipient = %+v", r)
	}
	if r.PasscodeDelivery == nil || r.PasscodeDelivery.Enabled {
		t.Errorf("passcode delivery = %+v", r.PasscodeDelivery)
	}
	if len(r.AttachmentRequests) != 1 || !r.AttachmentRequests[0].Required {
		t.Errorf("attachments = %+v", r.AttachmentRequests)
	}
	f := doc.Fields[0][2]
	if f.APIID != "Signature_1" || f.Type != FieldSignature || f.Width != 112 || f.Height != 32 || f.X != 1.1 {
		t.Errorf("field = %+v", f)
	}
	if f.Value != nil {
		t.Errorf("value = %v", f.Value)
	}
	if len(doc.CheckboxGroups) != 1 || doc.CheckboxGroups[0].CheckboxIDs[0] != "CheckBox_2" {
		t.Errorf("checkbox groups = %+v", doc.CheckboxGroups)
	}
	rule := doc.ConditionalRules[0]
	if rule.ActionType != "require" || rule.Conditions[0].ConditionType != "any_of" {
		t.Errorf("rule = %+v", rule)
	}
	if len(doc.CopiedContacts) != 1 || doc.CopiedContacts[0].Email != "armandina@kshlerin.test" {
		t.Errorf("copied = %+v", doc.CopiedContacts)
	}
}

func TestDocumentsListQuery(t *testing.T) {
	tests := []struct {
		name   string
		params *ListParams
		want   url.Values
	}{
		{"nil params", nil, url.Values{}},
		{"zero params", &ListParams{}, url.Values{}},
		{"page", &ListParams{Page: 3}, url.Values{"page": {"3"}}},
		{"limit", &ListParams{Limit: 25}, url.Values{"limit": {"25"}}},
		{"limit above max is lowered", &ListParams{Limit: 500}, url.Values{"limit": {"50"}}},
		{"negative values ignored", &ListParams{Page: -1, Limit: -5}, url.Values{}},
		{"query", &ListParams{Query: "status:completed AND name:NDA"}, url.Values{"query": {"status:completed AND name:NDA"}}},
		{"all", &ListParams{Page: 2, Limit: 50, Query: "name:NDA"}, url.Values{"page": {"2"}, "limit": {"50"}, "query": {"name:NDA"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, api := newTestClient(t, respond(t, http.StatusOK, "documents_empty.json"))
			list, err := c.Documents.List(context.Background(), tt.params)
			if err != nil {
				t.Fatal(err)
			}
			call := api.last(t)
			if call.Path != "/api/v1/documents" {
				t.Errorf("path = %s", call.Path)
			}
			got, _ := url.ParseQuery(call.Query)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("query = %v, want %v", got, tt.want)
			}
			if len(list.Documents) != 0 || list.HasNext() {
				t.Errorf("list = %+v", list)
			}
		})
	}
}

func TestDocumentsListPage(t *testing.T) {
	c, _ := newTestClient(t, respond(t, http.StatusOK, "documents_page1.json"))
	list, err := c.Documents.List(context.Background(), &ListParams{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Documents) != 2 || list.CurrentPage != 1 || list.TotalCount != 3 || list.TotalPages != 2 {
		t.Errorf("list = %d docs, %+v", len(list.Documents), list.Pagination)
	}
	if !list.HasNext() || *list.NextPage != 2 || list.PreviousPage != nil {
		t.Errorf("pagination = %+v", list.Pagination)
	}
}

func TestDocumentsListAll(t *testing.T) {
	c, api := newTestClient(t, byPage(map[string]http.HandlerFunc{
		"1": respond(t, http.StatusOK, "documents_page1.json"),
		"2": respond(t, http.StatusOK, "documents_page2.json"),
	}))
	it := c.Documents.ListAll(&ListParams{Limit: 2, Query: "status:sent"})
	var ids []string
	for it.Next(context.Background()) {
		ids = append(ids, it.Current().ID)
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"1234c241-ea39-40a8-94a9-6bc69e83e9d6",
		"1b9a02e9-2b2d-42f1-81e6-a28e7c239f43",
		"d7955008-c997-466c-957d-117433a84c36",
	}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %v", ids)
	}
	if it.Pages() != 2 || it.TotalCount() != 3 {
		t.Errorf("pages %d total %d", it.Pages(), it.TotalCount())
	}
	calls := api.Calls()
	if len(calls) != 2 {
		t.Fatalf("%d requests, want 2", len(calls))
	}
	for i, call := range calls {
		q, _ := url.ParseQuery(call.Query)
		if q.Get("limit") != "2" || q.Get("query") != "status:sent" || q.Get("page") != []string{"1", "2"}[i] {
			t.Errorf("request %d query = %v", i, q)
		}
	}
}

func TestDocumentsListAllDoesNotMutateParams(t *testing.T) {
	c, _ := newTestClient(t, byPage(map[string]http.HandlerFunc{
		"1": respond(t, http.StatusOK, "documents_page1.json"),
		"2": respond(t, http.StatusOK, "documents_page2.json"),
	}))
	params := &ListParams{Limit: 2}
	if _, err := c.Documents.ListAll(params).Collect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if params.Page != 0 {
		t.Errorf("caller params changed: %+v", params)
	}
}

func TestDocumentsRemind(t *testing.T) {
	tests := []struct {
		name string
		req  *RemindRequest
		want string
	}{
		{"everyone", nil, `{}`},
		{"one recipient", &RemindRequest{Recipients: []ReminderRecipient{{Email: "jane@example.com"}}}, `{"recipients":[{"email":"jane@example.com"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, api := newTestClient(t, respond(t, http.StatusCreated, "document_reminded.json"))
			doc, err := c.Documents.Remind(context.Background(), "b58e25b8-a20f-4b06-b6b9-18b7993b33ea", tt.req)
			if err != nil {
				t.Fatal(err)
			}
			call := api.last(t)
			if call.Path != "/api/v1/documents/b58e25b8-a20f-4b06-b6b9-18b7993b33ea/remind" || string(call.Body) != tt.want {
				t.Errorf("request = %s %s", call.Path, call.Body)
			}
			if doc.Status != "Sent" {
				t.Errorf("status = %s", doc.Status)
			}
		})
	}
}

func TestDocumentsDelete(t *testing.T) {
	c, api := newTestClient(t, respondBytes(http.StatusNoContent, nil))
	if err := c.Documents.Delete(context.Background(), "doc-1"); err != nil {
		t.Fatal(err)
	}
	if call := api.last(t); call.Method != http.MethodDelete || call.Path != "/api/v1/documents/doc-1" {
		t.Errorf("request = %s %s", call.Method, call.Path)
	}
}

func TestDocumentsDeleteNotFound(t *testing.T) {
	c, _ := newTestClient(t, respond(t, http.StatusNotFound, "error_not_found.json"))
	err := c.Documents.Delete(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "record_not_found" || apiErr.Method != http.MethodDelete {
		t.Errorf("api error = %+v", apiErr)
	}
}

func TestDownloadCompletedPDF(t *testing.T) {
	pdf := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\n")
	tests := []struct {
		name      string
		params    *CompletedPDFParams
		wantQuery url.Values
		ctype     string
	}{
		{"defaults", nil, url.Values{}, "application/pdf"},
		{"without audit page", &CompletedPDFParams{AuditPage: Bool(false)}, url.Values{"audit_page": {"false"}}, "application/pdf"},
		{"zip", &CompletedPDFParams{FileFormat: FormatZIP, AuditPage: Bool(true)}, url.Values{"file_format": {"zip"}, "audit_page": {"true"}}, "application/zip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, api := newTestClient(t, respondBytes(http.StatusOK, pdf,
				"Content-Type", tt.ctype,
				"Content-Disposition", `attachment; filename="Agreement.pdf"`))
			dl, err := c.Documents.DownloadCompletedPDF(context.Background(), "doc-1", tt.params)
			if err != nil {
				t.Fatal(err)
			}
			defer dl.Body.Close()
			got, err := io.ReadAll(dl.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(pdf) {
				t.Errorf("body = %q", got)
			}
			if dl.ContentType != tt.ctype || dl.Filename != "Agreement.pdf" || dl.ContentLength != int64(len(pdf)) {
				t.Errorf("download = %+v", dl)
			}
			call := api.last(t)
			if call.Path != "/api/v1/documents/doc-1/completed_pdf" || call.Header.Get("Accept") != "application/octet-stream" {
				t.Errorf("request = %s Accept %s", call.Path, call.Header.Get("Accept"))
			}
			q, _ := url.ParseQuery(call.Query)
			if !reflect.DeepEqual(q, tt.wantQuery) {
				t.Errorf("query = %v, want %v", q, tt.wantQuery)
			}
		})
	}
}

func TestDownloadCompletedPDFNoDisposition(t *testing.T) {
	c, _ := newTestClient(t, respondBytes(http.StatusOK, []byte("%PDF"), "Content-Type", "application/pdf"))
	dl, err := c.Documents.DownloadCompletedPDF(context.Background(), "doc-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Body.Close()
	if dl.Filename != "" {
		t.Errorf("filename = %q", dl.Filename)
	}
}

func TestDownloadCompletedPDFNotFound(t *testing.T) {
	c, _ := newTestClient(t, respond(t, http.StatusNotFound, "error_not_found.json"))
	_, err := c.Documents.DownloadCompletedPDF(context.Background(), "missing", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestCompletedPDFURL(t *testing.T) {
	c, api := newTestClient(t, respond(t, http.StatusOK, "completed_pdf_url.json"))
	u, err := c.Documents.CompletedPDFURL(context.Background(), "doc-1", &CompletedPDFParams{AuditPage: Bool(true)})
	if err != nil {
		t.Fatal(err)
	}
	if u != "https://www.signwell.com/signed/D0dDgQgOMGuR4lXBabLKTbYjER9.pdf?audit_page=true" {
		t.Errorf("url = %s", u)
	}
	call := api.last(t)
	q, _ := url.ParseQuery(call.Query)
	if q.Get("url_only") != "true" || q.Get("audit_page") != "true" || call.Header.Get("Accept") != "application/json" {
		t.Errorf("query = %v accept = %s", q, call.Header.Get("Accept"))
	}
}

func TestCompletedPDFURLMissing(t *testing.T) {
	c, _ := newTestClient(t, respondBytes(http.StatusOK, []byte(`{}`)))
	_, err := c.Documents.CompletedPDFURL(context.Background(), "doc-1", nil)
	var de *DecodeError
	if !errors.As(err, &de) {
		t.Fatalf("err = %v, want *DecodeError", err)
	}
}

func TestDocumentErrorResponses(t *testing.T) {
	ctx := context.Background()
	create := func(c *Client) error {
		_, err := c.Documents.Create(ctx, &CreateDocumentRequest{
			Files:      []File{FileFromURL("a.pdf", "https://example.com/a.pdf")},
			Recipients: []RecipientRequest{{ID: "1", Email: "a@example.com"}},
		})
		return err
	}
	tests := []struct {
		name     string
		status   int
		fixture  string
		call     func(c *Client) error
		sentinel error
		message  string
		field    string
	}{
		{"create 400", 400, "error_bad_request.json", create, ErrBadRequest, "The request contains invalid key values.", "invalid_keys"},
		{"create 422", 422, "error_validation.json", create, ErrUnprocessableEntity, "", "recipients.recipient_1.email"},
		{"get 404", 404, "error_not_found.json", func(c *Client) error { _, err := c.Documents.Get(ctx, "x"); return err }, ErrNotFound, "Couldn't find the document requested", ""},
		{"get 401", 401, "error_unauthorized.json", func(c *Client) error { _, err := c.Documents.Get(ctx, "x"); return err }, ErrUnauthorized, "Not valid authorization token", ""},
		{"remind 422", 422, "error_remind.json", func(c *Client) error { _, err := c.Documents.Remind(ctx, "x", nil); return err }, ErrUnprocessableEntity, "document: The document is in a status that cannot be reminded", "document"},
		{"send 409", 409, "error_conflict.json", func(c *Client) error { _, err := c.Documents.Send(ctx, "x", nil); return err }, ErrConflict, "Document in 'completed' status cannot have recipients managed. Only sent, viewed, pending, or bounced documents are eligible.", "base"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, api := newTestClient(t, respond(t, tt.status, tt.fixture))
			err := tt.call(c)
			if !errors.Is(err, tt.sentinel) {
				t.Fatalf("err = %v, want %v", err, tt.sentinel)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("err is %T", err)
			}
			if apiErr.StatusCode != tt.status {
				t.Errorf("status = %d", apiErr.StatusCode)
			}
			if tt.message != "" && apiErr.Message != tt.message {
				t.Errorf("message = %q, want %q", apiErr.Message, tt.message)
			}
			if tt.field != "" {
				if _, ok := apiErr.FieldErrors()[tt.field]; !ok {
					t.Errorf("FieldErrors has no %q: %v", tt.field, apiErr.FieldErrors())
				}
			}
			if n := len(api.Calls()); n != 1 {
				t.Errorf("%d requests, 4xx must not be retried", n)
			}
		})
	}
}
