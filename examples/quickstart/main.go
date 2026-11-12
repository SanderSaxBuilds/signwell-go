// Command quickstart sends a PDF for signature in SignWell test mode and
// prints the new document's ID and status.
//
//	SIGNWELL_API_KEY=... go run ./examples/quickstart agreement.pdf jane@example.com "Jane Doe"
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	signwell "github.com/SanderSaxBuilds/signwell-go"
)

func main() {
	if len(os.Args) < 4 {
		log.Fatal("usage: quickstart FILE.pdf EMAIL NAME")
	}
	path, email, name := os.Args[1], os.Args[2], os.Args[3]

	client, err := signwell.New(os.Getenv("SIGNWELL_API_KEY"),
		signwell.WithTestMode(true), // test documents are free and not legally binding
	)
	if err != nil {
		log.Fatal(err)
	}

	file, err := signwell.FileFromPath(path)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	doc, err := client.Documents.Create(ctx, &signwell.CreateDocumentRequest{
		Name:       "Consulting agreement",
		Files:      []signwell.File{file},
		Recipients: []signwell.RecipientRequest{{ID: "1", Name: name, Email: email}},
		// SignWell adds a signature page at the end, so no field
		// coordinates are needed.
		WithSignaturePage: true,
	})
	var apiErr *signwell.APIError
	switch {
	case errors.As(err, &apiErr):
		for field, msgs := range apiErr.FieldErrors() {
			fmt.Fprintf(os.Stderr, "%s: %v\n", field, msgs)
		}
		log.Fatal(apiErr)
	case err != nil:
		log.Fatal(err)
	}

	fmt.Printf("created %s (%s)\n", doc.ID, doc.Status)
	for _, r := range doc.Recipients {
		fmt.Printf("  %s <%s>: %s\n", r.Name, r.Email, r.Status)
	}
}
