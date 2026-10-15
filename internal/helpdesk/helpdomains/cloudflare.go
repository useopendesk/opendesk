package helpdomains

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Cloudflare adds and removes custom hostnames on a Cloudflare for SaaS zone. The token
// needs SSL and Certificates edit on that zone only.
type Cloudflare struct {
	ZoneID string
	Token  string
	// BaseURL is the API root; tests point it at a fake.
	BaseURL string
	HTTP    *http.Client
}

// NewCloudflare returns a client for the zone.
func NewCloudflare(zoneID, token string) *Cloudflare {
	return &Cloudflare{ZoneID: zoneID, Token: token, BaseURL: "https://api.cloudflare.com/client/v4", HTTP: &http.Client{Timeout: 15 * time.Second}}
}

// CustomHostname is the part of a Cloudflare custom hostname a check reads.
type CustomHostname struct {
	ID        string    `json:"id"`
	Hostname  string    `json:"hostname"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	SSL       struct {
		Status           string `json:"status"`
		ValidationErrors []struct {
			Message string `json:"message"`
		} `json:"validation_errors"`
	} `json:"ssl"`
	VerificationErrors []string `json:"verification_errors"`
}

// ErrNotFound answers a custom hostname the zone does not have.
var ErrNotFound = errors.New("cloudflare: custom hostname not found")

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// APIError is an error Cloudflare answered with.
type APIError struct {
	Status int
	Errors []cfError
}

func (e *APIError) Error() string {
	if len(e.Errors) > 0 {
		return fmt.Sprintf("cloudflare: %d %d %s", e.Status, e.Errors[0].Code, e.Errors[0].Message)
	}
	return fmt.Sprintf("cloudflare: %d", e.Status)
}

// duplicateCode is Cloudflare's answer to a hostname the zone already has.
const duplicateCode = 1406

func (c *Cloudflare) do(ctx context.Context, method, path string, body, out any) (json.RawMessage, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/zones/"+url.PathEscape(c.ZoneID)+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: %s custom hostnames: %w", method, errors.Unwrap(err))
	}
	defer res.Body.Close()
	var env struct {
		Success    bool            `json:"success"`
		Errors     []cfError       `json:"errors"`
		Result     json.RawMessage `json:"result"`
		ResultInfo json.RawMessage `json:"result_info"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&env); err != nil {
		return nil, fmt.Errorf("cloudflare: %d, unreadable answer", res.StatusCode)
	}
	if res.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if !env.Success || res.StatusCode >= 300 {
		return nil, &APIError{Status: res.StatusCode, Errors: env.Errors}
	}
	if out != nil && len(env.Result) > 0 && string(env.Result) != "null" {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return nil, fmt.Errorf("cloudflare: unreadable result: %w", err)
		}
	}
	return env.ResultInfo, nil
}

// sslSettings asks for a certificate validated over HTTP, which Cloudflare completes on
// its own once the hostname's CNAME points at the zone.
var sslSettings = map[string]any{"method": "http", "type": "dv", "settings": map[string]any{"min_tls_version": "1.2"}}

// Create adds a custom hostname, or returns the zone's existing one for it.
func (c *Cloudflare) Create(ctx context.Context, hostname string) (CustomHostname, error) {
	var h CustomHostname
	_, err := c.do(ctx, http.MethodPost, "/custom_hostnames", map[string]any{"hostname": hostname, "ssl": sslSettings}, &h)
	var api *APIError
	if errors.As(err, &api) && len(api.Errors) > 0 && api.Errors[0].Code == duplicateCode {
		return c.Find(ctx, hostname)
	}
	return h, err
}

// Find returns the zone's custom hostname for a hostname, or ErrNotFound.
func (c *Cloudflare) Find(ctx context.Context, hostname string) (CustomHostname, error) {
	var list []CustomHostname
	if _, err := c.do(ctx, http.MethodGet, "/custom_hostnames?hostname="+url.QueryEscape(hostname), nil, &list); err != nil {
		return CustomHostname{}, err
	}
	for _, h := range list {
		if h.Hostname == hostname {
			return h, nil
		}
	}
	return CustomHostname{}, ErrNotFound
}

// Get returns a custom hostname by id, or ErrNotFound.
func (c *Cloudflare) Get(ctx context.Context, id string) (CustomHostname, error) {
	var h CustomHostname
	_, err := c.do(ctx, http.MethodGet, "/custom_hostnames/"+url.PathEscape(id), nil, &h)
	return h, err
}

// Revalidate starts the certificate's validation again, after it timed out.
func (c *Cloudflare) Revalidate(ctx context.Context, id string) (CustomHostname, error) {
	var h CustomHostname
	_, err := c.do(ctx, http.MethodPatch, "/custom_hostnames/"+url.PathEscape(id), map[string]any{"ssl": sslSettings}, &h)
	return h, err
}

// Delete removes a custom hostname; one already gone is not an error.
func (c *Cloudflare) Delete(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/custom_hostnames/"+url.PathEscape(id), nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// List returns one page of the zone's custom hostnames and whether more follow.
func (c *Cloudflare) List(ctx context.Context, page int) ([]CustomHostname, bool, error) {
	var list []CustomHostname
	raw, err := c.do(ctx, http.MethodGet, "/custom_hostnames?per_page=50&page="+strconv.Itoa(page), nil, &list)
	if err != nil {
		return nil, false, err
	}
	var info struct {
		Page       int `json:"page"`
		TotalPages int `json:"total_pages"`
	}
	_ = json.Unmarshal(raw, &info)
	return list, info.Page < info.TotalPages, nil
}
