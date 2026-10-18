package integrations

// A small Model Context Protocol client: initialize, tools/list and tools/call over
// Streamable HTTP, falling back to the older HTTP+SSE transport. Every request goes
// through the SSRF-guarded client; the server only ever sees our bearer token.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/useopendesk/opendesk/internal/netsafe"
	"github.com/useopendesk/opendesk/internal/version"
)

const (
	protocolVersion = "2025-11-25"
	// maxMessage bounds one JSON-RPC message from a server.
	maxMessage = 4 << 20
	// maxTools bounds how many tools one server may contribute.
	maxTools = 200
)

// errUnauthorized means the server refused the credentials (401). challenge is its
// WWW-Authenticate header, for OAuth discovery.
type errUnauthorized struct{ challenge string }

func (e *errUnauthorized) Error() string { return "the server asked to sign in again" }

// errSessionGone means the server forgot our MCP session (404 with a session id).
var errSessionGone = errors.New("mcp session expired")

// errTransport means the URL does not speak Streamable HTTP (4xx on initialize).
var errTransport = errors.New("not a streamable HTTP endpoint")

// RemoteError is a JSON-RPC error from the server, safe to show: code and a clipped message.
type RemoteError struct {
	Code    int
	Message string
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("the server answered %d: %s", e.Code, e.Message)
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// RemoteTool is a tool as a server lists it.
type RemoteTool struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations *struct {
		Title           string `json:"title,omitempty"`
		ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
		DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	} `json:"annotations,omitempty"`
}

// CallResult is a tools/call answer.
type CallResult struct {
	Content []struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		URI      string `json:"uri,omitempty"`
		Name     string `json:"name,omitempty"`
		Resource *struct {
			URI  string `json:"uri"`
			Text string `json:"text,omitempty"`
		} `json:"resource,omitempty"`
	} `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
}

// Text is the result as plain text for the model: text parts, resource links and the
// structured content when there is no text.
func (r *CallResult) Text() string {
	var b strings.Builder
	for _, c := range r.Content {
		switch c.Type {
		case "text":
			b.WriteString(c.Text)
		case "resource_link":
			fmt.Fprintf(&b, "[%s](%s)", c.Name, c.URI)
		case "resource":
			if c.Resource != nil {
				b.WriteString(c.Resource.Text)
				if c.Resource.Text == "" {
					b.WriteString(c.Resource.URI)
				}
			}
		default:
			fmt.Fprintf(&b, "[%s content]", c.Type)
		}
		b.WriteString("\n")
	}
	if strings.TrimSpace(b.String()) == "" && len(r.StructuredContent) > 0 {
		b.Write(r.StructuredContent)
	}
	return strings.TrimSpace(b.String())
}

// session is one MCP connection. It is safe for concurrent use.
type session struct {
	http   *http.Client
	url    string
	bearer string
	nextID atomic.Int64

	mu        sync.Mutex
	sessionID string
	version   string
	legacy    bool
	used      time.Time
	icons     []ServerIcon
}

// ServerIcon is one of the icons a server advertises in its initialize result
// (serverInfo.icons).
type ServerIcon struct {
	Src      string    `json:"src"`
	MimeType string    `json:"mimeType,omitempty"`
	Sizes    iconSizes `json:"sizes,omitempty"`
	Theme    string    `json:"theme,omitempty"`
}

// iconSizes reads sizes as a list ("48x48", "any") or as one space-separated string.
type iconSizes []string

func (z *iconSizes) UnmarshalJSON(b []byte) error {
	var list []string
	if json.Unmarshal(b, &list) == nil {
		*z = list
		return nil
	}
	var one string
	if json.Unmarshal(b, &one) == nil {
		*z = strings.Fields(one)
	}
	return nil
}

// maxIcons bounds the icons kept from one server.
const maxIcons = 20

// parseServerIcons reads serverInfo.icons from an initialize result; anything malformed
// is left out.
func parseServerIcons(res json.RawMessage) []ServerIcon {
	var init struct {
		ServerInfo struct {
			Icons []json.RawMessage `json:"icons"`
		} `json:"serverInfo"`
	}
	if json.Unmarshal(res, &init) != nil {
		return nil
	}
	var out []ServerIcon
	for _, raw := range init.ServerInfo.Icons {
		var ic ServerIcon
		if json.Unmarshal(raw, &ic) != nil || strings.TrimSpace(ic.Src) == "" {
			continue
		}
		if out = append(out, ic); len(out) >= maxIcons {
			break
		}
	}
	return out
}

// Icons are the icons the server advertised when the session opened (none for the
// legacy transport).
func (s *session) Icons() []ServerIcon {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.icons
}

func newSession(c *http.Client, endpoint, bearer string) *session {
	return &session{http: c, url: endpoint, bearer: bearer, used: time.Now()}
}

// open runs the initialize handshake, trying Streamable HTTP first.
func (s *session) open(ctx context.Context) error {
	err := s.initialize(ctx)
	if errors.Is(err, errTransport) {
		s.mu.Lock()
		s.legacy = true
		s.mu.Unlock()
		return nil // each legacy operation opens its own stream and handshake
	}
	return err
}

func (s *session) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "opendesk", "title": "opendesk", "version": version.Version},
	}
	res, hdr, err := s.roundTrip(ctx, "initialize", params, true)
	if err != nil {
		return err
	}
	var init struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(res, &init)
	icons := parseServerIcons(res)
	s.mu.Lock()
	s.sessionID = hdr.Get("Mcp-Session-Id")
	s.version = init.ProtocolVersion
	s.icons = icons
	s.mu.Unlock()
	return s.notify(ctx, "notifications/initialized")
}

// ListTools pages through tools/list.
func (s *session) ListTools(ctx context.Context) ([]RemoteTool, error) {
	if s.isLegacy() {
		return legacyListTools(ctx, s)
	}
	var out []RemoteTool
	cursor := ""
	for page := 0; page < 20 && len(out) < maxTools; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := s.call(ctx, "tools/list", params)
		if err != nil {
			return nil, err
		}
		var res struct {
			Tools      []RemoteTool `json:"tools"`
			NextCursor string       `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &res); err != nil {
			return nil, errors.New("the server sent a tool list we could not read")
		}
		out = append(out, res.Tools...)
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	if len(out) > maxTools {
		out = out[:maxTools]
	}
	return out, nil
}

