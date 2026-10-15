package integrations

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/core"
	"github.com/useopendesk/opendesk/internal/helpdesk"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
)

func TestMain(m *testing.M) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		os.Setenv("TEST_DATABASE_URL", "postgres://opendesk:opendesk@localhost:55432/opendesk_integrations?sslmode=disable")
	}
	os.Exit(m.Run())
}

func apiEnv(t *testing.T) (*hdtest.Env, hdtest.WS) {
	t.Helper()
	e := hdtest.New(t, core.Register, helpdesk.Register, Register)
	return e, e.NewWorkspace()
}

func TestConnectListUpdateDisconnect(t *testing.T) {
	e, ws := apiEnv(t)
	srv := httptest.NewServer(&fakeMCP{bearer: "pat-1"})
	defer srv.Close()
	admin, agent, viewer := ws.Admin.As(ws), ws.Agent.As(ws), ws.Viewer.As(ws)

	cat := e.Do("GET", "/v1/integrations/catalog", viewer, nil).Want(t, 200).Map(t)
	if len(cat["data"].([]any)) < 10 {
		t.Fatalf("catalog = %v", cat)
	}
	body := map[string]any{"provider": "custom", "url": srv.URL + "/mcp?key=s3cret", "auth": "token", "token": "pat-1", "name": "Tracker"}
	e.Do("POST", "/v1/integrations", agent, body).Want(t, 403)
	e.Do("POST", "/v1/integrations", admin, map[string]any{"provider": "custom", "url": srv.URL + "/mcp", "auth": "token", "token": "wrong"}).Want(t, 422)
	_, seq := e.Events(ws.ID, 0)
	var created struct {
		Integration Integration `json:"integration"`
	}
	cr := e.Do("POST", "/v1/integrations", admin, body).Want(t, 201)
	if v := cr.Header.Values("Set-Cookie"); len(v) > 0 {
		t.Fatalf("token connect set cookies: %q", v)
	}
	cr.JSON(t, &created)
	x := created.Integration
	if x.Status != "connected" || x.Name != "Tracker" || len(x.Tools) != 2 || x.URL != "" || strings.Contains(x.Host, "s3cret") {
		t.Fatalf("integration = %+v", x)
	}
	if types, _ := e.Events(ws.ID, seq); !slices.Contains(types, "integration.created") {
		t.Fatalf("events = %v", types)
	}
	var auditN int
	e.Scalar(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'integration.connected' AND NOT (data::text LIKE '%s3cret%')`, &auditN, ws.ID)
	if auditN != 1 {
		t.Fatalf("audit entries = %d", auditN)
	}
	var sealed string
	e.Scalar(`SELECT encode(credentials_enc, 'escape') FROM integrations WHERE id = $1`, &sealed, x.ID)
	if strings.Contains(sealed, "pat-1") {
		t.Fatal("token stored in the clear")
	}

	// Viewers read the list; nothing secret is in it.
	list := e.Do("GET", "/v1/integrations", viewer, nil).Want(t, 200)
	if strings.Contains(string(list.Body), "pat-1") || strings.Contains(string(list.Body), "s3cret") {
		t.Fatalf("list leaks: %s", list.Body)
	}

	path := "/v1/integrations/" + x.ID.String()
	e.Do("PATCH", path, agent, map[string]any{"enabled": false}).Want(t, 403)
	var upd Integration
	e.Do("PATCH", path, admin, map[string]any{"disabled_tools": []string{"create_issue", "unknown"}}).Want(t, 200).JSON(t, &upd)
	for _, tl := range upd.Tools {
		if tl.Enabled == (tl.Name == "create_issue") {
			t.Fatalf("tools = %+v", upd.Tools)
		}
	}
	got, err := For(e.D).Tools(e.Ctx, ws.ID)
	if err != nil || len(got) != 1 || got[0].Remote != "search_issues" || !got[0].ReadOnly {
		t.Fatalf("offered = %+v %v", got, err)
	}
	if _, err := For(e.D).Call(e.Ctx, ws.ID, x.ID, "create_issue", json.RawMessage(`{}`)); err == nil {
		t.Fatal("a turned off tool ran")
	}
	res, err := For(e.D).Call(e.Ctx, ws.ID, x.ID, "search_issues", json.RawMessage(`{"title":"x"}`))
	if err != nil || !strings.Contains(res.Text(), "linear.app") {
		t.Fatalf("call = %v %v", res, err)
	}

	// A wrong new token is refused and leaves the working connection alone.
	e.Do("PATCH", path, admin, map[string]any{"token": "wrong"}).Want(t, 422)
	var status string
	e.Scalar(`SELECT status FROM integrations WHERE id = $1`, &status, x.ID)
	if status != "connected" {
		t.Fatalf("status after a refused token = %s", status)
	}
	e.Do("PATCH", path, admin, map[string]any{"token": "pat-1"}).Want(t, 200)

	// Another workspace cannot see or touch it.
	other := e.NewWorkspace()
	e.Do("PATCH", path, other.Admin.As(other), map[string]any{"enabled": false}).Want(t, 404)
	e.Do("DELETE", path, other.Admin.As(other), nil).Want(t, 404)

	e.Do("DELETE", path, admin, nil).Want(t, 204)
	e.Do("DELETE", path, admin, nil).Want(t, 404)
	e.Scalar(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'integration.disconnected'`, &auditN, ws.ID)
	if auditN != 1 {
		t.Fatalf("disconnect audit = %d", auditN)
	}
}

