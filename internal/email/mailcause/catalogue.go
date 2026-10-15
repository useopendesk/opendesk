// Package mailcause turns a failed mailbox connection check into a stable cause
// key with a short title and a concrete fix (contracts/api.md "Mailboxes"), so the
// person connecting a mailbox learns what to do about it, not only that it failed.
package mailcause

// Cause is one kind of connect failure and what fixes it.
type Cause struct {
	Key   string `json:"code"`
	Title string `json:"title"`
	Fix   string `json:"fix"`
	// Retryable: the same settings can succeed on a later attempt or with a new password.
	Retryable bool `json:"retryable"`
}

// Cause keys.
const (
	GoogleAppPasswordRequired   = "google_app_password_required"
	GoogleBadCredentials        = "google_bad_credentials"
	GoogleIMAPDisabled          = "google_imap_disabled"
	GoogleWebLoginRequired      = "google_web_login_required"
	MicrosoftSMTPAuthDisabled   = "microsoft_smtp_auth_disabled"
	MicrosoftSecurityDefaults   = "microsoft_security_defaults"
	MicrosoftConditionalAccess  = "microsoft_conditional_access"
	MicrosoftBasicAuthDisabled  = "microsoft_basic_auth_disabled"
	MicrosoftBadCredentials     = "microsoft_bad_credentials"
	YahooAppPasswordRequired    = "yahoo_app_password_required"
	AOLAppPasswordRequired      = "aol_app_password_required"
	ICloudAppPasswordRequired   = "icloud_app_password_required"
	ZohoAppPasswordRequired     = "zoho_app_password_required"
	FastmailAppPasswordRequired = "fastmail_app_password_required"
	YandexAppPasswordRequired   = "yandex_app_password_required"
	IMAPDisabled                = "imap_disabled"
	AccountLocked               = "account_locked"
	TooManyLogins               = "too_many_logins"
	AuthUnavailable             = "auth_unavailable"
	AuthRefused                 = "auth_refused"
	HostNotFound                = "host_not_found"
	HostUnreachable             = "host_unreachable"
	PortBlocked                 = "port_blocked"
	TLSFailed                   = "tls_failed"
	CleartextRefused            = "cleartext_refused"
	Timeout                     = "timeout"
	Temporary                   = "temporary"
	ServerDeclined              = "server_declined"
	HostNotAllowed              = "host_not_allowed"
)

// Reasons a connection check leg failed, set by the IMAP and SMTP clients.
const (
	ReasonAuthRefused     = "auth_refused"
	ReasonTLS             = "tls"
	ReasonUnreachable     = "unreachable"
	ReasonCleartext       = "cleartext"
	ReasonProtocol        = "protocol"
	ReasonTemporary       = "temporary"
	ReasonTimeout         = "timeout"
	ReasonBlocked         = "blocked"
	ReasonAuthUnavailable = "auth_unavailable"
)

