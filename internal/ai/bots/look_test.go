package bots

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/useopendesk/opendesk/internal/httpx"
)

type fixture struct {
	Look json.RawMessage `json:"look"`
	SVG  string          `json:"svg"`
}

func fixtures(t *testing.T) []fixture {
	t.Helper()
	raw, err := os.ReadFile("../../../web/src/lib/botlook.fixtures.json")
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("web/src/lib/botlook.fixtures.json is not here")
	}
	if err != nil {
		t.Fatal(err)
	}
	var fs []fixture
	if err := json.Unmarshal(raw, &fs); err != nil {
		t.Fatal(err)
	}
	if len(fs) == 0 {
		t.Fatal("no fixtures")
	}
	return fs
}

func TestRenderSVGMatchesFixtures(t *testing.T) {
	for i, f := range fixtures(t) {
		l, err := ParseLook(f.Look)
		if err != nil {
			t.Fatalf("fixture %d: ParseLook: %v", i, err)
		}
		if got := RenderSVG(l); got != f.SVG {
			t.Fatalf("fixture %d: svg differs\n got: %s\nwant: %s", i, got, f.SVG)
		}
		var rawLook Look
		if err := json.Unmarshal(f.Look, &rawLook); err != nil {
			t.Fatal(err)
		}
		if got := RenderSVG(rawLook); got != f.SVG {
			t.Fatalf("fixture %d: svg of the unparsed look differs\n got: %s\nwant: %s", i, got, f.SVG)
		}
		if err := rawLook.Validate(); err != nil {
			t.Fatalf("fixture %d: Validate: %v", i, err)
		}
	}
}

