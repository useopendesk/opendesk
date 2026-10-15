package bots

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/useopendesk/opendesk/internal/httpx"
)

// The look (contracts/bots.md "The look"): one JSON document whose every value is
// from an allow-list or a #rrggbb colour. web/src/lib/botlook.ts is the canonical
// TypeScript twin; ParseLook gives the same answer and message as its lookProblem.

// Fill is the body colour: a solid colour or a gradient from Color to To.
type Fill struct {
	Type  string `json:"type" enum:"solid,gradient" doc:"solid paints the shape in color; gradient runs from color at the top left to to"`
	Color string `json:"color" pattern:"^#[0-9a-fA-F]{6}$" doc:"#rrggbb, the fill (the gradient's first stop)"`
	To    string `json:"to" pattern:"^#[0-9a-fA-F]{6}$" doc:"#rrggbb, the gradient's second stop"`
}

// Look is how a bot is drawn. Every key is always present.
type Look struct {
	Kind       string `json:"kind" enum:"built,emoji,image" doc:"built draws the look; emoji shows the emoji on the shape; image shows the uploaded picture (image_url)"`
	Shape      string `json:"shape" enum:"blob,circle,rounded,squircle,hexagon,cloud" doc:"The body shape"`
	Fill       Fill   `json:"fill" doc:"The body colour"`
	Eyes       string `json:"eyes" enum:"dots,ovals,pills,happy,sleepy,sparkle,wink,glasses,star" doc:"The eye style"`
	Mouth      string `json:"mouth" enum:"none,smile,grin,open,cat,flat,tongue,smirk,tiny" doc:"The mouth style"`
	Accessory  string `json:"accessory" enum:"none,antenna,headphones,bow,cap,halo,sprout,crown" doc:"What the bot wears"`
	Accent     string `json:"accent" pattern:"^#[0-9a-fA-F]{6}$" doc:"#rrggbb: the bow, cap, headphones and antenna tip"`
	Blush      bool   `json:"blush" doc:"Rosy cheeks"`
	Background string `json:"background" enum:"none,soft,ring" doc:"Behind the bot: nothing, a soft disc or a ring"`
	Emoji      string `json:"emoji" doc:"One emoji (up to 10 code points, 32 bytes): the face when kind is emoji; may be empty"`
}

// The allow-lists, in the order of web/src/lib/botlook.ts.
var (
	Kinds       = []string{"built", "emoji", "image"}
	Shapes      = []string{"blob", "circle", "rounded", "squircle", "hexagon", "cloud"}
	Eyes        = []string{"dots", "ovals", "pills", "happy", "sleepy", "sparkle", "wink", "glasses", "star"}
	Mouths      = []string{"none", "smile", "grin", "open", "cat", "flat", "tongue", "smirk", "tiny"}
	Accessories = []string{"none", "antenna", "headphones", "bow", "cap", "halo", "sprout", "crown"}
	Backgrounds = []string{"none", "soft", "ring"}
)

// OllieLook is the default bot: the violet blob with two pill eyes.
var OllieLook = Look{
	Kind:       "built",
	Shape:      "blob",
	Fill:       Fill{Type: "gradient", Color: "#c9bcff", To: "#4a2bc4"},
	Eyes:       "pills",
	Mouth:      "none",
	Accessory:  "none",
	Accent:     "#f59e0b",
	Blush:      false,
	Background: "none",
	Emoji:      "",
}

var (
	lookKeys = []string{"kind", "shape", "fill", "eyes", "mouth", "accessory", "accent", "blush", "background", "emoji"}
	fillKeys = []string{"type", "color", "to"}
)

const (
	inkLight = "#ffffff"
	inkDark  = "#2a2838"
)

