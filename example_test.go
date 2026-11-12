package signwell_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	signwell "github.com/SanderSaxBuilds/signwell-go"
)

func ExampleClient_documentsCreate() {
	// A stand-in for www.signwell.com, so the example runs offline.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":"8a7c2b1e-0d4f-4c2a-9b61-3e5f7a9c1d20","name":"NDA","status":"Sent","test_mode":true}`)
	}))
	defer srv.Close()

	client, err := signwell.New("YOUR_API_KEY", signwell.WithBaseURL(srv.URL))
	if err != nil {
		panic(err)
	}
	doc, err := client.Documents.Create(context.Background(), &signwell.CreateDocumentRequest{
		TestMode: true,
		Name:     "NDA",
		Files:    []signwell.File{signwell.FileFromURL("nda.pdf", "https://example.com/nda.pdf")},
		Recipients: []signwell.RecipientRequest{
			{ID: "1", Name: "Jane Doe", Email: "jane@example.com"},
		},
		Fields: [][]signwell.FieldRequest{{
			{X: 72, Y: 640, Page: 1, Type: signwell.FieldSignature, RecipientID: "1"},
		}},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(doc.Name, doc.Status, doc.TestMode)
	// Output: NDA Sent true
}

func ExampleAPIError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Not found","meta":{"error":"record_not_found","message":"Couldn't find the document requested","messages":["Couldn't find the document requested"]}}`)
	}))
	defer srv.Close()

	client, _ := signwell.New("YOUR_API_KEY", signwell.WithBaseURL(srv.URL))
	_, err := client.Documents.Get(context.Background(), "missing")

	var apiErr *signwell.APIError
	if errors.As(err, &apiErr) {
		fmt.Println(apiErr.StatusCode, apiErr.Code)
	}
	fmt.Println(errors.Is(err, signwell.ErrNotFound))
	// Output:
	// 404 record_not_found
	// true
}

func ExampleVerifyEvent() {
	webhookID := "6ff542d5-590c-44b2-8131-7ae998e78c5b"
	event := signwell.WebhookEvent{Type: signwell.EventDocumentCompleted, Time: "1689332300"}
	event.Hash = signwell.Signature(webhookID, event.Type, event.Time)

	fmt.Println(signwell.VerifyEvent(event, webhookID))
	fmt.Println(signwell.VerifyEvent(event, "some-other-webhook"))
	// Output:
	// <nil>
	// signwell: webhook signature does not match
}

func ExampleIterator() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, `{"documents":[{"id":"c","test_mode":false}],"current_page":2,"next_page":null,"previous_page":1,"total_count":3,"total_pages":2}`)
			return
		}
		fmt.Fprint(w, `{"documents":[{"id":"a","test_mode":false},{"id":"b","test_mode":false}],"current_page":1,"next_page":2,"previous_page":null,"total_count":3,"total_pages":2}`)
	}))
	defer srv.Close()

	client, _ := signwell.New("YOUR_API_KEY", signwell.WithBaseURL(srv.URL))
	ctx := context.Background()
	it := client.Documents.ListAll(&signwell.ListParams{Limit: 2})
	for it.Next(ctx) {
		fmt.Println(it.Current().ID)
	}
	if err := it.Err(); err != nil {
		panic(err)
	}
	// Output:
	// a
	// b
	// c
}
