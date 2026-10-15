package email

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Network limits for every connection to a mail server.
const (
	dialTimeout    = 15 * time.Second
	sessionTimeout = 4 * time.Minute // absolute bound on one IMAP/SMTP session
	httpTimeout    = 60 * time.Second
)

var errBlockedAddress = errors.New("that mail server address is not allowed")

// netGuard dials user-supplied mail hosts. Outside development it refuses private,
// loopback, link-local and other non-public targets, checked against the address
// actually connected to (so a DNS answer that changes between checks cannot bypass it).
type netGuard struct {
	allowPrivate bool
	// rootCAs overrides the system pool (tests only).
	rootCAs *x509.CertPool
}

var blockedPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
		"192.0.0.0/24", "192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24",
		"224.0.0.0/4", "240.0.0.0/4", "255.255.255.255/32",
		"::/128", "::1/128", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001:db8::/32", "fc00::/7", "fe80::/10", "ff00::/8",
	} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// publicAddr reports whether ip is a public unicast address.
func publicAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

func (g *netGuard) control(_, address string, _ syscall.RawConn) error {
	if g.allowPrivate {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errBlockedAddress
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !publicAddr(ip) {
		return errBlockedAddress
	}
	return nil
}

// dial connects to host:port with the guard and a connect timeout.
func (g *netGuard) dial(ctx context.Context, host string, port int) (net.Conn, error) {
	d := &net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second, Control: g.control}
	return d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
}

// tlsConfig verifies the server certificate for host with TLS 1.2 or newer.
func (g *netGuard) tlsConfig(host string) *tls.Config {
	return &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, RootCAs: g.rootCAs}
}

// validHost checks a user-supplied mail host name at save time. The dial-time guard
// is the authority; this only refuses values that can never be right.
func (g *netGuard) validHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" || len(host) > 253 || strings.ContainsAny(host, "/:@ \t\r\n?#[]") {
		return errors.New("enter a host name like imap.example.com")
	}
	if g.allowPrivate {
		return nil
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") || strings.HasSuffix(lower, ".local") || strings.HasSuffix(lower, ".internal") {
		return errBlockedAddress
	}
	if ip, err := netip.ParseAddr(host); err == nil && !publicAddr(ip) {
		return errBlockedAddress
	}
	return nil
}

// withDeadline bounds a whole session: one absolute deadline, and it closes when ctx ends.
func withDeadline(ctx context.Context, c net.Conn, d time.Duration) (net.Conn, func()) {
	deadline := time.Now().Add(d)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = c.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	return c, func() { stop() }
}

// httpClient fetches from hosts named by user input (mail autoconfig files): every
// dial goes through the guard, no proxy, HTTPS only, at most three redirects, each
// to an HTTPS URL on the default port.
func (g *netGuard) httpClient() *http.Client {
	d := &net.Dialer{Timeout: 5 * time.Second, Control: g.control}
	tr := &http.Transport{
		DialContext:           d.DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: g.rootCAs},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		MaxIdleConns:          8,
		IdleConnTimeout:       30 * time.Second,
	}
	return &http.Client{Transport: tr, Timeout: 8 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "https" || req.URL.User != nil || (req.URL.Port() != "" && req.URL.Port() != "443") {
			return errors.New("redirect refused")
		}
		return nil
	}}
}
