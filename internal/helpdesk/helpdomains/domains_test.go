package helpdomains_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/helpdesk/brands"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
	"github.com/useopendesk/opendesk/internal/helpdesk/helpdomains"
)

// fakeDNS answers from maps; anything else does not exist.
type fakeDNS struct {
	mu    sync.Mutex
	txt   map[string][]string
	cname map[string]string
	ips   map[string][]string
}

func newDNS() *fakeDNS {
	return &fakeDNS{txt: map[string][]string{}, cname: map[string]string{}, ips: map[string][]string{}}
}

var errNoHost = errors.New("no such host")

func (f *fakeDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.txt[name]; ok {
		return v, nil
	}
	return nil, errNoHost
}

func (f *fakeDNS) LookupCNAME(_ context.Context, host string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.cname[host]; ok {
		return v + ".", nil
	}
	if _, ok := f.ips[host]; ok {
		return host + ".", nil
	}
	return "", errNoHost
}

func (f *fakeDNS) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.cname[host]; ok {
		host = c
	}
	var out []net.IPAddr
	for _, ip := range f.ips[host] {
		out = append(out, net.IPAddr{IP: net.ParseIP(ip)})
	}
	if len(out) == 0 {
		return nil, errNoHost
	}
	return out, nil
}

func (f *fakeDNS) set(fn func(f *fakeDNS)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

// fakeCloudflare is a zone's custom hostnames.
type fakeCloudflare struct {
	mu      sync.Mutex
	hosts   map[string]map[string]any
	ready   bool // new and existing hostnames report active
	deleted []string
}

func (f *fakeCloudflare) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer cf-test-token" {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":10000,"message":"Authentication error"}]}`))
		return
	}
	ok := func(v any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "errors": []any{}, "result": v,
			"result_info": map[string]any{"page": 1, "total_pages": 1}})
	}
	status := func(h map[string]any) map[string]any {
		if f.ready {
			h["status"], h["ssl"] = "active", map[string]any{"status": "active"}
		}
		return h
	}
	id := strings.TrimPrefix(r.URL.Path, "/zones/zone-1/custom_hostnames")
	id = strings.TrimPrefix(id, "/")
	switch {
	case r.Method == http.MethodPost && id == "":
		var body struct{ Hostname string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, h := range f.hosts {
			if h["hostname"] == body.Hostname {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":1406,"message":"Duplicate custom hostname found."}]}`))
				return
			}
		}
		h := map[string]any{"id": "ch-" + body.Hostname, "hostname": body.Hostname, "status": "pending",
			"ssl": map[string]any{"status": "pending_validation"}, "created_at": time.Now().Add(-2 * time.Hour).Format(time.RFC3339)}
		f.hosts[h["id"].(string)] = h
		ok(status(h))
	case r.Method == http.MethodGet && id == "":
		list := []any{}
		for _, h := range f.hosts {
			if q := r.URL.Query().Get("hostname"); q == "" || h["hostname"] == q {
				list = append(list, status(h))
			}
		}
		ok(list)
	case r.Method == http.MethodGet || r.Method == http.MethodPatch:
		h, found := f.hosts[id]
		if !found {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":1436,"message":"not found"}]}`))
			return
		}
		ok(status(h))
	case r.Method == http.MethodDelete:
		delete(f.hosts, id)
		f.deleted = append(f.deleted, id)
		ok(map[string]any{"id": id})
	}
}

type env struct {
	*hdtest.Env
	dns *fakeDNS
	cf  *fakeCloudflare
	svc *helpdomains.Service
}

func newEnv(t *testing.T, withCloudflare bool) *env {
	dns := newDNS()
	var client *helpdomains.Cloudflare
	fake := &fakeCloudflare{hosts: map[string]map[string]any{}}
	if withCloudflare {
		srv := httptest.NewServer(fake)
		t.Cleanup(srv.Close)
		client = helpdomains.NewCloudflare("zone-1", "cf-test-token")
		client.BaseURL = srv.URL
	}
	e := &env{dns: dns, cf: fake}
	e.Env = hdtest.New(t, brands.Register, func(d *deps.Deps, api huma.API, _ chi.Router) {
		e.svc = helpdomains.NewService(d, dns, client)
		helpdomains.RegisterService(d, api, e.svc)
	})
	cfg := e.D.Cfg
	cfg.HelpCNAMETarget = "custom.help.test"
	dns.set(func(f *fakeDNS) { f.ips["custom.help.test"] = []string{"203.0.113.7"} })
	t.Cleanup(func() { cfg.HelpCNAMETarget = "" })
	return e
}

