package email

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/useopendesk/opendesk/internal/displayname"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// What the customer sees on every email (contracts/teams.md "Reply address"): the From
// name is the address's from_name_format filled in, the signature is the team's or else
// the address's, and both take the ticket's variables.

const (
	defaultFromNameFormat = "{agent} at {mailbox}"
	maxFromNameFormat     = 80
	maxAliases            = 20
)

// fromNameVars are the variables of from_name_format.
var fromNameVars = []string{"agent", "agent_first", "team", "brand", "mailbox"}

var fromNameVarRe = regexp.MustCompile(`\{([a-z_]+)\}`)

// cleanFromNameFormat validates a from_name_format: up to 80 characters, only the known
// variables, no markup or control characters. "" is the default format.
func cleanFromNameFormat(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return defaultFromNameFormat, nil
	}
	if utf8.RuneCountInString(s) > maxFromNameFormat {
		return "", httpx.Validation("from_name_format must be 80 characters or less")
	}
	if strings.ContainsAny(s, "<>") || strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return "", httpx.Validation("from_name_format cannot contain < or > characters")
	}
	rest := fromNameVarRe.ReplaceAllStringFunc(s, func(m string) string {
		if isFromNameVar(m[1 : len(m)-1]) {
			return ""
		}
		return m
	})
	if strings.ContainsAny(rest, "{}") {
		return "", httpx.Validation("from_name_format takes only the variables {agent}, {agent_first}, {team}, {brand} and {mailbox}")
	}
	return s, nil
}

func isFromNameVar(v string) bool {
	for _, k := range fromNameVars {
		if k == v {
			return true
		}
	}
	return false
}

// senderNames are the values a From name and a signature are filled with.
type senderNames struct {
	agent   string // the author's full name, Ollie's bot name, or "" without an author
	team    string // the ticket's team, else ""
	brand   string
	mailbox string // the address's display_name, else its name
}

func (n senderNames) agentFirst() string {
	first, _, _ := strings.Cut(strings.TrimSpace(n.agent), " ")
	return first
}

func (n senderNames) teamOrBrand() string {
	if n.team != "" {
		return n.team
	}
	return n.brand
}

// fromName fills format. Without an author the agent variable and the words between it
// and the next variable are dropped; a format left with no variable reads the mailbox.
func fromName(format string, n senderNames) string {
	if strings.TrimSpace(format) == "" {
		format = defaultFromNameFormat
	}
	type seg struct {
		text   string
		isVar  bool
		isUser bool
	}
	var segs []seg
	last := 0
	for _, loc := range fromNameVarRe.FindAllStringSubmatchIndex(format, -1) {
		name := format[loc[2]:loc[3]]
		if !isFromNameVar(name) {
			continue
		}
		if loc[0] > last {
			segs = append(segs, seg{text: format[last:loc[0]]})
		}
		segs = append(segs, seg{text: name, isVar: true, isUser: name == "agent" || name == "agent_first"})
		last = loc[1]
	}
	if last < len(format) {
		segs = append(segs, seg{text: format[last:]})
	}
	noAuthor := strings.TrimSpace(n.agent) == ""
	if noAuthor {
		kept := segs[:0:0]
		others := false
		skip := false
		for _, sg := range segs {
			switch {
			case sg.isUser:
				skip = true
				continue
			case sg.isVar:
				skip = false
				others = true
			case skip:
				continue
			}
			kept = append(kept, sg)
		}
		if !others {
			return n.mailbox
		}
		segs = kept
	}
	var b strings.Builder
	for _, sg := range segs {
		if !sg.isVar {
			b.WriteString(sg.text)
			continue
		}
		switch sg.text {
		case "agent":
			b.WriteString(n.agent)
		case "agent_first":
			b.WriteString(n.agentFirst())
		case "team":
			b.WriteString(n.teamOrBrand())
		case "brand":
			b.WriteString(n.brand)
		case "mailbox":
			b.WriteString(n.mailbox)
		}
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	out = strings.TrimFunc(out, func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune("-–—|,:;·/", r) })
	out = truncate(out, 200)
	if out == "" {
		return n.mailbox
	}
	return out
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
