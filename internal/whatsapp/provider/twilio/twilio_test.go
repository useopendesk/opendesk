package twilio

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/useopendesk/opendesk/internal/whatsapp/provider"
)

func TestSignatureIsTwilios(t *testing.T) {
	// The example of Twilio's webhook security guide.
	form := url.Values{"CallSid": {"CA1234567890ABCDE"}, "Caller": {"+14158675310"}, "Digits": {"1234"}, "From": {"+14158675310"}, "To": {"+18005551212"}}
	got := Signature("12345", "https://mycompany.com/myapp.php?foo=1&bar=2", form)
	if got != "GvWf1cFY/Q7PnoempGyD5oXAezc=" {
		t.Fatalf("signature %q", got)
	}
	creds := map[string]string{"account_sid": "AC00000000000000000000000000000000", "auth_token": "12345"}
	r := provider.Request{URL: "https://mycompany.com/myapp.php?foo=1&bar=2", Header: http.Header{"X-Twilio-Signature": {got}}, Form: form}
	if err := (Adapter{}).Verify(r, creds); err != nil {
		t.Fatal(err)
	}
	form.Set("Digits", "9999")
	if err := (Adapter{}).Verify(r, creds); err == nil {
		t.Fatal("a changed form verified")
	}
}

func TestParse(t *testing.T) {
	a := Adapter{}
	ev, err := a.ParseMessage(url.Values{"MessageSid": {"SM1"}, "From": {"whatsapp:+4915123456789"}, "Body": {" hi "}, "NumMedia": {"2"},
		"MediaUrl0": {"https://api.twilio.com/m/1"}, "MediaContentType0": {"image/jpeg"}, "MediaUrl1": {"https://api.twilio.com/m/2"}, "ProfileName": {"Ana"}})
	if err != nil || ev.Number != "+4915123456789" || ev.Text != "hi" || len(ev.Media) != 2 || ev.Name != "Ana" || ev.ID != "SM1" {
		t.Fatalf("%+v %v", ev, err)
	}
	for name, f := range map[string]url.Values{
		"sms":      {"MessageSid": {"SM1"}, "From": {"+4915123456789"}, "Body": {"x"}},
		"no words": {"MessageSid": {"SM1"}, "From": {"whatsapp:+4915123456789"}, "NumMedia": {"0"}},
		"bad from": {"MessageSid": {"SM1"}, "From": {"whatsapp:abc"}, "Body": {"x"}},
		"no sid":   {"From": {"whatsapp:+4915123456789"}, "Body": {"x"}},
	} {
		if _, err := a.ParseMessage(f); err != provider.ErrIgnored {
			t.Errorf("%s: %v", name, err)
		}
	}
	st, err := a.ParseStatus(url.Values{"MessageSid": {"SM9"}, "MessageStatus": {"undelivered"}, "To": {"whatsapp:+4915123456789"}, "ErrorCode": {"63024"}})
	if err != nil || st.Status != provider.StatusFailed || st.Code != "63024" || st.ID != "SM9:failed" {
		t.Fatalf("%+v %v", st, err)
	}
	if _, err := a.ParseStatus(url.Values{"MessageSid": {"SM9"}, "MessageStatus": {"queued"}, "To": {"whatsapp:+4915123456789"}}); err != provider.ErrIgnored {
		t.Fatalf("queued: %v", err)
	}
}

func TestMediaHostsAreTwilios(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://api.twilio.com/2010-04-01/Accounts/AC/Messages/MM/Media/ME": true,
		"https://media.twiliocdn.com/x":                                      true,
		"https://s3-external-1.amazonaws.com/x":                              true,
		"http://api.twilio.com/x":                                            false,
		"https://evil.example.com/x":                                         false,
		"https://api.twilio.com.evil.example/x":                              false,
		"https://169.254.169.254/latest":                                     false,
	} {
		u, _ := url.Parse(raw)
		if got := mediaHost(u); got != want {
			t.Errorf("%s: %v", raw, got)
		}
	}
}