type domainOut struct {
	Hostname string `json:"hostname"`
	Status   string `json:"status"`
	Problem  string `json:"problem"`
	Records  []struct {
		Type, Name, Value string
		OK                bool
	} `json:"records"`
	VerifiedAt *time.Time `json:"verified_at"`
}

type brandOut struct {
	ID         string `json:"id"`
	Slug       string `json:"slug"`
	HelpCenter struct {
		URL        string     `json:"url"`
		DefaultURL string     `json:"default_url"`
		Domain     *domainOut `json:"domain"`
	} `json:"help_center"`
}

func host() string { return "help-" + strings.ToLower(crypto.RandomString(8)) + ".example.com" }

func (e *env) defaultBrand(ws hdtest.WS) string {
	var id uuid.UUID
	e.Scalar(`SELECT id FROM brands WHERE workspace_id = $1 AND is_default`, &id, ws.ID)
	return id.String()
}

func (e *env) set(t *testing.T, ws hdtest.WS, brandID, hostname string) brandOut {
	var b brandOut
	e.Do("PUT", "/v1/brands/"+brandID+"/help-domain", ws.Admin.As(ws), map[string]any{"hostname": hostname}).Want(t, 200).JSON(t, &b)
	return b
}

func (e *env) check(t *testing.T, ws hdtest.WS, brandID string) brandOut {
	var b brandOut
	e.Do("POST", "/v1/brands/"+brandID+"/help-domain/check", ws.Admin.As(ws), nil).Want(t, 200).JSON(t, &b)
	return b
}