func TestOAuthConnectStartsSignIn(t *testing.T) {
	e, ws := apiEnv(t)
	var origin string
	mux := http.NewServeMux()
	mux.Handle("/mcp", &fakeMCP{bearer: "never"})
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"resource": origin + "/mcp", "authorization_servers": []string{origin}})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": origin, "authorization_endpoint": origin + "/authorize",
			"token_endpoint": origin + "/token", "registration_endpoint": origin + "/register", "code_challenge_methods_supported": []string{"S256"}})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "c-1"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	origin = srv.URL

	r := e.Do("POST", "/v1/integrations", ws.Admin.As(ws), map[string]any{"provider": "custom", "url": origin + "/mcp"}).Want(t, 201)
	var out struct {
		Integration  Integration `json:"integration"`
		AuthorizeURL string      `json:"authorize_url"`
	}
	r.JSON(t, &out)
	if out.Integration.Status != "pending" || !strings.HasPrefix(out.AuthorizeURL, origin+"/authorize?") {
		t.Fatalf("out = %+v", out)
	}
	au, _ := url.Parse(out.AuthorizeURL)
	if au.Query().Get("client_id") != "c-1" || au.Query().Get("code_challenge") == "" || au.Query().Get("state") == "" {
		t.Fatalf("authorize = %s", out.AuthorizeURL)
	}
	ck := r.Header.Get("Set-Cookie")
	if !strings.Contains(ck, "od_integration_oauth=") || !strings.Contains(ck, "HttpOnly") || !strings.Contains(ck, "Path=/v1/integrations/oauth") {
		t.Fatalf("cookie = %s", ck)
	}

	// The callback refuses a state without the browser's cookie.
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Get(e.Srv.URL + "/v1/integrations/oauth/callback?code=x&state=" + url.QueryEscape(au.Query().Get("state")))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); resp.StatusCode != 302 || !strings.Contains(loc, "error=oauth_state") {
		t.Fatalf("callback = %d %s", resp.StatusCode, loc)
	}
}

func TestTicketLinks(t *testing.T) {
	e, ws := apiEnv(t)
	agent, viewer := ws.Agent.As(ws), ws.Viewer.As(ws)
	var tk struct {
		ID string `json:"id"`
	}
	e.Do("POST", "/v1/tickets", agent, map[string]any{"subject": "Broken export", "body_html": "It fails"}).Want(t, 201).JSON(t, &tk)
	path := "/v1/tickets/" + tk.ID + "/links"
	e.Do("POST", path, viewer, map[string]any{"url": "https://linear.app/a/issue/A-1"}).Want(t, 403)
	e.Do("POST", path, agent, map[string]any{"url": "javascript:alert(1)"}).Want(t, 422)
	e.Do("POST", path, agent, map[string]any{"url": "ftp://linear.app/x"}).Want(t, 422)
	l := e.Do("POST", path, agent, map[string]any{"url": "https://linear.app/a/issue/A-1", "title": "  Export   fails "}).Want(t, 201).Map(t)
	if l["provider"] != "linear" || l["title"] != "Export fails" || l["actor_type"] != "user" {
		t.Fatalf("link = %v", l)
	}
	again := e.Do("POST", path, agent, map[string]any{"url": "https://linear.app/a/issue/A-1"}).Want(t, 201).Map(t)
	if again["id"] != l["id"] {
		t.Fatal("same link added twice")
	}
	other := e.NewWorkspace()
	e.Do("GET", path, other.Agent.As(other), nil).Want(t, 404)
	e.Do("DELETE", path+"/"+l["id"].(string), other.Agent.As(other), nil).Want(t, 404)
	if n := len(e.Do("GET", path, viewer, nil).Want(t, 200).Map(t)["data"].([]any)); n != 1 {
		t.Fatalf("links = %d", n)
	}
	e.Do("DELETE", path+"/"+l["id"].(string), agent, nil).Want(t, 204)
	var acts int
	e.Scalar(`SELECT count(*) FROM activities WHERE ticket_id = $1 AND type IN ('linked','unlinked')`, &acts, tk.ID)
	if acts != 2 {
		t.Fatalf("activities = %d", acts)
	}
}

func TestTicketLinksFollowVisibility(t *testing.T) {
	e, ws := apiEnv(t)
	lead := e.AddUser(ws, "agent", "Lena Lead")
	var team uuid.UUID
	e.Scalar(`INSERT INTO teams (id, workspace_id, name, private) VALUES (gen_random_uuid(), $1, 'Security', true) RETURNING id`, &team, ws.ID)
	e.Exec(`INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, 'lead')`, team, lead.ID)
	var tk struct {
		ID string `json:"id"`
	}
	e.Do("POST", "/v1/tickets", lead.As(ws), map[string]any{"subject": "Breach", "body_html": "x", "team_id": team.String()}).Want(t, 201).JSON(t, &tk)
	path := "/v1/tickets/" + tk.ID + "/links"
	l := e.Do("POST", path, lead.As(ws), map[string]any{"url": "https://linear.app/a/issue/S-1"}).Want(t, 201).Map(t)
	outsider := ws.Agent.As(ws)
	e.Do("GET", path, outsider, nil).Want(t, 404)
	e.Do("POST", path, outsider, map[string]any{"url": "https://linear.app/a/issue/S-2"}).Want(t, 404)
	e.Do("DELETE", path+"/"+l["id"].(string), outsider, nil).Want(t, 404)
	if n := len(e.Do("GET", path, lead.As(ws), nil).Want(t, 200).Map(t)["data"].([]any)); n != 1 {
		t.Fatalf("links = %d", n)
	}
	var scoped int
	e.Scalar(`SELECT count(*) FROM events WHERE workspace_id = $1 AND type = 'ticket_link.created' AND team_id = $2`, &scoped, ws.ID, team)
	if scoped != 1 {
		t.Fatalf("link event not scoped to the team: %d", scoped)
	}
}
