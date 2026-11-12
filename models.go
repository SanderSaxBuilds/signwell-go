package signwell

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Bool returns a pointer to v. Use it for request fields whose API default
// is true, such as Reminders or AllowDecline.
func Bool(v bool) *bool { return &v }

// Int returns a pointer to v.
func Int(v int) *int { return &v }

// FieldType is the kind of a document field.
type FieldType string

// Field types accepted by the API.
const (
	FieldInitials           FieldType = "initials"
	FieldSignature          FieldType = "signature"
	FieldCheckbox           FieldType = "checkbox"
	FieldDate               FieldType = "date"
	FieldSelect             FieldType = "select"
	FieldText               FieldType = "text"
	FieldDropdown           FieldType = "dropdown"
	FieldAutofillCompany    FieldType = "autofill_company"
	FieldAutofillEmail      FieldType = "autofill_email"
	FieldAutofillFirstName  FieldType = "autofill_first_name"
	FieldAutofillLastName   FieldType = "autofill_last_name"
	FieldAutofillName       FieldType = "autofill_name"
	FieldAutofillPhone      FieldType = "autofill_phone"
	FieldAutofillTitle      FieldType = "autofill_title"
	FieldAutofillDateSigned FieldType = "autofill_date_signed"
)

// TextValidation is a validation rule for text fields.
type TextValidation string

// Text validation rules.
const (
	ValidationNone                TextValidation = "no_text_validation"
	ValidationNumbers             TextValidation = "numbers"
	ValidationLetters             TextValidation = "letters"
	ValidationEmailAddress        TextValidation = "email_address"
	ValidationUSPhoneNumber       TextValidation = "us_phone_number"
	ValidationUSZipCode           TextValidation = "us_zip_code"
	ValidationUSSSN               TextValidation = "us_ssn"
	ValidationUSAge               TextValidation = "us_age"
	ValidationAlphanumeric        TextValidation = "alphanumeric"
	ValidationUSBankRoutingNumber TextValidation = "us_bank_routing_number"
	ValidationUSBankAccountNumber TextValidation = "us_bank_account_number"
	ValidationCustom              TextValidation = "custom"
)

// DateFormat is the display format of a date field.
type DateFormat string

// Date formats.
const (
	DateMMDDYYYY         DateFormat = "MM/DD/YYYY"
	DateDDMMYYYY         DateFormat = "DD/MM/YYYY"
	DateYYYYMMDD         DateFormat = "YYYY/MM/DD"
	DateMonthDDYYYY      DateFormat = "Month DD, YYYY"
	DateMMDDYYYYWithTime DateFormat = "MM/DD/YYYY hh:mm:ss a"
)

// DeliveryMethod is how a recipient receives the signing request.
type DeliveryMethod string

// Delivery methods.
const (
	DeliveryEmail       DeliveryMethod = "email"
	DeliverySMS         DeliveryMethod = "sms"
	DeliveryEmailAndSMS DeliveryMethod = "email_and_sms"
)

// CheckboxValidation is the selection rule of a checkbox group.
type CheckboxValidation string

// Checkbox group rules.
const (
	CheckboxMinimum CheckboxValidation = "minimum"
	CheckboxMaximum CheckboxValidation = "maximum"
	CheckboxRange   CheckboxValidation = "range"
	CheckboxExact   CheckboxValidation = "exact"
)

// FileFormat is the format of a completed document download.
type FileFormat string

// Download formats.
const (
	FormatPDF FileFormat = "pdf"
	FormatZIP FileFormat = "zip"
)

// Number is a float that decodes from a JSON number or a numeric string.
// SignWell returns field sizes as strings, for example "112.0".
type Number float64

// UnmarshalJSON accepts 12, 12.5, "12.5" and null.
func (n *Number) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		return nil
	}
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		if s == "" {
			*n = 0
			return nil
		}
		data = []byte(s)
	}
	f, err := strconv.ParseFloat(string(data), 64)
	if err != nil {
		return fmt.Errorf("signwell: %q is not a number", data)
	}
	*n = Number(f)
	return nil
}

// Float64 returns n as a float64.
func (n Number) Float64() float64 { return float64(n) }

// File is a document file in a create request. Set exactly one of FileURL
// and FileBase64. The helpers FileFromURL, FileFromBytes and FileFromPath
// build one.
type File struct {
	Name       string `json:"name,omitempty"`
	FileURL    string `json:"file_url,omitempty"`
	FileBase64 string `json:"file_base64,omitempty"`
}

// FileFromURL returns a File that SignWell downloads from url.
func FileFromURL(name, url string) File {
	return File{Name: name, FileURL: url}
}

// FileFromBytes returns a File uploaded inline as base64.
func FileFromBytes(name string, data []byte) File {
	return File{Name: name, FileBase64: base64.StdEncoding.EncodeToString(data)}
}

// FileFromPath reads a local file and returns it as an inline upload named
// after the file.
func FileFromPath(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, fmt.Errorf("signwell: read file: %w", err)
	}
	return FileFromBytes(filepath.Base(path), data), nil
}

// FileInfo describes a file attached to a document or template.
type FileInfo struct {
	Name        string `json:"name"`
	PagesNumber int    `json:"pages_number"`
}

// PasscodeDelivery controls whether a recipient's passcode is sent to them.
type PasscodeDelivery struct {
	Enabled           bool     `json:"enabled"`
	Methods           []string `json:"methods,omitempty"`
	ExpireAfterAccess bool     `json:"expire_after_access,omitempty"`
}

