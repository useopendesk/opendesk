package whatsapp

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// whatsappTemplate is one template send as the outbound message carries it: the approved
// template Meta knows plus the values an agent filled in for this message. It lives in
// messages.delivery_meta under "whatsapp_template" (contracts/whatsapp.md).
type whatsappTemplate struct {
	ID         string            `json:"id,omitempty"`
	Name       string            `json:"name"`
	Language   string            `json:"language"`
	Body       string            `json:"body,omitempty"`
	Params     map[string]string `json:"params,omitempty"`
	HeaderType string            `json:"header_type,omitempty"`
	HeaderText string            `json:"header_text,omitempty"`
	ButtonURLs map[string]string `json:"button_urls,omitempty"`
}

// templateEnvelope reads the template of an outbound message out of delivery_meta. It
// returns nil for a free-form message.
func whatsappTemplateFromMeta(raw []byte) *whatsappTemplate {
	if len(raw) == 0 {
		return nil
	}
	var env struct {
		Template *whatsappTemplate `json:"whatsapp_template"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Template == nil || strings.TrimSpace(env.Template.Name) == "" {
		return nil
	}
	return env.Template
}

// components renders Meta's components array for this send: the body's parameters in the
// order the template's placeholders appear, an optional text header with its one
// placeholder, and URL button values by button index. Components a template does not need
// are left out, so Meta uses the approved text as it is.
func (t *whatsappTemplate) components() []map[string]any {
	var out []map[string]any
	if strings.EqualFold(t.HeaderType, "TEXT") && strings.TrimSpace(t.HeaderText) != "" {
		header := map[string]any{"type": "header"}
		if key := orderedPlaceholders(t.HeaderText); len(key) > 0 {
			header["parameters"] = []map[string]any{{"type": "text", "text": t.param(key[0])}}
		}
		out = append(out, header)
	}
	if keys := orderedPlaceholders(t.Body); len(keys) > 0 {
		params := make([]map[string]any, 0, len(keys))
		for _, key := range keys {
			params = append(params, map[string]any{"type": "text", "text": t.param(key)})
		}
		out = append(out, map[string]any{"type": "body", "parameters": params})
	}
	if len(t.ButtonURLs) > 0 {
		indexes := make([]int, 0, len(t.ButtonURLs))
		for raw := range t.ButtonURLs {
			if i, err := strconv.Atoi(raw); err == nil {
				indexes = append(indexes, i)
			}
		}
		sort.Ints(indexes)
		for _, i := range indexes {
			out = append(out, map[string]any{
				"type":       "button",
				"sub_type":   "url",
				"index":      strconv.Itoa(i),
				"parameters": []map[string]any{{"type": "text", "text": t.ButtonURLs[strconv.Itoa(i)]}},
			})
		}
	}
	return out
}

// param is the value for one placeholder, or empty when the agent left it blank (Meta then
// refuses the send with its own error).
func (t *whatsappTemplate) param(key string) string {
	if t.Params == nil {
		return ""
	}
	if v, ok := t.Params[key]; ok {
		return v
	}
	// Meta numbers positional placeholders from one; a template listed with named ones is
	// filled by its name. Both are accepted as the same key.
	if i, err := strconv.Atoi(key); err == nil {
		return t.Params[strconv.Itoa(i)]
	}
	return ""
}

var placeholderPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)

// orderedPlaceholders returns the distinct placeholders of a template text in the order they
// appear, which is the order Meta expects their parameters.
func orderedPlaceholders(text string) []string {
	matches := placeholderPattern.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		key := m[1]
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

// renderTemplateBody fills a template's body with the agent's values for the message's own
// text, so the timeline shows what the customer gets.
func renderTemplateBody(body string, params map[string]string) string {
	if body == "" || len(params) == 0 {
		return body
	}
	return placeholderPattern.ReplaceAllStringFunc(body, func(match string) string {
		sub := placeholderPattern.FindStringSubmatch(match)
		if sub == nil {
			return match
		}
		if v, ok := params[sub[1]]; ok {
			return v
		}
		return match
	})
}
