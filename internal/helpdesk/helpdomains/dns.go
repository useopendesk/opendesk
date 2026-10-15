package helpdomains

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/useopendesk/opendesk/internal/helpdesk/brands"
)

// Resolver is the DNS a check reads (net.Resolver; tests use their own).
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupCNAME(ctx context.Context, host string) (string, error)
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

const lookupTimeout = 5 * time.Second

// txtFound reports whether the hostname's verification record carries the claim's token.
func txtFound(ctx context.Context, r Resolver, hostname, token string) bool {
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	values, err := r.LookupTXT(ctx, brands.VerifyName(hostname))
	if err != nil {
		return false
	}
	want := brands.VerifyValue(token)
	for _, v := range values {
		if strings.TrimSpace(strings.Trim(v, `"`)) == want {
			return true
		}
	}
	return false
}

// pointsHere reports whether the hostname is a CNAME of the target, or resolves to the
// target's addresses (a flattened record, or a proxy in front of both).
func pointsHere(ctx context.Context, r Resolver, hostname, target string) bool {
	if target == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	if c, err := r.LookupCNAME(ctx, hostname); err == nil && strings.EqualFold(strings.TrimSuffix(c, "."), target) {
		return true
	}
	have, err := r.LookupIPAddr(ctx, hostname)
	if err != nil || len(have) == 0 {
		return false
	}
	want, err := r.LookupIPAddr(ctx, target)
	if err != nil {
		return false
	}
	for _, a := range have {
		for _, b := range want {
			if a.IP.Equal(b.IP) {
				return true
			}
		}
	}
	return false
}