func TestHelpDomainVerifiesWithBothRecords(t *testing.T) {
	e := newEnv(t, false)
	ws := e.NewWorkspace()
	brandID := e.defaultBrand(ws)
	h := host()

	e.Do("PUT", "/v1/brands/"+brandID+"/help-domain", ws.Viewer.As(ws), map[string]any{"hostname": h}).Want(t, 403)
	e.Do("PUT", "/v1/brands/"+brandID+"/help-domain", ws.Admin.As(ws), map[string]any{"hostname": "example.com"}).Want(t, 422)

	_, seq := e.Events(ws.ID, 0)
	b := e.set(t, ws, brandID, "https://"+strings.ToUpper(h)+"/")
	d := b.HelpCenter.Domain
	if d == nil || d.Hostname != h || d.Status != "pending" || d.Problem != "txt_missing" || len(d.Records) != 2 {
		t.Fatalf("new domain %+v", d)
	}
	if b.HelpCenter.URL != b.HelpCenter.DefaultURL || b.HelpCenter.URL != "http://app.test/help/"+b.Slug {
		t.Fatalf("a pending domain is not the address yet: %+v", b.HelpCenter)
	}
	txt, cname := d.Records[0], d.Records[1]
	if txt.Type != "TXT" || txt.Name != "_opendesk-verify."+h || !strings.HasPrefix(txt.Value, "opendesk-verify=") ||
		cname.Type != "CNAME" || cname.Name != h || cname.Value != "custom.help.test" {
		t.Fatalf("records %+v", d.Records)
	}
	if types, _ := e.Events(ws.ID, seq); !contains(types, "brand.updated") {
		t.Fatalf("events %v", types)
	}
	var audited int
	e.Scalar(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'brand.help_domain_set'`, &audited, ws.ID)
	if audited != 1 {
		t.Fatalf("audit rows %d", audited)
	}
	// Setting the same hostname again changes nothing, the token included.
	if again := e.set(t, ws, brandID, h); again.HelpCenter.Domain.Records[0].Value != txt.Value {
		t.Fatal("the token changed")
	}

	// The CNAME alone proves nothing.
	e.dns.set(func(f *fakeDNS) { f.cname[h] = "custom.help.test" })
	if b = e.check(t, ws, brandID); b.HelpCenter.Domain.Status != "pending" || b.HelpCenter.Domain.Problem != "txt_missing" ||
		!b.HelpCenter.Domain.Records[1].OK {
		t.Fatalf("cname only %+v", b.HelpCenter.Domain)
	}
	e.dns.set(func(f *fakeDNS) { f.txt["_opendesk-verify."+h] = []string{"unrelated", txt.Value} })
	b = e.check(t, ws, brandID)
	if d := b.HelpCenter.Domain; d.Status != "active" || d.Problem != "" || d.VerifiedAt == nil || b.HelpCenter.URL != "https://"+h {
		t.Fatalf("verified %+v url %q", d, b.HelpCenter.URL)
	}

	// Another workspace cannot take a verified hostname.
	other := e.NewWorkspace()
	e.Do("PUT", "/v1/brands/"+e.defaultBrand(other)+"/help-domain", other.Admin.As(other), map[string]any{"hostname": h}).Want(t, 409)

	// Removing the TXT record later does not matter; a CNAME pointing away shows.
	e.dns.set(func(f *fakeDNS) {
		delete(f.txt, "_opendesk-verify."+h)
		delete(f.cname, h)
	})
	if b = e.check(t, ws, brandID); b.HelpCenter.Domain.Status != "active" || b.HelpCenter.Domain.Problem != "cname_missing" {
		t.Fatalf("moved away %+v", b.HelpCenter.Domain)
	}

	e.Do("DELETE", "/v1/brands/"+brandID+"/help-domain", ws.Admin.As(ws), nil).Want(t, 204)
	var got brandOut
	e.Do("GET", "/v1/brands/"+brandID, ws.Admin.As(ws), nil).Want(t, 200).JSON(t, &got)
	if got.HelpCenter.Domain != nil || got.HelpCenter.URL != got.HelpCenter.DefaultURL {
		t.Fatalf("after removal %+v", got.HelpCenter)
	}
	// The hostname is free again, with a new token.
	if b := e.set(t, other, e.defaultBrand(other), h); b.HelpCenter.Domain.Records[0].Value == txt.Value {
		t.Fatal("a new claim reused the old token")
	}
}

func TestHelpDomainAcceptsFlattenedRecords(t *testing.T) {
	e := newEnv(t, false)
	ws := e.NewWorkspace()
	brandID := e.defaultBrand(ws)
	h := host()
	b := e.set(t, ws, brandID, h)
	e.dns.set(func(f *fakeDNS) {
		f.ips[h] = []string{"203.0.113.7"}
		f.txt["_opendesk-verify."+h] = []string{b.HelpCenter.Domain.Records[0].Value}
	})
	if b = e.check(t, ws, brandID); b.HelpCenter.Domain.Status != "active" {
		t.Fatalf("same addresses %+v", b.HelpCenter.Domain)
	}
}

func TestHelpDomainOnCloudflare(t *testing.T) {
	e := newEnv(t, true)
	ws := e.NewWorkspace()
	brandID := e.defaultBrand(ws)
	h := host()
	b := e.set(t, ws, brandID, h)
	e.dns.set(func(f *fakeDNS) {
		f.cname[h] = "custom.help.test"
		f.txt["_opendesk-verify."+h] = []string{b.HelpCenter.Domain.Records[0].Value}
	})
	if b = e.check(t, ws, brandID); b.HelpCenter.Domain.Status != "pending" || b.HelpCenter.Domain.Problem != "certificate_pending" ||
		b.HelpCenter.Domain.VerifiedAt == nil {
		t.Fatalf("certificate pending %+v", b.HelpCenter.Domain)
	}
	var provider string
	e.Scalar(`SELECT provider_id FROM help_domains WHERE hostname = $1`, &provider, h)
	if provider != "ch-"+h {
		t.Fatalf("provider id %q", provider)
	}
	e.cf.mu.Lock()
	e.cf.ready = true
	e.cf.mu.Unlock()
	// The scheduled check runs the same way.
	var id uuid.UUID
	e.Scalar(`SELECT id FROM help_domains WHERE hostname = $1`, &id, h)
	if err := e.svc.Check(e.Ctx, id, false); err != nil {
		t.Fatal(err)
	}
	var got brandOut
	e.Do("GET", "/v1/brands/"+brandID, ws.Admin.As(ws), nil).Want(t, 200).JSON(t, &got)
	if got.HelpCenter.Domain.Status != "active" || got.HelpCenter.URL != "https://"+h {
		t.Fatalf("active %+v", got.HelpCenter)
	}

	// Removing it queues the release; releasing deletes the hostname at Cloudflare.
	e.Do("DELETE", "/v1/brands/"+brandID+"/help-domain", ws.Admin.As(ws), nil).Want(t, 204)
	var queued int
	e.Scalar(`DELETE FROM river_job WHERE kind = 'help_domain.release' AND args->>'provider_id' = $1 RETURNING 1`, &queued, provider)
	if err := e.svc.Release(e.Ctx, provider); err != nil {
		t.Fatal(err)
	}
	e.cf.mu.Lock()
	defer e.cf.mu.Unlock()
	if len(e.cf.deleted) != 1 || e.cf.deleted[0] != provider {
		t.Fatalf("deleted %v", e.cf.deleted)
	}
}

func TestHelpDomainReconcileReleasesStrays(t *testing.T) {
	e := newEnv(t, true)
	e.cf.mu.Lock()
	e.cf.hosts["ch-stray"] = map[string]any{"id": "ch-stray", "hostname": "stray.example.com", "status": "active",
		"ssl": map[string]any{"status": "active"}, "created_at": time.Now().Add(-48 * time.Hour).Format(time.RFC3339)}
	e.cf.hosts["ch-new"] = map[string]any{"id": "ch-new", "hostname": "new.example.com", "status": "pending",
		"ssl": map[string]any{"status": "pending_validation"}, "created_at": time.Now().Format(time.RFC3339)}
	e.cf.mu.Unlock()
	if err := e.svc.Reconcile(e.Ctx); err != nil {
		t.Fatal(err)
	}
	e.cf.mu.Lock()
	defer e.cf.mu.Unlock()
	if _, ok := e.cf.hosts["ch-stray"]; ok {
		t.Fatal("a stray hostname was kept")
	}
	if _, ok := e.cf.hosts["ch-new"]; !ok {
		t.Fatal("a hostname created in the last hour was released")
	}
}

func TestHelpDomainGivesUpAfterThreeDays(t *testing.T) {
	e := newEnv(t, false)
	ws := e.NewWorkspace()
	brandID := e.defaultBrand(ws)
	h := host()
	e.set(t, ws, brandID, h)
	var id uuid.UUID
	e.Scalar(`SELECT id FROM help_domains WHERE hostname = $1`, &id, h)
	e.Exec(`UPDATE help_domains SET pending_since = now() - interval '73 hours' WHERE id = $1`, id)
	if err := e.svc.Check(e.Ctx, id, false); err != nil {
		t.Fatal(err)
	}
	var status string
	e.Scalar(`SELECT status FROM help_domains WHERE id = $1`, &status, id)
	if status != "failed" {
		t.Fatalf("status %q", status)
	}
	// A manual check starts over.
	if b := e.check(t, ws, brandID); b.HelpCenter.Domain.Status != "pending" {
		t.Fatalf("after a manual check %+v", b.HelpCenter.Domain)
	}
}

func TestDeletingABrandReleasesItsDomain(t *testing.T) {
	e := newEnv(t, true)
	ws := e.NewWorkspace()
	var b brandOut
	e.Do("POST", "/v1/brands", ws.Admin.As(ws), map[string]any{"name": "Second " + crypto.RandomString(5)}).Want(t, 201).JSON(t, &b)
	h := host()
	b = e.set(t, ws, b.ID, h)
	e.dns.set(func(f *fakeDNS) {
		f.cname[h] = "custom.help.test"
		f.txt["_opendesk-verify."+h] = []string{b.HelpCenter.Domain.Records[0].Value}
	})
	e.check(t, ws, b.ID)
	e.Do("DELETE", "/v1/brands/"+b.ID, ws.Admin.As(ws), nil).Want(t, 204)
	var n int
	e.Scalar(`SELECT count(*) FROM river_job WHERE kind = 'help_domain.release' AND args->>'provider_id' = $1`, &n, "ch-"+h)
	if n != 1 {
		t.Fatalf("release jobs %d", n)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