// IsHex reports whether s is a #rrggbb colour (either case).
func IsHex(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for i := 1; i < 7; i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// IsEmoji reports whether s is one emoji: a short run of pictographs, modifiers and
// joiners (at most 10 code points and 32 bytes), with at least one pictograph or keycap.
func IsEmoji(s string) bool {
	if s == "" || len(s) > 32 || utf8.RuneCountInString(s) > 10 {
		return false
	}
	pict := false
	for _, c := range s {
		p := (c >= 0x1f000 && c <= 0x1faff) || (c >= 0x2600 && c <= 0x27bf) || (c >= 0x2300 && c <= 0x23ff) || (c >= 0x2b00 && c <= 0x2bff) ||
			(c >= 0x2190 && c <= 0x21ff) || (c >= 0x25a0 && c <= 0x25ff) || c == 0x3030 || c == 0x303d || c == 0x3297 || c == 0x3299 ||
			c == 0xa9 || c == 0xae || c == 0x203c || c == 0x2049 || c == 0x2122 || c == 0x2139 || c == 0x24c2
		mod := c == 0x200d || c == 0xfe0f || c == 0x20e3 || (c >= 0xe0020 && c <= 0xe007f) || (c >= 0x30 && c <= 0x39) || c == 0x23 || c == 0x2a
		if !p && !mod {
			return false
		}
		pict = pict || p || c == 0x20e3
	}
	return pict
}

func rgb(hex string) (int, int, int) {
	n, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil {
		return 0, 0, 0
	}
	return int(n>>16) & 255, int(n>>8) & 255, int(n) & 255
}

// Ink is the colour the eyes and mouth are drawn in: white on dark fills, ink on
// light ones (the brightness of the fill, or of the mean of a gradient's stops).
func Ink(f Fill) string {
	r, g, b := rgb(f.Color)
	if f.Type == "gradient" {
		r2, g2, b2 := rgb(f.To)
		r, g, b = (r+r2)/2, (g+g2)/2, (b+b2)/2
	}
	if 299*r+587*g+114*b >= 165000 {
		return inkDark
	}
	return inkLight
}

// Hash is a short digest of the look, for cache busting in avatar URLs.
func (l Look) Hash() string {
	b, _ := json.Marshal(l)
	h := fnv.New32a()
	h.Write(b)
	return fmt.Sprintf("%08x", h.Sum32())
}

// object reads a JSON object keeping its keys in the order JavaScript's Object.keys
// gives them (array-index keys ascending, then the rest as first written; a repeated
// key keeps its first place and its last value). ok is false for anything else.
func object(raw []byte) (keys []string, vals map[string]json.RawMessage, ok bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, nil, false
	}
	vals = map[string]json.RawMessage{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, nil, false
		}
		k, isKey := t.(string)
		if !isKey {
			return nil, nil, false
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, nil, false
		}
		if _, seen := vals[k]; !seen {
			keys = append(keys, k)
		}
		vals[k] = v
	}
	if t, err := dec.Token(); err != nil || t != json.Delim('}') {
		return nil, nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, nil, false
	}
	var index []string
	rest := keys[:0:0]
	for _, k := range keys {
		if arrayIndex(k) {
			index = append(index, k)
		} else {
			rest = append(rest, k)
		}
	}
	sort.Slice(index, func(i, j int) bool {
		a, _ := strconv.ParseUint(index[i], 10, 32)
		b, _ := strconv.ParseUint(index[j], 10, 32)
		return a < b
	})
	return append(index, rest...), vals, true
}

// arrayIndex reports whether k is a canonical array index ("0", "17"; not "017").
func arrayIndex(k string) bool {
	if k == "" || len(k) > 10 || (len(k) > 1 && k[0] == '0') {
		return false
	}
	n, err := strconv.ParseUint(k, 10, 64)
	return err == nil && n < 1<<32-1
}

