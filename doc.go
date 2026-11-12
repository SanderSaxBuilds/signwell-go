// Package signwell is a client for the SignWell e-signature API.
//
// Create a client with an API key from Settings, API in the SignWell app:
//
//	client, err := signwell.New(os.Getenv("SIGNWELL_API_KEY"))
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	doc, err := client.Documents.Create(ctx, &signwell.CreateDocumentRequest{
//		TestMode: true,
//		Name:     "NDA",
//		Files:    []signwell.File{signwell.FileFromURL("nda.pdf", "https://example.com/nda.pdf")},
//		Recipients: []signwell.RecipientRequest{
//			{ID: "1", Name: "Jane Doe", Email: "jane@example.com"},
//		},
//		WithSignaturePage: true,
//	})
//
// Every call takes a context.Context. Requests that fail with HTTP 429 or a
// 5xx status are retried with exponential backoff, see [WithMaxRetries].
// Errors returned by the API are *APIError values and match the sentinel
// errors in this package through errors.Is:
//
//	if errors.Is(err, signwell.ErrNotFound) {
//		// ...
//	}
//
// Webhook events can be checked with [VerifyEvent], [ParseWebhook] or
// [WebhookHandler].
package signwell
