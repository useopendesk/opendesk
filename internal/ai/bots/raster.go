package bots

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// RenderPNG rasterizes the same geometry RenderSVG draws. There is no emoji font on
// the server, so an emoji look shows initial (the bot name's first letter) on its
// shape instead; "" draws the shape alone.

// PNGSizes are the sizes RenderPNG draws, in pixels.
var PNGSizes = []int{64, 128, 256}

// initialSize is the letter's font size in face units (the SVG emoji is 31).
const initialSize = 30

type seg struct {
	op  byte // 'M', 'L', 'Q', 'C' or 'Z'
	pts [3][2]float32
}

// parsePath reads the path data the parts table uses: absolute M, L, Q, C and Z
// with space separated numbers. Anything else is refused.
func parsePath(d string) ([]seg, error) {
	var out []seg
	var op byte
	var nums []float32
	arity := map[byte]int{'M': 2, 'L': 2, 'Q': 4, 'C': 6, 'Z': 0}
	flush := func() error {
		if op == 0 {
			if len(nums) > 0 {
				return errors.New("path data before a command")
			}
			return nil
		}
		n := arity[op]
		if n == 0 {
			if len(nums) > 0 {
				return errors.New("numbers after Z")
			}
			out = append(out, seg{op: 'Z'})
			return nil
		}
		if len(nums) == 0 || len(nums)%n != 0 {
			return fmt.Errorf("%c takes %d numbers, got %d", op, n, len(nums))
		}
		for i := 0; i < len(nums); i += n {
			s := seg{op: op}
			if op == 'M' && i > 0 {
				s.op = 'L' // further pairs after M are line segments
			}
			for j := 0; j < n/2; j++ {
				s.pts[j] = [2]float32{nums[i+2*j], nums[i+2*j+1]}
			}
			out = append(out, s)
		}
		return nil
	}
	for i := 0; i < len(d); {
		c := d[i]
		switch {
		case c == ' ':
			i++
		case c == 'M' || c == 'L' || c == 'Q' || c == 'C' || c == 'Z':
			if err := flush(); err != nil {
				return nil, err
			}
			op, nums = c, nums[:0]
			i++
		case c == '-' || c == '.' || (c >= '0' && c <= '9'):
			j := i + 1
			for j < len(d) && (d[j] == '.' || (d[j] >= '0' && d[j] <= '9')) {
				j++
			}
			v, err := strconv.ParseFloat(d[i:j], 32)
			if err != nil {
				return nil, fmt.Errorf("bad number %q", d[i:j])
			}
			nums = append(nums, float32(v))
			i = j
		default:
			return nil, fmt.Errorf("unsupported path data %q", c)
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return out, nil
}

// paths is every part parsed once.
var paths = sync.OnceValues(func() (map[string][]seg, error) {
	m := make(map[string][]seg, len(parts))
	for k, d := range parts {
		p, err := parsePath(d)
		if err != nil {
			return nil, fmt.Errorf("bots: part %s: %w", k, err)
		}
		m[k] = p
	}
	return m, nil
})

var goBold = sync.OnceValues(func() (*opentype.Font, error) { return opentype.Parse(gobold.TTF) })

// xform maps face coordinates to pixels: (a*x+e)*k, (a*y+f)*k.
type xform struct{ a, e, f, k float32 }

func (t xform) at(p [2]float32) (float32, float32) {
	return (t.a*p[0] + t.e) * t.k, (t.a*p[1] + t.f) * t.k
}

func (t xform) path(z *vector.Rasterizer, segs []seg) {
	open := false
	for _, s := range segs {
		if s.op == 'M' && open {
			z.ClosePath() // a fill closes every subpath
		}
		open = s.op != 'Z' && (open || s.op == 'M')
		switch s.op {
		case 'M':
			z.MoveTo(t.at(s.pts[0]))
		case 'L':
			z.LineTo(t.at(s.pts[0]))
		case 'Q':
			x1, y1 := t.at(s.pts[0])
			x2, y2 := t.at(s.pts[1])
			z.QuadTo(x1, y1, x2, y2)
		case 'C':
			x1, y1 := t.at(s.pts[0])
			x2, y2 := t.at(s.pts[1])
			x3, y3 := t.at(s.pts[2])
			z.CubeTo(x1, y1, x2, y2, x3, y3)
		case 'Z':
			z.ClosePath()
		}
	}
	if open {
		z.ClosePath()
	}
}

func hexColor(hex string, opacity float64) color.NRGBA {
	r, g, b := rgb(hex)
	if len(hex) == 4 { // #fff and #000, the renderer's own shorthands
		n, _ := strconv.ParseUint(hex[1:], 16, 16)
		r, g, b = int(n>>8&15)*17, int(n>>4&15)*17, int(n&15)*17
	}
	return color.NRGBA{uint8(r), uint8(g), uint8(b), uint8(math.Round(opacity * 255))}
}

func opacityOf(s string) float64 {
	if s == "" {
		return 1
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 1
	}
	return min(max(v, 0), 1)
}

// radial is the body gradient as an image: centre (22,16), radius 50 in face
// coordinates, from c0 to c1 in sRGB, clamped past the radius.
type radial struct {
	t      xform
	c0, c1 [3]float64
}

func (g radial) ColorModel() color.Model { return color.RGBAModel }
func (g radial) Bounds() image.Rectangle {
	return image.Rect(-1e9, -1e9, 1e9, 1e9)
}
func (g radial) At(x, y int) color.Color {
	fx := (float64(x)+.5)/float64(g.t.k) - float64(g.t.e)
	fy := (float64(y)+.5)/float64(g.t.k) - float64(g.t.f)
	lx, ly := fx/float64(g.t.a), fy/float64(g.t.a)
	u := min(math.Hypot(lx-22, ly-16)/50, 1)
	var c [3]uint8
	for i := range c {
		c[i] = uint8(math.Round(g.c0[i] + (g.c1[i]-g.c0[i])*u))
	}
	return color.RGBA{c[0], c[1], c[2], 255}
}

// RenderPNG draws the look as a size x size PNG (size is 64, 128 or 256). An image
// look draws as built (callers redirect to the uploaded picture).
func RenderPNG(l Look, size int, initial string) ([]byte, error) {
	switch size {
	case 64, 128, 256:
	default:
		return nil, fmt.Errorf("bots: png size %d is not 64, 128 or 256", size)
	}
	ps, err := paths()
	if err != nil {
		return nil, err
	}
	s := newScene(l)
	k := float32(size) / 64
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	z := vector.NewRasterizer(size, size)
	fill := func(t xform, d, hex, opacity string) { // d is a part name
		segs := ps[d]
		if len(segs) == 0 {
			return
		}
		z.Reset(size, size)
		t.path(z, segs)
		z.Draw(dst, dst.Bounds(), image.NewUniform(hexColor(hex, opacityOf(opacity))), image.Point{})
	}
	plain := xform{1, 0, 0, k}
	face := xform{float32(s.matrix[0]), float32(s.matrix[1]), float32(s.matrix[2]), k}

	if s.bg != nil {
		fill(plain, s.bg.part, s.bg.fill, s.bg.opacity)
	}
	if s.gradient {
		if segs := ps[s.body]; len(segs) > 0 {
			r0, g0, b0 := rgb(s.color)
			r1, g1, b1 := rgb(s.to)
			z.Reset(size, size)
			face.path(z, segs)
			z.Draw(dst, dst.Bounds(), radial{face, [3]float64{float64(r0), float64(g0), float64(b0)}, [3]float64{float64(r1), float64(g1), float64(b1)}}, image.Point{})
		}
	} else {
		fill(face, s.body, s.color, "")
	}
	if s.emoji {
		if err := drawInitial(z, dst, face, s.ink, initial); err != nil {
			return nil, err
		}
	} else {
		for _, p := range s.features {
			fill(face, p.part, p.fill, p.opacity)
		}
		for _, grp := range [][]layer{s.eyes, s.mouth, s.acc} {
			for _, p := range grp {
				c := p.fill
				if c == "" {
					c = s.ink
				}
				fill(face, p.part, c, p.opacity)
			}
		}
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// drawInitial draws the first letter of initial, uppercased, in Go Bold, its ink box
// centred on the face.
func drawInitial(z *vector.Rasterizer, dst *image.RGBA, t xform, ink, initial string) error {
	r, _ := utf8.DecodeRuneInString(initial)
	if initial == "" || r == utf8.RuneError || !unicode.IsGraphic(r) || unicode.IsSpace(r) {
		return nil
	}
	r = unicode.ToUpper(r)
	f, err := goBold()
	if err != nil {
		return err
	}
	var b sfnt.Buffer
	gi, err := f.GlyphIndex(&b, r)
	if err != nil || gi == 0 {
		return nil // not in the font: the shape alone
	}
	upem := fixed.Int26_6(f.UnitsPerEm()) << 6 // load in font units (26.6)
	bounds, _, err := f.GlyphBounds(&b, gi, upem, font.HintingNone)
	if err != nil {
		return nil
	}
	segs, err := f.LoadGlyph(&b, gi, upem, nil)
	if err != nil || len(segs) == 0 {
		return nil
	}
	scale := float32(initialSize) / float32(f.UnitsPerEm()) / 64
	cx := float32(bounds.Min.X+bounds.Max.X) / 2
	cy := float32(bounds.Min.Y+bounds.Max.Y) / 2
	at := func(p fixed.Point26_6) (float32, float32) {
		return t.at([2]float32{32 + (float32(p.X)-cx)*scale, 32 + (float32(p.Y)-cy)*scale})
	}
	z.Reset(dst.Bounds().Dx(), dst.Bounds().Dy())
	for i, s := range segs {
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			if i > 0 {
				z.ClosePath()
			}
			z.MoveTo(at(s.Args[0]))
		case sfnt.SegmentOpLineTo:
			z.LineTo(at(s.Args[0]))
		case sfnt.SegmentOpQuadTo:
			x1, y1 := at(s.Args[0])
			x2, y2 := at(s.Args[1])
			z.QuadTo(x1, y1, x2, y2)
		case sfnt.SegmentOpCubeTo:
			x1, y1 := at(s.Args[0])
			x2, y2 := at(s.Args[1])
			x3, y3 := at(s.Args[2])
			z.CubeTo(x1, y1, x2, y2, x3, y3)
		}
	}
	z.ClosePath()
	z.Draw(dst, dst.Bounds(), image.NewUniform(hexColor(ink, 1)), image.Point{})
	return nil
}
