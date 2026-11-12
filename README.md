# signwell-go

A Go client for the [SignWell](https://www.signwell.com) e-signature API. It covers documents, templates and webhooks, verifies webhook events, retries rate limits and server errors, and pages through lists for you. Standard library only, Go 1.22 or newer.

Names follow the official Python and Node SDKs, so code reads the same across the three.

## Install

```sh
go get github.com/SanderSaxBuilds/signwell-go
```

## Quick start

Get an API key from the API section of your SignWell account settings. Test mode documents are free and not legally binding, so turn it on while you build.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	signwell "github.com/SanderSaxBuilds/signwell-go"
)

func main() {
	client, err := signwell.New(os.Getenv("SIGNWELL_API_KEY"), signwell.WithTestMode(true))
	if err != nil {
		log.Fatal(err)
	}

	doc, err := client.Documents.Create(context.Background(), &signwell.CreateDocumentRequest{
		Name:  "Consulting agreement",
		Files: []signwell.File{signwell.FileFromURL("agreement.pdf", "https://example.com/agreement.pdf")},
		Recipients: []signwell.RecipientRequest{
			{ID: "1", Name: "Jane Doe", Email: "jane@example.com"},
		},
		WithSignaturePage: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(doc.ID, doc.Status)
}
```

A runnable version that uploads a local PDF is in [examples/quickstart](examples/quickstart).

## The client

`signwell.New(apiKey, options...)` returns a `*Client` with three services, `Documents`, `Templates` and `Webhooks`. The client is safe for concurrent use. Every call takes a `context.Context`, and cancelling it stops the request and any retry wait.

| Option | Default | What it does |
| --- | --- | --- |
| `WithTestMode(bool)` | off | Sends `X-TestMode-Enabled` on create and send calls. It overrides the `TestMode` field in the body. |
| `WithHTTPClient(*http.Client)` | 60 s timeout | Your own client, for proxies, tracing or custom transports. |
| `WithBaseURL(string)` | `https://www.signwell.com` | Another host, for a proxy or a local test server. |
| `WithUserAgent(string)` | `signwell-go/<version>` | Puts your app name in front of the default. |
| `WithMaxRetries(int)` | 3 | Retries after the first attempt. 0 turns retries off. |
| `WithRetryBackoff(initial, max)` | 500 ms, 30 s | Exponential backoff with jitter. |
| `WithMaxRetryAfter(time.Duration)` | 60 s | The longest `Retry-After` the client will wait. A longer one is returned as an error at once. |
| `WithRetryNonIdempotent(bool)` | off | Also retries POST calls after a 5xx or a network error. |

## Documents

```go
// Create from files. Place fields per file, or set WithSignaturePage or TextTags.
doc, err := client.Documents.Create(ctx, &signwell.CreateDocumentRequest{
	Name:       "NDA",
	Files:      []signwell.File{file},
	Recipients: []signwell.RecipientRequest{{ID: "1", Name: "Jane Doe", Email: "jane@example.com"}},
	Fields: [][]signwell.FieldRequest{{
		{X: 72, Y: 640, Page: 1, Type: signwell.FieldSignature, RecipientID: "1"},
		{X: 320, Y: 640, Page: 1, Type: signwell.FieldDate, RecipientID: "1"},
	}},
})

// Create from a template. Recipients are matched to placeholders by name.
doc, err = client.Documents.CreateFromTemplate(ctx, &signwell.CreateDocumentFromTemplateRequest{
	TemplateID: "2851644b-8c61-4901-ab97-43fee9106bec",
	Recipients: []signwell.RecipientRequest{
		{ID: "1", PlaceholderName: "Client", Name: "Jane Doe", Email: "jane@example.com"},
	},
	TemplateFields: []signwell.TemplateFieldValue{{APIID: "company_name", Value: "Acme Ltd"}},
})

// Send a draft. A nil request sends it as it is.
doc, err = client.Documents.Send(ctx, doc.ID, nil)

doc, err = client.Documents.Get(ctx, doc.ID)

// Remind everyone who has not signed, or name the recipients.
doc, err = client.Documents.Remind(ctx, doc.ID, &signwell.RemindRequest{
	Recipients: []signwell.ReminderRecipient{{Email: "jane@example.com"}},
})

err = client.Documents.Delete(ctx, doc.ID)
```

Files come from a URL, from bytes or from disk:

```go
signwell.FileFromURL("nda.pdf", "https://example.com/nda.pdf")
signwell.FileFromBytes("nda.pdf", pdfBytes)
file, err := signwell.FileFromPath("contracts/nda.pdf")
```

Fields whose API default is true, such as `Reminders` or `AllowDecline`, are `*bool`. Use `signwell.Bool(false)` to turn one off.

### Completed PDF

```go
dl, err := client.Documents.DownloadCompletedPDF(ctx, id, &signwell.CompletedPDFParams{
	AuditPage: signwell.Bool(true),
})
if err != nil {
	return err
}
defer dl.Body.Close()
out, _ := os.Create(dl.Filename)
_, err = io.Copy(out, dl.Body)
```

`FileFormat: signwell.FormatZIP` returns the files as a ZIP archive. If you only need a link, `CompletedPDFURL` returns a short-lived file URL instead of the bytes.

### Listing

`List` returns one page. `ListAll` returns an iterator that fetches the next page when it needs it.

```go
page, err := client.Documents.List(ctx, &signwell.ListParams{Page: 1, Limit: 50})
fmt.Println(page.TotalCount, page.HasNext())

it := client.Documents.ListAll(&signwell.ListParams{Limit: 50})
for it.Next(ctx) {
	doc := it.Current()
	fmt.Println(doc.ID, doc.Status)
}
if err := it.Err(); err != nil {
	return err
}
```

`it.Collect(ctx)` reads everything into a slice. The page size is capped at 50, the API maximum.

## Templates

```go
tpl, err := client.Templates.Create(ctx, &signwell.CreateTemplateRequest{
	Name:         "Service agreement",
	Files:        []signwell.File{file},
	Placeholders: []signwell.PlaceholderRequest{{ID: "1", Name: "Client"}},
	Fields: [][]signwell.FieldRequest{{
		{X: 72, Y: 640, Page: 1, Type: signwell.FieldSignature, PlaceholderID: "1"},
	}},
})

tpl, err = client.Templates.Get(ctx, tpl.ID)

it := client.Templates.ListAll(nil)
```

## Webhooks

```go
hook, err := client.Webhooks.Create(ctx, &signwell.CreateWebhookRequest{
	CallbackURL: "https://example.com/signwell/events",
})

hooks, err := client.Webhooks.List(ctx)

err = client.Webhooks.Delete(ctx, hook.ID)
```

Keep `hook.ID`. It is the key SignWell signs your events with.

## Verifying webhook events

SignWell signs each event with HMAC-SHA256. The key is the webhook ID and the message is the event type and time joined by `@`. The hex digest arrives in `event.hash`. The client checks it in constant time.

The simplest route is the handler. It reads the body, checks the signature and calls your function only for events that pass:

```go
http.Handle("/signwell/events", signwell.WebhookHandler(webhookID,
	func(ctx context.Context, p *signwell.WebhookPayload) error {
		switch p.Event.Type {
		case signwell.EventDocumentCompleted:
			doc, err := p.Document()
			if err != nil {
				return err
			}
			log.Printf("%s was signed by everyone", doc.Name)
		case signwell.EventDocumentDeclined:
			log.Printf("declined by %s", p.Event.RelatedSigner.Email)
		}
		return nil
	},
	signwell.WithTolerance(10*time.Minute),
))
```

It answers 200 when your function returns nil, 400 for a bad or unsigned event, 405 for anything but POST, 413 for a body over the size limit and 500 when your function returns an error, so SignWell knows to try again.

In your own handler, use `ParseWebhook(body, webhookID)` to check and decode in one step, or `VerifyEvent(event, webhookID)` when you already have the event. `WithTolerance` rejects events older or newer than the window with `ErrEventExpired`. It is off by default, since SignWell does not publish how long it keeps retrying.

## Errors

Every API failure is an `*APIError` with the status, the method and path, the API message and error code, the field errors and the raw body. It also matches a sentinel through `errors.Is`:

| Sentinel | Status |
| --- | --- |
| `ErrBadRequest` | 400 |
| `ErrUnauthorized` | 401 |
| `ErrForbidden` | 403 |
| `ErrNotFound` | 404 |
| `ErrConflict` | 409 |
| `ErrUnprocessableEntity` | 422 |
| `ErrRateLimited` | 429 |
| `ErrServer` | 5xx |

```go
doc, err := client.Documents.Get(ctx, id)
switch {
case errors.Is(err, signwell.ErrNotFound):
	// gone
case err != nil:
	var apiErr *signwell.APIError
	if errors.As(err, &apiErr) {
		for field, msgs := range apiErr.FieldErrors() {
			log.Printf("%s: %v", field, msgs) // recipients.recipient_1.email: [format is invalid.]
		}
	}
	return err
}
```

Bad input caught before a request, such as a missing file or an empty ID, matches `ErrInvalidArgument`. A 2xx body that is not the expected JSON is a `*DecodeError`.

## Retries

A 429 is always retried. The client waits for `Retry-After` when the API sends it, and uses exponential backoff with jitter otherwise. A 5xx (other than 501) or a network error is retried for GET and DELETE only, because retrying a create after a server error could make a second document. `WithRetryNonIdempotent(true)` changes that if you prefer. Request bodies are encoded once and sent the same way on every attempt.

SignWell allows 100 requests a minute, 20 a minute in test mode, and 30 a minute for document and template creation.

## Testing

```sh
go test ./...
go test -race ./...
go test -cover ./...
```

The tests run against `httptest` servers and replay JSON fixtures in [testdata](testdata) that follow the response examples in SignWell's API reference. Nothing calls the live API, and no key is needed. Retry waits are recorded instead of slept, so the suite runs in about a second.

To test your own code, point the client at a local server:

```go
srv := httptest.NewServer(handler)
defer srv.Close()
client, _ := signwell.New("key", signwell.WithBaseURL(srv.URL))
```

## License

MIT, see [LICENSE](LICENSE).
