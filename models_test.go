package signwell

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNumberUnmarshal(t *testing.T) {
	tests := []struct {
		in      string
		want    Number
		wantErr bool
	}{
		{`12`, 12, false},
		{`12.5`, 12.5, false},
		{`-3`, -3, false},
		{`"13.0"`, 13, false},
		{`"112.0"`, 112, false},
		{`""`, 0, false},
		{`null`, 0, false},
		{`"wide"`, 0, true},
		{`true`, 0, true},
		{`"\u00`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			var n Number
			err := n.UnmarshalJSON([]byte(tt.in))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && n != tt.want {
				t.Errorf("Number = %v, want %v", n, tt.want)
			}
			if n.Float64() != float64(n) {
				t.Error("Float64 mismatch")
			}
		})
	}
}

func TestDropdownOptionUnmarshal(t *testing.T) {
	tests := []struct {
		in   string
		want DropdownOption
	}{
		{`"Option A"`, DropdownOption{Name: "Option A"}},
		{`{"name":"Other","api_id":"opt_3","is_other":true}`, DropdownOption{Name: "Other", APIID: "opt_3", IsOther: true}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			var o DropdownOption
			if err := json.Unmarshal([]byte(tt.in), &o); err != nil {
				t.Fatal(err)
			}
			if o != tt.want {
				t.Errorf("option = %+v, want %+v", o, tt.want)
			}
		})
	}
	out, _ := json.Marshal(DropdownOption{Name: "Yes"})
	if string(out) != `{"name":"Yes"}` {
		t.Errorf("marshal = %s", out)
	}
}

func TestEmbeddedBrandingUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantNil bool
		accent  string
	}{
		{"null", `{"embedded":null}`, true, ""},
		{"false", `{"embedded":false}`, false, ""},
		{"true", `{"embedded":true}`, false, ""},
		{"object", `{"embedded":{"branding":{"company_logo":"provided","accent_color":"#00aa88","button_border_radius":6}}}`, false, "#00aa88"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Document
			if err := json.Unmarshal([]byte(tt.in), &d); err != nil {
				t.Fatal(err)
			}
			if (d.Embedded == nil) != tt.wantNil {
				t.Fatalf("Embedded = %+v", d.Embedded)
			}
			if d.Embedded != nil && d.Embedded.Branding.AccentColor != tt.accent {
				t.Errorf("accent = %q", d.Embedded.Branding.AccentColor)
			}
		})
	}
}

func TestDocumentFixturesDecode(t *testing.T) {
	for _, name := range []string{"document.json", "document_created.json", "document_from_template.json", "document_sent.json", "document_reminded.json"} {
		t.Run(name, func(t *testing.T) {
			var d Document
			if err := json.Unmarshal(fixture(t, name), &d); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if d.ID == "" || d.CreatedAt.IsZero() || len(d.Recipients) == 0 || len(d.Files) == 0 {
				t.Errorf("document %s is missing data", d.ID)
			}
			for _, group := range d.Fields {
				for _, f := range group {
					if f.APIID == "" || f.Type == "" {
						t.Errorf("field %+v has no api_id or type", f)
					}
				}
			}
		})
	}
}

func TestTemplateFixturesDecode(t *testing.T) {
	for _, name := range []string{"template.json", "template_created.json"} {
		t.Run(name, func(t *testing.T) {
			var tpl Template
			if err := json.Unmarshal(fixture(t, name), &tpl); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if tpl.ID == "" || len(tpl.Placeholders) == 0 || tpl.TemplateLink == "" {
				t.Errorf("template %s is missing data", tpl.ID)
			}
		})
	}
}

func TestFileHelpers(t *testing.T) {
	if f := FileFromURL("a.pdf", "https://example.com/a.pdf"); f.Name != "a.pdf" || f.FileURL == "" || f.FileBase64 != "" {
		t.Errorf("FileFromURL = %+v", f)
	}
	f := FileFromBytes("b.pdf", []byte("hello"))
	if dec, _ := base64.StdEncoding.DecodeString(f.FileBase64); string(dec) != "hello" || f.FileURL != "" {
		t.Errorf("FileFromBytes = %+v", f)
	}
	out, _ := json.Marshal(f)
	if string(out) != `{"name":"b.pdf","file_base64":"aGVsbG8="}` {
		t.Errorf("marshal = %s", out)
	}

	path := filepath.Join(t.TempDir(), "contract.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4"), 0o600); err != nil {
		t.Fatal(err)
	}
	pf, err := FileFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if pf.Name != "contract.pdf" || pf.FileBase64 != base64.StdEncoding.EncodeToString([]byte("%PDF-1.4")) {
		t.Errorf("FileFromPath = %+v", pf)
	}
	if _, err := FileFromPath(filepath.Join(t.TempDir(), "missing.pdf")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file err = %v", err)
	}
}

func TestPaginationHasNext(t *testing.T) {
	tests := []struct {
		name string
		p    Pagination
		want bool
	}{
		{"last page", Pagination{CurrentPage: 2}, false},
		{"next page", Pagination{CurrentPage: 1, NextPage: Int(2)}, true},
		{"next equals current", Pagination{CurrentPage: 2, NextPage: Int(2)}, false},
		{"next behind", Pagination{CurrentPage: 3, NextPage: Int(1)}, false},
	}
	for _, tt := range tests {
		if got := tt.p.HasNext(); got != tt.want {
			t.Errorf("%s: HasNext = %v", tt.name, got)
		}
	}
}

func TestPointerHelpers(t *testing.T) {
	if b := Bool(false); b == nil || *b {
		t.Error("Bool(false)")
	}
	if i := Int(7); i == nil || *i != 7 {
		t.Error("Int(7)")
	}
}

func TestFieldRequestEncoding(t *testing.T) {
	f := FieldRequest{X: 0, Y: 0, Page: 1, Type: FieldCheckbox, RecipientID: "1", Value: false, Required: Bool(false)}
	out, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	// x and y are always sent, even at zero, since 0,0 is a valid position.
	for _, k := range []string{"x", "y", "page", "type", "recipient_id", "required"} {
		if _, ok := m[k]; !ok {
			t.Errorf("%s missing from %s", k, out)
		}
	}
	if _, ok := m["placeholder_id"]; ok {
		t.Errorf("placeholder_id sent: %s", out)
	}
}
