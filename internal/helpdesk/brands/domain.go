package brands

import (
	"time"

	"github.com/useopendesk/opendesk/internal/config"
)

// HelpDomain is a help center's custom domain (contracts/teams.md "Help center addresses").
type HelpDomain struct {
	Hostname   string             `json:"hostname"`
	Status     string             `json:"status" enum:"pending,active,failed"`
	Problem    string             `json:"problem" enum:",txt_missing,cname_missing,domain_taken,certificate_pending,certificate_failed,provider_error"`
	Records    []HelpDomainRecord `json:"records" doc:"What to add at the DNS provider"`
	CheckedAt  *time.Time         `json:"checked_at"`
	VerifiedAt *time.Time         `json:"verified_at" doc:"Both records were found"`
	CreatedAt  time.Time          `json:"created_at"`
}

// HelpDomainRecord is one DNS record a custom domain needs.
type HelpDomainRecord struct {
	Type  string `json:"type" enum:"TXT,CNAME"`
	Name  string `json:"name"`
	Value string `json:"value"`
	OK    bool   `json:"ok" doc:"Found at the last check"`
}

// VerifyName is the TXT record that proves a hostname is the customer's.
func VerifyName(hostname string) string { return "_opendesk-verify." + hostname }

// VerifyValue is the TXT record's value for a claim's token.
func VerifyValue(token string) string { return "opendesk-verify=" + token }

// Records lists the DNS records a custom domain needs, with whether each was found.
func Records(cfg *config.Config, hostname, token string, txtOK, cnameOK bool) []HelpDomainRecord {
	return []HelpDomainRecord{
		{Type: "TXT", Name: VerifyName(hostname), Value: VerifyValue(token), OK: txtOK},
		{Type: "CNAME", Name: hostname, Value: cfg.HelpCNAMETarget, OK: cnameOK},
	}
}

// domainRow is the help_domains columns brandSelect reads; all NULL without a domain.
type domainRow struct {
	hostname, token, status, problem *string
	txtOK, cnameOK                   *bool
	checkedAt, verifiedAt, createdAt *time.Time
}

func (r domainRow) domain(cfg *config.Config) *HelpDomain {
	if r.hostname == nil {
		return nil
	}
	verified := r.verifiedAt != nil
	return &HelpDomain{Hostname: *r.hostname, Status: *r.status, Problem: *r.problem,
		// A verified domain's TXT record is no longer read.
		Records:   Records(cfg, *r.hostname, *r.token, *r.txtOK || verified, *r.cnameOK),
		CheckedAt: r.checkedAt, VerifiedAt: r.verifiedAt, CreatedAt: *r.createdAt}
}

func (d *HelpDomain) activeHost() string {
	if d == nil || d.Status != "active" {
		return ""
	}
	return d.Hostname
}
