package email

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/emersion/go-smtp"

	"github.com/useopendesk/opendesk/internal/httpx"
)

// InboundSecretHeader authenticates the Cloudflare Email Worker (or SES relay).
const InboundSecretHeader = "OD-Inbound-Secret"

// EnvelopeFromHeader carries the SMTP envelope sender from the relay; empty is the null
// sender of a delivery report.
const EnvelopeFromHeader = "OD-Envelope-From"

func writeProblem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(httpx.E(status, code, detail))
}

// secretOK compares in constant time (hashing first so length does not leak either).
func secretOK(got, want string) bool {
	if want == "" {
		return false
	}
	a, b := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

// inboundHTTP accepts one raw RFC 822 message for a forwarding address.
func (s *service) inboundHTTP(w http.ResponseWriter, r *http.Request) {
	if s.d.Cfg.InboundSecret == "" {
		writeProblem(w, http.StatusNotFound, "not_found", "Inbound email is not enabled on this instance.")
		return
	}
	if !secretOK(r.Header.Get(InboundSecretHeader), s.d.Cfg.InboundSecret) {
		writeProblem(w, http.StatusUnauthorized, "unauthenticated", "Missing or wrong inbound secret.")
		return
	}
	rcpt := r.URL.Query().Get("to")
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRawBytes+1))
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig) || len(body) > maxRawBytes:
		writeProblem(w, http.StatusRequestEntityTooLarge, "too_large", "Messages are limited to 30 MB.")
		return
	case err != nil:
		writeProblem(w, http.StatusBadRequest, "bad_request", "Could not read the message.")
		return
	case len(bytes.TrimSpace(body)) == 0:
		writeProblem(w, http.StatusBadRequest, "bad_request", "The message is empty.")
		return
	}
	m, err := s.mailboxForRecipient(r.Context(), rcpt)
	if errors.Is(err, errUnknownRecipient) {
		writeProblem(w, http.StatusNotFound, "unknown_recipient", "No mailbox uses that forwarding address.")
		return
	}
	if err != nil {
		s.log.Error("inbound lookup", "err", err)
		writeProblem(w, http.StatusServiceUnavailable, "unavailable", "Try again later.")
		return
	}
	var envFrom *string
	if vals, ok := r.Header[http.CanonicalHeaderKey(EnvelopeFromHeader)]; ok && len(vals) > 0 {
		envFrom = &vals[0]
	}
	id, dup, err := s.ingest(r.Context(), m, body, rcpt, envFrom)
	if err != nil {
		s.log.Error("inbound ingest", "mailbox_id", m.ID, "err", err)
		writeProblem(w, http.StatusServiceUnavailable, "unavailable", "Try again later.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	out := map[string]any{"duplicate": dup}
	if !dup {
		out["id"] = id
	}
	_ = json.NewEncoder(w).Encode(out)
}

// smtpBackend is the built-in receiver for self-hosters (INBOUND_SMTP_ADDR). It takes
// mail only for existing forwarding addresses at INBOUND_DOMAIN.
type smtpBackend struct{ s *service }

func (b *smtpBackend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	ip := ""
	if addr, ok := c.Conn().RemoteAddr().(*net.TCPAddr); ok {
		ip = addr.IP.String()
	}
	if b.s.d.Limiter != nil && !b.s.d.Limiter.Allow("smtp-in:"+ip, 300, time.Minute) {
		return nil, &smtp.SMTPError{Code: 421, EnhancedCode: smtp.EnhancedCode{4, 7, 0}, Message: "Too many connections, try later"}
	}
	return &smtpSession{s: b.s}, nil
}

type smtpSession struct {
	s       *service
	envFrom string
	rcpts   []string
	boxes   []*mailboxRow
}

func (ss *smtpSession) Reset()        { ss.envFrom, ss.rcpts, ss.boxes = "", nil, nil }
func (ss *smtpSession) Logout() error { return nil }

func (ss *smtpSession) Mail(from string, _ *smtp.MailOptions) error {
	ss.envFrom = from
	return nil
}

func (ss *smtpSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m, err := ss.s.mailboxForRecipient(ctx, to)
	if errors.Is(err, errUnknownRecipient) {
		return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "No such mailbox"}
	}
	if err != nil {
		return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "Temporary failure, try later"}
	}
	for _, b := range ss.boxes {
		if b.ID == m.ID {
			return nil // two plus addresses of one mailbox: deliver once
		}
	}
	ss.rcpts = append(ss.rcpts, strings.ToLower(to))
	ss.boxes = append(ss.boxes, m)
	return nil
}

func (ss *smtpSession) Data(r io.Reader) error {
	raw, err := io.ReadAll(io.LimitReader(r, maxRawBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxRawBytes {
		return &smtp.SMTPError{Code: 552, EnhancedCode: smtp.EnhancedCode{5, 3, 4}, Message: "Message too big"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for i, m := range ss.boxes {
		envFrom := ss.envFrom
		if _, _, err := ss.s.ingest(ctx, m, raw, ss.rcpts[i], &envFrom); err != nil {
			ss.s.log.Error("smtp ingest", "mailbox_id", m.ID, "err", err)
			return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "Temporary failure, try later"}
		}
	}
	return nil
}

// newSMTPServer builds the receiver. It advertises no AUTH: it only accepts mail
// addressed to forwarding tokens.
func (s *service) newSMTPServer(addr string) *smtp.Server {
	srv := smtp.NewServer(&smtpBackend{s: s})
	srv.Addr = addr
	srv.Domain = s.inboundDomain()
	srv.MaxMessageBytes = maxRawBytes
	srv.MaxRecipients = 20
	srv.ReadTimeout = 60 * time.Second
	srv.WriteTimeout = 60 * time.Second
	srv.ErrorLog = smtpLogger{s}
	return srv
}

type smtpLogger struct{ s *service }

func (l smtpLogger) Printf(format string, v ...any) { l.s.log.Debug("smtp receiver", "msg", format) }
func (l smtpLogger) Println(v ...any)               { l.s.log.Debug("smtp receiver") }

// runSMTP serves until ctx ends.
func (s *service) runSMTP(ctx context.Context, addr string) {
	srv := s.newSMTPServer(addr)
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	s.log.Info("smtp receiver listening", "addr", addr, "domain", srv.Domain)
	if err := srv.ListenAndServe(); err != nil && ctx.Err() == nil && !errors.Is(err, smtp.ErrServerClosed) {
		s.log.Error("smtp receiver stopped", "err", err)
	}
}
