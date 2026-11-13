package emails

import (
	"fmt"

	"github.com/useopendesk/opendesk/internal/mailer"
)

// ChatVerification asks a chat visitor of brand to verify the address they gave in its chat
// (contracts/api.md "Contact verification"). The chat goes on while they have not.
func ChatVerification(s Site, brand, link string, hours int) mailer.Mail {
	return s.build(mailer.Email{
		Subject:   "Verify your email for " + brand,
		Preheader: "Confirm the address you gave in the chat with " + brand + ".",
		Eyebrow:   brand,
		Heading:   "Verify your account",
		Intro: []string{
			"Thanks for chatting with " + brand + ". Your account is not verified yet: confirm that this address is yours, so the team knows the chat is from you.",
			"You can keep chatting in the meantime. Your chat shows as unverified until you do.",
		},
		Button: &mailer.Button{Label: "Verify account", URL: link},
		Outro:  []string{fmt.Sprintf("The link works for %d hours.", hours)},
		Footer: mailer.Footer{
			Reason:  "You get this email because this address was given in the chat on a website of " + brand + ". If that was not you, ignore it and nothing changes.",
			Contact: s.Contact,
		},
	})
}
