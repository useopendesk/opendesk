// Package helpdomains is help center custom domains (contracts/teams.md "Help center
// addresses"): claiming a hostname, checking its DNS records, provisioning its
// certificate on Cloudflare for SaaS when the instance uses it, and releasing it.
package helpdomains

import (
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"

	"github.com/useopendesk/opendesk/internal/config"
	"github.com/useopendesk/opendesk/internal/httpx"
)

var labelsRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)

func invalid(detail string) error {
	return httpx.E(http.StatusUnprocessableEntity, "hostname_invalid", detail)
}

// CleanHostname turns what an admin typed (a hostname, or a URL of one) into the lowercase
// ASCII hostname a custom domain is stored as, refusing what cannot be one: an IP address,
// a registrable domain itself (a CNAME needs a subdomain), and this instance's own hosts.
func CleanHostname(cfg *config.Config, raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil || u.Host == "" {
			return "", invalid("Enter a hostname such as help.example.com.")
		}
		s = u.Host
	} else if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSuffix(s, ".")
	if strings.Contains(s, ":") {
		return "", invalid("Enter the hostname without a port.")
	}
	ascii, err := idna.Lookup.ToASCII(s)
	if err != nil || len(ascii) > 253 || !labelsRe.MatchString(ascii) {
		return "", invalid("Enter a hostname such as help.example.com.")
	}
	if net.ParseIP(ascii) != nil {
		return "", invalid("Use a hostname, not an IP address.")
	}
	suffix, icann := publicsuffix.PublicSuffix(ascii)
	if cfg.IsProduction() && !icann && !strings.Contains(suffix, ".") {
		return "", invalid("Use a hostname on a public domain.")
	}
	if apex, err := publicsuffix.EffectiveTLDPlusOne(ascii); err != nil || apex == ascii {
		return "", invalid("Use a subdomain such as help." + strings.TrimPrefix(ascii, "www.") + ": a domain itself cannot point here with a CNAME.")
	}
	if own(cfg, ascii) {
		return "", invalid("That hostname belongs to this service.")
	}
	return ascii, nil
}

// own reports whether a hostname is one this instance already answers on.
func own(cfg *config.Config, host string) bool {
	if cfg.HelpDomain != "" && (host == cfg.HelpDomain || strings.HasSuffix(host, "."+cfg.HelpDomain)) {
		return true
	}
	if host == cfg.HelpCNAMETarget {
		return true
	}
	for _, raw := range []string{cfg.AppURL, cfg.APIURL, cfg.GatewayURL, cfg.SiteURL} {
		if u, err := url.Parse(raw); err == nil && strings.ToLower(u.Hostname()) == host {
			return true
		}
	}
	return false
}
