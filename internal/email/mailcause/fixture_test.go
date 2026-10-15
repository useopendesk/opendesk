package mailcause

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// webFixture is the catalogue as the web app's mock serves it, so the mock answers
// with the same codes, titles and fixes as the API.
var webFixture = filepath.Join("..", "..", "..", "web", "src", "mock", "mailCauses.json")

// TestWebFixture fails when the web mock's copy of the catalogue differs from this
// one. UPDATE_MAILCAUSES=1 go test ./internal/email/mailcause rewrites it.
func TestWebFixture(t *testing.T) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(catalogue); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("UPDATE_MAILCAUSES") != "" {
		if err := os.WriteFile(webFixture, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(webFixture)
	if err != nil {
		t.Fatalf("read %s: %v (UPDATE_MAILCAUSES=1 go test ./internal/email/mailcause writes it)", webFixture, err)
	}
	if !bytes.Equal(got, buf.Bytes()) {
		t.Fatalf("%s differs from the catalogue; run UPDATE_MAILCAUSES=1 go test ./internal/email/mailcause", webFixture)
	}
}
