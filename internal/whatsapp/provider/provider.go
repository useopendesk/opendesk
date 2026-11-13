// Package provider is the boundary between the WhatsApp engine (internal/whatsapp) and a
// business solution provider such as Twilio (contracts/whatsapp.md). An adapter speaks its
// provider's protocol and nothing else: it never imports a helpdesk package, reads a table
// or decides anything about tickets. The engine owns connections, sealed credentials,
// contact matching, placement, the timeline and every ticket write.
package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Descriptor describes a provider for the settings page.
type Descriptor struct {
	Key    string  `json:"key"`
	Name   string  `json:"name"`
	Fields []Field `json:"fields"`
	// Docs says where an admin sets the webhook URL in the provider.
	Docs string `json:"docs"`
}

// Field is one credential an admin pastes.
type Field struct {
	Name   string `json:"name"`
	Label  string `json:"label"`
	Secret bool   `json:"secret"`
}

// Account is the provider account the credentials open.
type Account struct{ Name string }

// Event kinds.
const (
	KindMessage = "message"
	KindStatus  = "status"
)

// Delivery statuses of an outbound message, in the order they happen. A message only
// moves forward through them; Failed ends it.
const (
	StatusSent      = "sent"
	StatusDelivered = "delivered"
	StatusRead      = "read"
	StatusFailed    = "failed"
)

// Event is one provider webhook, normalized. Only what the engine needs crosses the
// boundary; the raw payload is never stored.
type Event struct {
	// ID deduplicates the provider's retries of the same event.
	ID   string `json:"id"`
	Kind string `json:"kind"`
	// ExternalID is the provider's id of the message the event is about.
	ExternalID string `json:"external_id"`
	// Number is the customer's number, in E.164.
	Number string `json:"number"`
	// Name is the name the customer's WhatsApp profile gives, when the provider says it.
	Name string `json:"name,omitempty"`
	// Text is a message's words.
	Text  string  `json:"text,omitempty"`
	Media []Media `json:"media,omitempty"`
	// Status and Code are a delivery status event's: the status and, for a failure, the
	// provider's error code.
	Status string    `json:"status,omitempty"`
	Code   string    `json:"code,omitempty"`
	At     time.Time `json:"at"`
}

// Media is a file a customer sent; the engine fetches it with Adapter.Media.
type Media struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
}

// WhatsAppTemplate is a message the provider's WhatsApp account may send outside the
// customer-service window.
type WhatsAppTemplate struct {
	// ID is the provider's id of it, what Send takes.
	ID       string `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language"`
	// Body is its text, with {{1}}, {{2}} where the variables go.
	Body string `json:"body"`
	// Variables are the names of the placeholders ("1", "2", ...).
	Variables []string `json:"variables"`
}

// Outbound is a message to send.
type Outbound struct {
	// From is the sender's number and To the customer's, both E.164.
	From, To string
	Text     string
	// TemplateID and Variables send an approved template instead of Text.
	TemplateID string
	Variables  map[string]string
	// StatusURL is where the provider reports the message's delivery.
	StatusURL string
}

// Request is a webhook call as it came in: the URL the provider was given, its headers and
// its form.
type Request struct {
	URL    string
	Header http.Header
	Form   url.Values
}

// Adapter is a WhatsApp provider.
type Adapter interface {
	Describe() Descriptor
	// Validate checks the credentials and names the account they open.
	Validate(ctx context.Context, hc *http.Client, creds map[string]string) (Account, error)
	// Verify checks that the request came from the provider, with the connection's
	// credentials, before anything in it is read.
	Verify(r Request, creds map[string]string) error
	// ParseMessage reads a customer's message; ParseStatus a delivery status. Both answer
	// ErrIgnored for what is of no use.
	ParseMessage(form url.Values) (Event, error)
	ParseStatus(form url.Values) (Event, error)
	// Send sends a message and answers the provider's id of it.
	Send(ctx context.Context, hc *http.Client, creds map[string]string, m Outbound) (string, error)
	// Templates lists the approved templates of the account.
	Templates(ctx context.Context, hc *http.Client, creds map[string]string) ([]WhatsAppTemplate, error)
	// Media opens a file a customer sent, at most limit bytes.
	Media(ctx context.Context, hc *http.Client, creds map[string]string, m Media, limit int64) (io.ReadCloser, error)
}

// ErrIgnored is Parse's answer for an event the engine has no use for: it is
// acknowledged and dropped.
var ErrIgnored = errors.New("whatsapp event ignored")

// ErrUnauthorized is Verify's answer for a request the provider did not sign.
var ErrUnauthorized = errors.New("whatsapp request not verified")

// StatusError is a request the provider answered with an error status. It names the
// operation and the status, and the provider's error code when it gave one, never
// credentials or the provider's text.
type StatusError struct {
	Op     string
	Status int
	Code   string
}

func (e *StatusError) Error() string {
	s := e.Op + ": status " + strconv.Itoa(e.Status)
	if e.Code != "" {
		s += " code " + e.Code
	}
	return s
}

// Refused reports whether err is the provider refusing the credentials.
func Refused(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && (se.Status == http.StatusUnauthorized || se.Status == http.StatusForbidden)
}

// RateLimited is returned by an adapter when the provider asked to wait.
type RateLimited struct{ RetryAfter time.Duration }

func (e *RateLimited) Error() string { return "provider rate limit reached" }

// OutsideWindow is Send's answer when the provider refuses a free-form message because
// the customer has not written within the customer-service window.
var OutsideWindow = errors.New("outside the customer-service window")

// Rejected is Send's answer when the provider refuses a message for good (a number that
// is not on WhatsApp, a template that was not approved); Reason is safe to show an agent.
type Rejected struct{ Reason string }

func (e *Rejected) Error() string { return e.Reason }
