package email

import (
	"strings"

	"github.com/useopendesk/opendesk/internal/displayname"
	"github.com/useopendesk/opendesk/internal/fromname"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// What the customer sees on every email (contracts/teams.md "Reply address"): the From
// name is the address's from_name_format filled in, the signature is the team's or else
// the address's, and both take the ticket's variables.

const (
	defaultFromNameFormat = "{agent} at {mailbox}"
	maxAliases            = 20
)

// senderNames are the values a From name is filled with: Agent (the author's full name,
// Ollie's bot name, or "" without an author), Team (the ticket's team, else ""), Brand,
// and Mailbox (the address's display_name, else its name).
type senderNames = fromname.Names

// cleanFromNameFormat validates a from_name_format: up to 80 characters, only the known
// variables, no markup or control characters. "" is the default format.
func cleanFromNameFormat(s string) (string, error) {
	s, err := fromname.Clean("from_name_format", s, fromname.Helpdesk)
	if err != nil || s != "" {
		return s, err
	}
	return defaultFromNameFormat, nil
}

// fromName fills format (internal/fromname): without an author the agent variable and the
// words between it and the next variable are dropped; a format left with no variable reads
// the mailbox.
func fromName(format string, n senderNames) string {
	if strings.TrimSpace(format) == "" {
		format = defaultFromNameFormat
	}
	return fromname.Fill(format, fromname.Helpdesk, n)
}

// cleanAliases normalizes a mailbox's aliases: at most 20 distinct valid addresses, none
// the mailbox's own address and none on the inbound forwarding domain.
func (s *service) cleanAliases(own string, list []string) ([]string, error) {
	if len(list) > maxAliases {
		return nil, httpx.Validation("A mailbox can have at most 20 aliases.")
	}
	out := make([]string, 0, len(list))
	for _, a := range list {
		a = strings.ToLower(strings.TrimSpace(a))
		if !validAddress(a) {
			return nil, httpx.Validation("Enter valid email addresses for the aliases.")
		}
		if strings.EqualFold(emailDomain(a), s.inboundDomain()) {
			return nil, httpx.Validation("An alias cannot be on the inbound forwarding domain.")
		}
		if strings.EqualFold(a, own) {
			return nil, httpx.Validation("An alias cannot be the mailbox's own address.")
		}
		for _, b := range out {
			if b == a {
				return nil, httpx.Validation("Aliases must not repeat.")
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// agentName is a person's name as shown to customers.
func agentName(name string) string { return displayname.Displayable(name) }