var catalogue = []Cause{
	{GoogleAppPasswordRequired, "Google needs an app password",
		"Google does not accept the account password over IMAP and SMTP. Turn on 2-Step Verification, create an app password at https://myaccount.google.com/apppasswords while signed in to this address, and paste the 16 letters it shows. On Google Workspace, your admin must allow 2-Step Verification and app passwords.", true},
	{GoogleBadCredentials, "Google did not accept the app password",
		"Google refused this address and app password. Create a new app password at https://myaccount.google.com/apppasswords while signed in to this exact address (not an alias or a group) and use it. A deleted app password stops working at once.", true},
	{GoogleIMAPDisabled, "IMAP is turned off in Google",
		"Turn IMAP on for this mailbox: in Gmail open Settings, See all settings, Forwarding and POP/IMAP, and enable IMAP. On Google Workspace an admin must also allow it in the Admin console under Apps, Google Workspace, Gmail, End User Access, POP and IMAP access.", true},
	{GoogleWebLoginRequired, "Google wants a browser sign-in first",
		"Google blocked this sign-in as unusual. Sign in to this mailbox once at https://mail.google.com in a browser, confirm any security prompt, then try again.", true},
	{MicrosoftSMTPAuthDisabled, "SMTP sign-in is turned off in Microsoft 365",
		"Microsoft 365 refused password sign-in over SMTP AUTH for this mailbox. Microsoft is phasing out password sign-in for SMTP AUTH, so send replies through opendesk instead. While Microsoft still allows it, an admin can turn on Authenticated SMTP for this mailbox in the Microsoft 365 admin center (Active users, Mail, Manage email apps).", true},
	{MicrosoftSecurityDefaults, "Microsoft security defaults block passwords",
		"Microsoft Entra security defaults block password sign-in over SMTP and IMAP for this organization. Connect the mailbox with Microsoft sign-in, or forward its mail into opendesk and send replies through opendesk. Microsoft is phasing out password sign-in for SMTP AUTH, so sending through opendesk is the lasting fix.", true},
	{MicrosoftConditionalAccess, "A Microsoft sign-in policy blocked this",
		"A Conditional Access policy in your Microsoft organization refused this password sign-in. Connect the mailbox with Microsoft sign-in, or forward its mail into opendesk and send replies through opendesk. Microsoft is phasing out password sign-in for SMTP AUTH, so an exception for it may not last.", true},
	{MicrosoftBasicAuthDisabled, "Microsoft needs Microsoft sign-in",
		"Exchange Online no longer accepts a password over IMAP. Connect the mailbox with Microsoft sign-in, or forward its mail into opendesk and send replies through opendesk. Microsoft is also phasing out password sign-in for SMTP AUTH, so its SMTP server is not a lasting way to send.", false},
	{MicrosoftBadCredentials, "Microsoft did not accept the password",
		"Microsoft refused this address and password. Check both, including the sign-in address if it differs from the mailbox address. A password does not work on an account with multi-factor sign-in. Microsoft is phasing out password sign-in for SMTP AUTH: if it is refused, send replies through opendesk instead.", true},
	{YahooAppPasswordRequired, "Yahoo needs an app password",
		"Yahoo only accepts an app password over IMAP and SMTP. Sign in to Yahoo, open Account security at https://login.yahoo.com/account/security, choose Generate app password, and use the password it shows.", true},
	{AOLAppPasswordRequired, "AOL needs an app password",
		"AOL only accepts an app password over IMAP and SMTP. Sign in to AOL, open Account security at https://login.aol.com/account/security, choose Generate app password, and use the password it shows.", true},
	{ICloudAppPasswordRequired, "iCloud needs an app-specific password",
		"iCloud Mail only accepts an app-specific password. Sign in at https://account.apple.com, open Sign-In and Security, App-Specific Passwords, create one, and use it. The username is the full iCloud address, not an alias.", true},
	{ZohoAppPasswordRequired, "Zoho refused the password",
		"Zoho refused this sign-in. If two-factor authentication is on, create an app password in Zoho Accounts under Security, App Passwords, and use it. Also check that IMAP access is on in Zoho Mail settings under Mail Accounts, IMAP.", true},
	{FastmailAppPasswordRequired, "Fastmail needs an app password",
		"Fastmail only accepts an app password over IMAP and SMTP. In Fastmail open Settings, Privacy and Security, Manage app passwords, create one with IMAP and SMTP access, and use it.", true},
	{YandexAppPasswordRequired, "Yandex needs an app password",
		"Yandex only accepts an app password over IMAP and SMTP. Create one for Mail at https://id.yandex.com/security/app-passwords, and check that IMAP is allowed in Yandex Mail settings under Email clients.", true},
	{IMAPDisabled, "IMAP is turned off for this mailbox",
		"The server says IMAP access is off for this account. Turn IMAP on in the provider's mail settings, or ask your email admin to, then try again.", true},
	{AccountLocked, "The account is locked or disabled",
		"The provider says this account is locked, suspended or disabled. Sign in to it in a browser to see why and restore access, or ask your email admin, then try again.", false},
	{TooManyLogins, "Too many sign-in attempts",
		"The provider is limiting sign-ins for this account. Wait 15 to 60 minutes before trying again, and avoid retrying a password that keeps failing.", true},
	{AuthUnavailable, "The server does not take a password here",
		"The server did not offer password sign-in on this port. Check the host and port: use the provider's submission server (usually port 587 or 465), not the MX server or port 25.", false},
	{AuthRefused, "The server refused the sign-in",
		"The server refused this username and password. Check both, and whether the provider needs an app password or a username other than the email address.", true},
	{HostNotFound, "The server name does not exist",
		"The server name could not be found in DNS. Check the spelling of the IMAP and SMTP server.", false},
	{HostUnreachable, "The server could not be reached",
		"No connection could be made to the server. Check the server and port, and that the server accepts connections from the internet.", false},
	{PortBlocked, "The mail port did not answer",
		"The SMTP port did not answer, which usually means it is blocked on the way. Try port 587 with STARTTLS instead of 465 or 25, or check the provider's documented SMTP port.", false},
	{TLSFailed, "The secure connection failed",
		"The server did not complete a secure connection. Check the security setting for the port: 465 and 993 use TLS, 587 and 143 use STARTTLS. Also check that the server name matches its certificate.", false},
	{CleartextRefused, "Encryption is required",
		"The server wants an encrypted connection first. Use TLS on 465 or 993, or STARTTLS on 587 or 143.", false},
	{Timeout, "The server did not answer in time",
		"The server did not answer before the time ran out. It may be slow or busy; try again later, and check the server and port if it keeps happening.", true},
	{Temporary, "The server asked to try again later",
		"The server declined the sign-in for now. Try again in a few minutes.", true},
	{ServerDeclined, "The server did not accept the connection",
		"The server answered but did not accept the sign-in. Check the server, port, security setting and password, then try again.", true},
	{HostNotAllowed, "The server must be a public address",
		"opendesk connects only to mail servers on the public internet. Use the provider's public server name.", false},
}

var byKey = func() map[string]Cause {
	m := make(map[string]Cause, len(catalogue))
	for _, c := range catalogue {
		m[c.Key] = c
	}
	return m
}()

// Lookup returns the catalogue entry for key.
func Lookup(key string) (Cause, bool) {
	c, ok := byKey[key]
	return c, ok
}

// Keys lists every cause key in catalogue order.
func Keys() []string {
	out := make([]string, len(catalogue))
	for i, c := range catalogue {
		out[i] = c.Key
	}
	return out
}

// Generic reports whether key is a catch-all a finer classifier may refine.
func Generic(key string) bool {
	return key == AuthRefused || key == ServerDeclined
}

func must(key string) Cause {
	c, ok := byKey[key]
	if !ok {
		return byKey[ServerDeclined]
	}
	return c
}