// str reads a JSON string value; ok is false for any other JSON type.
func str(raw json.RawMessage) (string, bool) {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// ParseLook reads a look strictly: a JSON object with every key and no other, every
// value from its allow-list. The colours come back lowercased. A refusal is a 422
// naming the field, with the message the web app shows for the same look.
func ParseLook(raw []byte) (Look, error) {
	keys, o, ok := object(raw)
	if !ok {
		return Look{}, httpx.Validation("look must be an object")
	}
	for _, k := range keys {
		if !slices.Contains(lookKeys, k) {
			return Look{}, httpx.Validation("look." + k + " is not a look setting")
		}
	}
	for _, k := range lookKeys {
		if _, ok := o[k]; !ok {
			return Look{}, httpx.Validation("look." + k + " is required")
		}
	}
	var l Look
	oneOf := func(k string, list []string, dst *string, msg string) error {
		s, ok := str(o[k])
		if !ok || !slices.Contains(list, s) {
			return httpx.Validation(msg)
		}
		*dst = s
		return nil
	}
	for _, c := range []struct {
		key  string
		list []string
		dst  *string
		msg  string
	}{
		{"kind", Kinds, &l.Kind, "look.kind must be built, emoji or image"},
		{"shape", Shapes, &l.Shape, "look.shape is not a shape"},
		{"eyes", Eyes, &l.Eyes, "look.eyes is not an eye style"},
		{"mouth", Mouths, &l.Mouth, "look.mouth is not a mouth style"},
		{"accessory", Accessories, &l.Accessory, "look.accessory is not an accessory"},
		{"background", Backgrounds, &l.Background, "look.background must be none, soft or ring"},
	} {
		if err := oneOf(c.key, c.list, c.dst, c.msg); err != nil {
			return Look{}, err
		}
	}
	if err := json.Unmarshal(o["blush"], &l.Blush); err != nil || string(bytes.TrimSpace(o["blush"])) == "null" {
		return Look{}, httpx.Validation("look.blush must be true or false")
	}
	accent, ok := str(o["accent"])
	if !ok || !IsHex(accent) {
		return Look{}, httpx.Validation("look.accent must be a #rrggbb colour")
	}
	l.Accent = strings.ToLower(accent)

	fkeys, f, ok := object(o["fill"])
	if !ok {
		return Look{}, httpx.Validation("look.fill must be an object")
	}
	for _, k := range fkeys {
		if !slices.Contains(fillKeys, k) {
			return Look{}, httpx.Validation("look.fill." + k + " is not a fill setting")
		}
	}
	typ, ok := str(f["type"])
	if !ok || (typ != "solid" && typ != "gradient") {
		return Look{}, httpx.Validation("look.fill.type must be solid or gradient")
	}
	l.Fill.Type = typ
	color, ok := str(f["color"])
	if !ok || !IsHex(color) {
		return Look{}, httpx.Validation("look.fill.color must be a #rrggbb colour")
	}
	to, ok := str(f["to"])
	if !ok || !IsHex(to) {
		return Look{}, httpx.Validation("look.fill.to must be a #rrggbb colour")
	}
	l.Fill.Color, l.Fill.To = strings.ToLower(color), strings.ToLower(to)

	emoji, isStr := str(o["emoji"])
	if !isStr || (emoji != "" && !IsEmoji(emoji)) {
		return Look{}, httpx.Validation("look.emoji must be one emoji")
	}
	if l.Kind == "emoji" && emoji == "" {
		return Look{}, httpx.Validation("look.emoji is required for an emoji look")
	}
	l.Emoji = emoji
	return l, nil
}

// Validate checks a look already in a struct (after merging, say) with the same
// rules and messages as ParseLook.
func (l Look) Validate() error {
	switch {
	case !slices.Contains(Kinds, l.Kind):
		return httpx.Validation("look.kind must be built, emoji or image")
	case !slices.Contains(Shapes, l.Shape):
		return httpx.Validation("look.shape is not a shape")
	case !slices.Contains(Eyes, l.Eyes):
		return httpx.Validation("look.eyes is not an eye style")
	case !slices.Contains(Mouths, l.Mouth):
		return httpx.Validation("look.mouth is not a mouth style")
	case !slices.Contains(Accessories, l.Accessory):
		return httpx.Validation("look.accessory is not an accessory")
	case !slices.Contains(Backgrounds, l.Background):
		return httpx.Validation("look.background must be none, soft or ring")
	case !IsHex(l.Accent):
		return httpx.Validation("look.accent must be a #rrggbb colour")
	case l.Fill.Type != "solid" && l.Fill.Type != "gradient":
		return httpx.Validation("look.fill.type must be solid or gradient")
	case !IsHex(l.Fill.Color):
		return httpx.Validation("look.fill.color must be a #rrggbb colour")
	case !IsHex(l.Fill.To):
		return httpx.Validation("look.fill.to must be a #rrggbb colour")
	case l.Emoji != "" && !IsEmoji(l.Emoji):
		return httpx.Validation("look.emoji must be one emoji")
	case l.Kind == "emoji" && l.Emoji == "":
		return httpx.Validation("look.emoji is required for an emoji look")
	}
	return nil
}