// CallTool runs one tool.
func (s *session) CallTool(ctx context.Context, name string, args json.RawMessage) (*CallResult, error) {
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	params := map[string]any{"name": name, "arguments": args}
	var raw json.RawMessage
	var err error
	if s.isLegacy() {
		raw, err = legacyCall(ctx, s, "tools/call", params)
	} else {
		raw, err = s.call(ctx, "tools/call", params)
	}
	if err != nil {
		return nil, err
	}
	var res CallResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, errors.New("the server sent a result we could not read")
	}
	return &res, nil
}

func (s *session) isLegacy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.legacy
}

func (s *session) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	res, _, err := s.roundTrip(ctx, method, params, false)
	return res, err
}

func (s *session) headers(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", "opendesk/"+version.Version)
	if s.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+s.bearer)
	}
	s.mu.Lock()
	if s.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", s.sessionID)
	}
	if s.version != "" {
		req.Header.Set("MCP-Protocol-Version", s.version)
	}
	s.mu.Unlock()
}

// notify sends a notification; servers answer 202 with no body.
func (s *session) notify(ctx context.Context, method string) error {
	body, _ := json.Marshal(rpcMessage{JSONRPC: "2.0", Method: method})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	s.headers(req)
	resp, err := s.http.Do(req)
	if err != nil {
		return transportError(err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return &errUnauthorized{challenge: resp.Header.Get("WWW-Authenticate")}
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("the server refused the request (%d)", resp.StatusCode)
	}
	return nil
}

// roundTrip posts one request and reads its response from JSON or an SSE stream.
func (s *session) roundTrip(ctx context.Context, method string, params any, initializing bool) (json.RawMessage, http.Header, error) {
	id := s.nextID.Add(1)
	idRaw := json.RawMessage(fmt.Sprintf("%d", id))
	body, err := json.Marshal(rpcMessage{JSONRPC: "2.0", ID: idRaw, Method: method, Params: params})
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	s.headers(req)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, nil, transportError(err)
	}
	defer resp.Body.Close()
	s.mu.Lock()
	s.used = time.Now()
	hadSession := s.sessionID != ""
	s.mu.Unlock()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, nil, &errUnauthorized{challenge: resp.Header.Get("WWW-Authenticate")}
	case resp.StatusCode == http.StatusNotFound && hadSession && !initializing:
		return nil, nil, errSessionGone
	case initializing && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed ||
		resp.StatusCode == http.StatusBadRequest):
		return nil, nil, errTransport
	case resp.StatusCode == http.StatusForbidden:
		return nil, nil, errors.New("the server refused access (403). Check the account's permissions")
	case resp.StatusCode >= 400:
		return nil, nil, fmt.Errorf("the server answered %d", resp.StatusCode)
	}
	ct, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	var msg *rpcMessage
	if ct == "text/event-stream" {
		msg, err = readSSEResponse(resp.Body, idRaw)
	} else {
		msg = &rpcMessage{}
		err = json.NewDecoder(io.LimitReader(resp.Body, maxMessage)).Decode(msg)
		if err != nil {
			err = errors.New("the server sent a response we could not read")
		}
	}
	if err != nil {
		return nil, nil, err
	}
	if msg.Error != nil {
		return nil, nil, &RemoteError{Code: msg.Error.Code, Message: clip(msg.Error.Message, 300)}
	}
	return msg.Result, resp.Header, nil
}