func lookJSON(t *testing.T, edit func(m map[string]any)) []byte {
	t.Helper()
	b, _ := json.Marshal(OllieLook)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	edit(m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestParseLook(t *testing.T) {
	l, err := ParseLook(lookJSON(t, func(map[string]any) {}))
	if err != nil || l != OllieLook {
		t.Fatalf("Ollie: %+v %v", l, err)
	}
	l, err = ParseLook(lookJSON(t, func(m map[string]any) {
		m["accent"] = "#ABCDEF"
		m["fill"] = map[string]any{"type": "solid", "color": "#FFFFFF", "to": "#CcCcCc"}
	}))
	if err != nil || l.Accent != "#abcdef" || l.Fill.Color != "#ffffff" || l.Fill.To != "#cccccc" {
		t.Fatalf("lowercases colours: %+v %v", l, err)
	}

	cases := []struct {
		name string
		raw  []byte
		want string
	}{
		{"not an object", []byte(`[1]`), "look must be an object"},
		{"null", []byte(`null`), "look must be an object"},
		{"unknown key", lookJSON(t, func(m map[string]any) { m["svg"] = "<svg/>" }), "look.svg is not a look setting"},
		{"bad shape", lookJSON(t, func(m map[string]any) { m["shape"] = "star" }), "look.shape is not a shape"},
		{"bad accent", lookJSON(t, func(m map[string]any) { m["accent"] = "red" }), "look.accent must be a #rrggbb colour"},
		{"url colour", lookJSON(t, func(m map[string]any) {
			m["fill"] = map[string]any{"type": "solid", "color": "url(#x)", "to": "#000000"}
		}), "look.fill.color must be a #rrggbb colour"},
		{"empty emoji", lookJSON(t, func(m map[string]any) { m["kind"] = "emoji"; m["emoji"] = "" }), "look.emoji is required for an emoji look"},
		{"missing key", lookJSON(t, func(m map[string]any) { delete(m, "blush") }), "look.blush is required"},
		{"blush not a bool", lookJSON(t, func(m map[string]any) { m["blush"] = nil }), "look.blush must be true or false"},
		{"nested unknown key", lookJSON(t, func(m map[string]any) {
			m["fill"] = map[string]any{"type": "solid", "color": "#000000", "to": "#000000", "href": "x"}
		}), "look.fill.href is not a fill setting"},
		{"fill not an object", lookJSON(t, func(m map[string]any) { m["fill"] = "#000000" }), "look.fill must be an object"},
		{"markup emoji", lookJSON(t, func(m map[string]any) { m["kind"] = "emoji"; m["emoji"] = "<script>" }), "look.emoji must be one emoji"},
		{"emoji not a string", lookJSON(t, func(m map[string]any) { m["emoji"] = 1 }), "look.emoji must be one emoji"},
		{"bad kind", lookJSON(t, func(m map[string]any) { m["kind"] = "svg" }), "look.kind must be built, emoji or image"},
		{"trailing data", append(lookJSON(t, func(map[string]any) {}), []byte(` {}`)...), "look must be an object"},
	}
	for _, c := range cases {
		_, err := ParseLook(c.raw)
		var he *httpx.Error
		if !errors.As(err, &he) || he.Status != 422 || he.Code != "validation_failed" || he.Detail != c.want {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}

	// The first unknown key is named in the order JavaScript reports keys.
	_, err = ParseLook([]byte(`{"b":1,"a":1,"7":1}`))
	if err == nil || err.Error() != "look.7 is not a look setting" {
		t.Errorf("key order: %v", err)
	}

	l, err = ParseLook(lookJSON(t, func(m map[string]any) { m["kind"] = "emoji"; m["emoji"] = "👩🏽‍🚀" }))
	if err != nil || l.Emoji != "👩🏽‍🚀" {
		t.Fatalf("emoji look: %+v %v", l, err)
	}
}

func TestValidate(t *testing.T) {
	if err := OllieLook.Validate(); err != nil {
		t.Fatal(err)
	}
	l := OllieLook
	l.Shape = "star"
	if err := l.Validate(); err == nil || err.Error() != "look.shape is not a shape" {
		t.Errorf("shape: %v", err)
	}
	l = OllieLook
	l.Kind = "emoji"
	if err := l.Validate(); err == nil || err.Error() != "look.emoji is required for an emoji look" {
		t.Errorf("emoji: %v", err)
	}
}

func TestIsEmoji(t *testing.T) {
	for s, want := range map[string]bool{
		"🦊": true, "👩🏽‍🚀": true, "1️⃣": true, "❤️": true,
		"": false, "hi": false, "<b>": false, "1": false, "🦊🦊🦊🦊🦊🦊🦊🦊🦊🦊🦊": false,
	} {
		if IsEmoji(s) != want {
			t.Errorf("IsEmoji(%q) = %v", s, !want)
		}
	}
}

func TestInk(t *testing.T) {
	if got := Ink(Fill{Type: "solid", Color: "#ffffff", To: "#ffffff"}); got != "#2a2838" {
		t.Errorf("white: %s", got)
	}
	if got := Ink(Fill{Type: "solid", Color: "#2b2a35", To: "#2b2a35"}); got != "#ffffff" {
		t.Errorf("ink: %s", got)
	}
	if got := Ink(OllieLook.Fill); got != "#ffffff" {
		t.Errorf("Ollie: %s", got)
	}
}

func TestHash(t *testing.T) {
	h := OllieLook.Hash()
	if len(h) != 8 || strings.Trim(h, "0123456789abcdef") != "" {
		t.Fatalf("hash %q", h)
	}
	l := OllieLook
	l.Blush = true
	if l.Hash() == h || OllieLook.Hash() != h {
		t.Fatal("hash is not a stable digest of the look")
	}
}

func TestPartsParse(t *testing.T) {
	if _, err := paths(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"M1 2A3 4 5 6 7 8 9", "m1 2", "M1", "L1 2 3", "M1 2Z3"} {
		if _, err := parsePath(bad); err == nil {
			t.Errorf("parsePath(%q) accepted", bad)
		}
	}
}

func decode(t *testing.T, b []byte, size int) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != size || img.Bounds().Dy() != size {
		t.Fatalf("size %v, want %d", img.Bounds(), size)
	}
	return img
}

func TestRenderPNG(t *testing.T) {
	for _, size := range PNGSizes {
		b, err := RenderPNG(OllieLook, size, "O")
		if err != nil {
			t.Fatal(err)
		}
		img := decode(t, b, size)
		r, g, bl, a := img.At(size/2, size/2+size/10).RGBA()
		if a != 0xffff || !(bl > r && r > g) {
			t.Errorf("%d: centre is not violet: %d %d %d %d", size, r>>8, g>>8, bl>>8, a>>8)
		}
		if _, _, _, a := img.At(1, 1).RGBA(); a != 0 {
			t.Errorf("%d: corner is not transparent", size)
		}
	}

	emoji := OllieLook
	emoji.Kind, emoji.Emoji, emoji.Shape, emoji.Background = "emoji", "🦊", "circle", "soft"
	b, err := RenderPNG(emoji, 128, "fox")
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, b, 128)
	// The letter is drawn in the ink (white on Ollie's fill) somewhere near the centre.
	white := 0
	for y := 44; y < 84; y++ {
		for x := 44; x < 84; x++ {
			if r, g, b, _ := img.At(x, y).RGBA(); r>>8 > 240 && g>>8 > 240 && b>>8 > 240 {
				white++
			}
		}
	}
	if white < 100 {
		t.Errorf("emoji look has no initial (%d white pixels)", white)
	}
	if _, err := RenderPNG(emoji, 64, ""); err != nil {
		t.Fatal(err)
	}

	halo := OllieLook
	halo.Accessory, halo.Background, halo.Mouth, halo.Blush = "halo", "ring", "smile", true
	b, err = RenderPNG(halo, 256, "")
	if err != nil {
		t.Fatal(err)
	}
	img = decode(t, b, 256)
	// The ring reaches the left edge's middle.
	if _, _, _, a := img.At(2, 128).RGBA(); a == 0 {
		t.Error("ring is not drawn")
	}

	for _, size := range []int{0, 32, 100, 512} {
		if _, err := RenderPNG(OllieLook, size, ""); err == nil {
			t.Errorf("size %d accepted", size)
		}
	}
}

func TestRenderPNGFixtures(t *testing.T) {
	for i, f := range fixtures(t) {
		l, err := ParseLook(f.Look)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := RenderPNG(l, 64, "A"); err != nil {
			t.Fatalf("fixture %d: %v", i, err)
		}
	}
}