// Label tags a document so it can be found in SignWell search.
type Label struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

// CopiedContact receives the final document once everyone has signed.
type CopiedContact struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

// AttachmentRequest asks a recipient to upload a file while signing. Use
// RecipientID on documents and PlaceholderID on templates.
type AttachmentRequest struct {
	Name          string `json:"name"`
	RecipientID   string `json:"recipient_id,omitempty"`
	PlaceholderID string `json:"placeholder_id,omitempty"`
	Required      bool   `json:"required"`
}

// RecipientAttachment is an attachment request as it appears on a recipient,
// with the URL of the uploaded file once there is one.
type RecipientAttachment struct {
	Name     string `json:"name"`
	URL      string `json:"url,omitempty"`
	Required bool   `json:"required"`
}

// CheckboxGroup groups checkbox fields under one selection rule. Use
// RecipientID on documents and PlaceholderID on templates.
type CheckboxGroup struct {
	ID            string             `json:"id,omitempty"`
	GroupName     string             `json:"group_name,omitempty"`
	RecipientID   string             `json:"recipient_id,omitempty"`
	PlaceholderID string             `json:"placeholder_id,omitempty"`
	CheckboxIDs   []string           `json:"checkbox_ids"`
	Validation    CheckboxValidation `json:"validation,omitempty"`
	Required      bool               `json:"required"`
	MinValue      *int               `json:"min_value,omitempty"`
	MaxValue      *int               `json:"max_value,omitempty"`
	ExactValue    *int               `json:"exact_value,omitempty"`
}

// ConditionalRule shows, hides or requires fields based on other fields.
type ConditionalRule struct {
	ID             string      `json:"id,omitempty"`
	Deleted        bool        `json:"deleted,omitempty"`
	TargetID       string      `json:"target_id"`
	ActionType     string      `json:"action_type"`
	ActionFieldIDs []string    `json:"action_field_ids,omitempty"`
	Conditions     []Condition `json:"conditions,omitempty"`
}

// Condition is one test inside a ConditionalRule.
type Condition struct {
	ConditionType string   `json:"condition_type,omitempty"`
	Status        string   `json:"status,omitempty"`
	JoinOperator  string   `json:"join_operator,omitempty"`
	FieldAPIIDs   []string `json:"field_api_ids,omitempty"`
	Position      int      `json:"position,omitempty"`
}

// ConditionalLogicDecision records how a conditional rule was resolved.
type ConditionalLogicDecision struct {
	RuleID       string    `json:"rule_id"`
	FieldAPIID   string    `json:"field_api_id"`
	DecisionType string    `json:"decision_type"`
	DecidedAt    time.Time `json:"decided_at"`
}

// DropdownOption is one choice in a dropdown field. The API accepts a plain
// string or an object, this type decodes both and encodes an object.
type DropdownOption struct {
	Name    string `json:"name"`
	APIID   string `json:"api_id,omitempty"`
	IsOther bool   `json:"is_other,omitempty"`
}

// UnmarshalJSON accepts "Option A" as well as {"name": "Option A"}.
func (o *DropdownOption) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &o.Name)
	}
	type plain DropdownOption
	return json.Unmarshal(data, (*plain)(o))
}

// EmbeddedRequest sets branding for embedded signing.
type EmbeddedRequest struct {
	Branding *BrandingRequest `json:"branding,omitempty"`
}

// BrandingRequest is the look of the embedded signing page.
type BrandingRequest struct {
	CompanyLogo        *File  `json:"company_logo,omitempty"`
	AccentColor        string `json:"accent_color,omitempty"`
	ButtonLabelColor   string `json:"button_label_color,omitempty"`
	LinkColor          string `json:"link_color,omitempty"`
	ButtonBorderRadius *int   `json:"button_border_radius,omitempty"`
}

// EmbeddedBranding is the branding stored on a document.
type EmbeddedBranding struct {
	Branding struct {
		CompanyLogo        string `json:"company_logo,omitempty"`
		AccentColor        string `json:"accent_color,omitempty"`
		ButtonLabelColor   string `json:"button_label_color,omitempty"`
		LinkColor          string `json:"link_color,omitempty"`
		ButtonBorderRadius *int   `json:"button_border_radius,omitempty"`
	} `json:"branding"`
}

// UnmarshalJSON also accepts true and false, which webhook payloads send in
// place of the branding object.
func (e *EmbeddedBranding) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("true")) || bytes.Equal(data, []byte("false")) {
		return nil
	}
	type plain EmbeddedBranding
	return json.Unmarshal(data, (*plain)(e))
}

// Pagination is the paging block of list responses.
type Pagination struct {
	CurrentPage  int  `json:"current_page"`
	NextPage     *int `json:"next_page"`
	PreviousPage *int `json:"previous_page"`
	TotalCount   int  `json:"total_count"`
	TotalPages   int  `json:"total_pages"`
}

// HasNext reports whether there is a page after this one.
func (p Pagination) HasNext() bool { return p.NextPage != nil && *p.NextPage > p.CurrentPage }

// ListParams filters and pages a list call. Zero values use the API
// defaults: page 1 and 10 items per page.
type ListParams struct {
	// Page is the page number, starting at 1.
	Page int
	// Limit is the page size. The API maximum is 50, larger values are lowered to 50.
	Limit int
	// Query is a SignWell search query, for example "status:completed AND name:NDA".
	Query string
}

// MaxPageSize is the largest page the list endpoints return.
const MaxPageSize = 50
