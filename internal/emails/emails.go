// Package emails builds every platform email opendesk sends: account emails (confirm,
// reset, invite), billing emails and notification emails. Each builder is a pure
// function of its input, so cmd/emailpreview renders all of them with sample data, and
// all of them share the layout in internal/mailer.
package emails

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/useopendesk/opendesk/internal/config"
	"github.com/useopendesk/opendesk/internal/mailer"
)

// Contact is opendesk's one contact address, shown in Cloud emails.
const Contact = "team@opendesk.sh"

// Site is what emails need from the instance.
type Site struct {
	AppURL string
	APIURL string
	// Contact is an address for questions in footers; empty on self-hosted instances,
	// whose people ask their own admins.
	Contact string
	// Legal is the sending company's identification line (config.EmailLegal); empty
	// leaves it out, so an instance never names a company its operator did not set.
	Legal string
}

// SiteFrom reads the instance's site. The Cloud (billing on) shows opendesk's contact.
func SiteFrom(c *config.Config) Site {
	s := Site{AppURL: c.AppURL, APIURL: c.APIURL, Legal: c.EmailLegal.Line()}
	if c.BillingEnabled() {
		s.Contact = Contact
	}
	return s
}

// App is an app link with optional query parameters, given as key, value pairs.
func (s Site) App(path string, kv ...string) string {
	u := strings.TrimRight(s.AppURL, "/") + path
	if len(kv) >= 2 {
		v := url.Values{}
		for i := 0; i+1 < len(kv); i += 2 {
			if kv[i+1] != "" {
				v.Set(kv[i], kv[i+1])
			}
		}
		if q := v.Encode(); q != "" {
			u += "?" + q
		}
	}
	return u
}

// build renders e with the instance's footer additions.
func (s Site) build(e mailer.Email) mailer.Mail {
	e.Footer.Legal = s.Legal
	html, text := mailer.Render(e)
	return mailer.Mail{Subject: e.Subject, HTML: html, Text: text}
}

// VerifyEmail asks a new account to confirm its address.
func VerifyEmail(s Site, link string) mailer.Mail {
	return s.build(mailer.Email{
		Subject:   "Confirm your email",
		Preheader: "One click to finish setting up your opendesk account.",
		Heading:   "Confirm your email",
		Intro:     []string{"Confirm this address to finish setting up your opendesk account. It is where opendesk sends sign-in help and notification emails."},
		Button:    &mailer.Button{Label: "Confirm email", URL: link},
		Outro:     []string{"The link works for 48 hours."},
		Footer: mailer.Footer{
			Reason:  "You get this email because this address was used to sign up for opendesk. If that was not you, ignore it and nothing happens.",
			Contact: s.Contact,
		},
	})
}

// ResetPassword sends a password reset link.
func ResetPassword(s Site, link string) mailer.Mail {
	return s.build(mailer.Email{
		Subject:   "Reset your opendesk password",
		Preheader: "Choose a new password with the link inside. It works for one hour.",
		Heading:   "Reset your password",
		Intro:     []string{"Someone asked to reset the password for your opendesk account. Choose a new one with the button below."},
		Button:    &mailer.Button{Label: "Choose a new password", URL: link},
		Outro:     []string{"The link works for one hour and only once. Choosing a new password signs you out on your other devices."},
		Footer: mailer.Footer{
			Reason:  "You get this email because a password reset was asked for this address. If it was not you, ignore it: your password stays the same.",
			Contact: s.Contact,
		},
	})
}

// Invite asks someone to join a workspace.
type Invite struct {
	Inviter   string
	Workspace string
	Role      string
	Link      string
}

// Invitation builds the invite email.
func Invitation(s Site, in Invite) mailer.Mail {
	inviter := in.Inviter
	if inviter == "" {
		inviter = "A teammate"
	}
	role := roleLabel(in.Role)
	return s.build(mailer.Email{
		Subject:   "You are invited to " + in.Workspace + " on opendesk",
		Preheader: inviter + " invited you to answer customers together in " + in.Workspace + ".",
		Eyebrow:   in.Workspace,
		Heading:   "Join " + in.Workspace + " on opendesk",
		Intro:     []string{fmt.Sprintf("%s invited you to the %s workspace as %s. opendesk is where the team answers customers together: shared inboxes, live chat and a help center.", inviter, in.Workspace, article(role))},
		Facts:     []mailer.Fact{{Label: "Workspace", Value: in.Workspace}, {Label: "Role", Value: capitalize(role)}, {Label: "Invited by", Value: inviter}},
		Button:    &mailer.Button{Label: "Accept invitation", URL: in.Link},
		Outro:     []string{"The invitation works for 7 days. Sign in or create an account with this email address to accept it."},
		Footer: mailer.Footer{
			Reason:  "You get this email because " + inviter + " invited this address to " + in.Workspace + ". If you were not expecting it, ignore it.",
			Contact: s.Contact,
		},
	})
}

// TwoFactorDisabled tells a person an instance administrator turned off their two-factor
// authentication (contracts/admin.md), so they notice if they did not ask for it.
func TwoFactorDisabled(s Site) mailer.Mail {
	return s.build(mailer.Email{
		Subject:   "Two-factor authentication was turned off",
		Preheader: "An administrator turned it off for your opendesk account. Turn it back on in your security settings.",
		Heading:   "Two-factor authentication is off",
		Intro: []string{"An administrator of this opendesk instance turned off two-factor authentication for your account and signed you out everywhere.",
			"This usually happens after you lost your authenticator and asked for help. Sign in and turn it back on to keep your account protected."},
		Button: &mailer.Button{Label: "Open security settings", URL: s.App("/settings/security")},
		Footer: mailer.Footer{
			Reason:  "You get this email because the security settings of your opendesk account changed. If you did not ask for this, change your password and contact support.",
			Contact: s.Contact,
		},
	})
}

// PaymentFailed tells owners and admins a subscription payment failed.
func PaymentFailed(s Site, workspace string, workspaceID string) mailer.Mail {
	return s.build(mailer.Email{
		Subject:   "Payment failed for " + workspace,
		Preheader: "Update the payment method to keep Pro. Stripe retries over the next few days.",
		Eyebrow:   workspace,
		Heading:   "We could not collect your payment",
		Intro: []string{"The latest payment for the opendesk Pro subscription of " + workspace + " failed.",
			"Stripe retries over the next few days and the workspace stays on Pro meanwhile. Update the payment method to keep it that way."},
		Button: &mailer.Button{Label: "Update payment method", URL: s.App("/settings/billing", "ws", workspaceID)},
		Footer: mailer.Footer{
			Reason:  "You get this email because you are an owner or admin of " + workspace + ".",
			Contact: s.Contact,
		},
	})
}

func roleLabel(role string) string {
	switch role {
	case "owner", "admin", "agent", "viewer":
		return role
	}
	return "agent"
}

func article(word string) string {
	if word != "" && strings.ContainsRune("aeiou", rune(word[0])) {
		return "an " + word
	}
	return "a " + word
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