// readSSEResponse reads server-sent events until the response to id arrives. Other
// messages on the stream (progress, logs, server requests) are skipped.
func readSSEResponse(r io.Reader, id json.RawMessage) (*rpcMessage, error) {
	var found *rpcMessage
	err := readEvents(r, func(event, data string) bool {
		if event != "" && event != "message" {
			return true
		}
		var m rpcMessage
		if json.Unmarshal([]byte(data), &m) != nil {
			return true
		}
		if m.Method == "" && bytes.Equal(bytes.TrimSpace(m.ID), id) {
			found = &m
			return false
		}
		return true
	})
	if found != nil {
		return found, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, errors.New("the server closed the stream without answering")
}

// readEvents parses an SSE stream, calling fn per event until it returns false.
func readEvents(r io.Reader, fn func(event, data string) bool) error {
	sc := bufio.NewScanner(io.LimitReader(r, maxMessage*4))
	sc.Buffer(make([]byte, 64<<10), maxMessage)
	var event string
	var data strings.Builder
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if data.Len() > 0 || event != "" {
				if !fn(event, strings.TrimSuffix(data.String(), "\n")) {
					return nil
				}
			}
			event = ""
			data.Reset()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		k, v, _ := strings.Cut(line, ":")
		v = strings.TrimPrefix(v, " ")
		switch k {
		case "event":
			event = v
		case "data":
			data.WriteString(v)
			data.WriteString("\n")
		}
	}
	if err := sc.Err(); err != nil {
		return errors.New("the server's stream broke off")
	}
	if data.Len() > 0 {
		fn(event, strings.TrimSuffix(data.String(), "\n"))
	}
	return nil
}

// --- legacy HTTP+SSE transport (protocol 2024-11-05) ----------------------------------

// legacyStream is one open SSE stream: the server names a POST endpoint in its first
// event and answers every request on the stream.
type legacyStream struct {
	s        *session
	endpoint string
	cancel   context.CancelFunc
	mu       sync.Mutex
	waiting  map[string]chan *rpcMessage
	done     chan struct{}
	err      error
}

func openLegacy(ctx context.Context, s *session) (*legacyStream, error) {
	sctx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(sctx, http.MethodGet, s.url, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	if s.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+s.bearer)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		cancel()
		return nil, transportError(err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		cancel()
		return nil, &errUnauthorized{challenge: resp.Header.Get("WWW-Authenticate")}
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("the server answered %d", resp.StatusCode)
	}
	ls := &legacyStream{s: s, cancel: cancel, waiting: map[string]chan *rpcMessage{}, done: make(chan struct{})}
	endpoint := make(chan string, 1)
	go func() {
		defer resp.Body.Close()
		defer close(ls.done)
		ls.err = readEvents(resp.Body, func(event, data string) bool {
			if event == "endpoint" {
				select {
				case endpoint <- data:
				default:
				}
				return true
			}
			var m rpcMessage
			if json.Unmarshal([]byte(data), &m) != nil || m.Method != "" {
				return true
			}
			ls.mu.Lock()
			ch := ls.waiting[string(bytes.TrimSpace(m.ID))]
			ls.mu.Unlock()
			if ch != nil {
				ch <- &m
			}
			return true
		})
	}()
	select {
	case e := <-endpoint:
		u, err := sameOrigin(s.url, e)
		if err != nil {
			ls.close()
			return nil, err
		}
		ls.endpoint = u
	case <-ls.done:
		cancel()
		return nil, errors.New("the server closed the stream before naming its endpoint")
	case <-time.After(15 * time.Second):
		ls.close()
		return nil, errors.New("the server did not name its endpoint")
	}
	return ls, nil
}

func (ls *legacyStream) close() { ls.cancel() }

func (ls *legacyStream) request(ctx context.Context, method string, params any, notification bool) (json.RawMessage, error) {
	m := rpcMessage{JSONRPC: "2.0", Method: method, Params: params}
	var ch chan *rpcMessage
	if !notification {
		id := fmt.Sprintf("%d", ls.s.nextID.Add(1))
		m.ID = json.RawMessage(id)
		ch = make(chan *rpcMessage, 1)
		ls.mu.Lock()
		ls.waiting[id] = ch
		ls.mu.Unlock()
	}
	body, _ := json.Marshal(m)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ls.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if ls.s.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+ls.s.bearer)
	}
	resp, err := ls.s.http.Do(req)
	if err != nil {
		return nil, transportError(err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, &errUnauthorized{challenge: resp.Header.Get("WWW-Authenticate")}
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("the server answered %d", resp.StatusCode)
	}
	if notification {
		return nil, nil
	}
	select {
	case msg := <-ch:
		if msg.Error != nil {
			return nil, &RemoteError{Code: msg.Error.Code, Message: clip(msg.Error.Message, 300)}
		}
		return msg.Result, nil
	case <-ls.done:
		return nil, errors.New("the server's stream broke off")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func legacyHandshake(ctx context.Context, s *session) (*legacyStream, error) {
	ls, err := openLegacy(ctx, s)
	if err != nil {
		return nil, err
	}
	params := map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "opendesk", "version": version.Version}}
	if _, err := ls.request(ctx, "initialize", params, false); err != nil {
		ls.close()
		return nil, err
	}
	if _, err := ls.request(ctx, "notifications/initialized", nil, true); err != nil {
		ls.close()
		return nil, err
	}
	return ls, nil
}

func legacyCall(ctx context.Context, s *session, method string, params any) (json.RawMessage, error) {
	ls, err := legacyHandshake(ctx, s)
	if err != nil {
		return nil, err
	}
	defer ls.close()
	return ls.request(ctx, method, params, false)
}

func legacyListTools(ctx context.Context, s *session) ([]RemoteTool, error) {
	ls, err := legacyHandshake(ctx, s)
	if err != nil {
		return nil, err
	}
	defer ls.close()
	var out []RemoteTool
	cursor := ""
	for page := 0; page < 20 && len(out) < maxTools; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := ls.request(ctx, "tools/list", params, false)
		if err != nil {
			return nil, err
		}
		var res struct {
			Tools      []RemoteTool `json:"tools"`
			NextCursor string       `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &res); err != nil {
			return nil, errors.New("the server sent a tool list we could not read")
		}
		out = append(out, res.Tools...)
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	if len(out) > maxTools {
		out = out[:maxTools]
	}
	return out, nil
}

// sameOrigin resolves ref against base and refuses another origin, so a server cannot
// point our bearer token at a different host.
func sameOrigin(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	r, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return "", errors.New("the server named an endpoint we could not read")
	}
	u := b.ResolveReference(r)
	if u.Scheme != b.Scheme || u.Host != b.Host {
		return "", errors.New("the server named an endpoint on another host")
	}
	return u.String(), nil
}

// transportError hides addresses and tokens a network error might carry.
func transportError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return errors.New("the server took too long to answer")
	}
	if errors.Is(err, netsafe.ErrBlocked) {
		return errors.New("the server's address is not a public internet address")
	}
	return errors.New("the server could not be reached")
}

func clip(s string, n int) string {
	s = strings.TrimSpace(strings.ToValidUTF8(s, ""))
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}
