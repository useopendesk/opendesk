// Package templates is the one template language of customer-facing messages
// (contracts/templates.md): a Liquid-style language with variables, filters,
// if/elsif/else, unless, case/when, for loops over lists, assign and capture,
// a shared list of variables, the design allowlist and the email layout.
// Filling is a single pass: a value is never read again as a template.
package templates

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Mode is how values are written: escaped HTML, or plain text (subjects).
type Mode int

const (
	HTML Mode = iota
	Text
)

// Problem is one thing wrong with a template, for the editor.
type Problem struct {
	Message string `json:"message"`
	Token   string `json:"token"`
}

// Limits keep any template cheap to fill.
const (
	maxTag        = 400
	maxDepth      = 24
	maxLoop       = 100  // items one loop goes over
	maxIterations = 1000 // loop bodies one fill runs
	maxOutput     = 512 << 10
)

var (
	pathRe   = regexp.MustCompile(`^[a-z_][a-z0-9_]*(\.[a-z0-9_]+)*$`)
	markupRe = regexp.MustCompile(`<[a-zA-Z/!]`)
)

type token struct {
	kind  byte // 't' text, 'v' {{ }}, 'g' {% %}
	raw   string
	inner string
	pos   int
}

// lex splits src into text, {{ output }} and {% tag %} tokens. A tag that is not closed,
// is too long or spans markup stays text.
func lex(src string) []token {
	var out []token
	offset := 0
	text := func(s string) {
		if s == "" {
			return
		}
		if n := len(out); n > 0 && out[n-1].kind == 't' {
			out[n-1].raw += s
			return
		}
		out = append(out, token{kind: 't', raw: s, pos: offset})
	}
	advance := func(n int) {
		src = src[n:]
		offset += n
	}
	for {
		i := strings.Index(src, "{")
		if i < 0 || i+1 >= len(src) {
			text(src)
			return out
		}
		open := src[i : i+2]
		if open != "{{" && open != "{%" {
			text(src[:i+1])
			advance(i + 1)
			continue
		}
		closer := "}}"
		if open == "{%" {
			closer = "%}"
		}
		j := strings.Index(src[i+2:], closer)
		if j < 0 || j > maxTag || strings.Contains(src[i+2:i+2+j], "{") || markupRe.MatchString(src[i+2:i+2+j]) {
			text(src[:i+2])
			advance(i + 2)
			continue
		}
		text(src[:i])
		raw := src[i : i+2+j+2]
		kind := byte('v')
		if open == "{%" {
			kind = 'g'
		}
		out = append(out, token{kind: kind, raw: raw, inner: strings.TrimSpace(cleanInner(src[i+2 : i+2+j])), pos: offset + i})
		advance(i + 2 + j + 2)
	}
}

// cleanInner reads a tag's inside as typed: entities decoded (editors write &quot;,
// &gt; and &nbsp;), every space one space.
func cleanInner(s string) string {
	s = html.UnescapeString(s)
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}

type node interface{}

type textNode string

// outNode is {{ expression | filters }}.
type outNode struct {
	raw  string
	expr pipeline
	free []string // the variables it reads that are not local
}

type branch struct {
	cond cond
	body []node
}

// ifNode is if/elsif/else and unless.
type ifNode struct {
	src      string
	branches []branch
	els      []node
	free     []string
}

type when struct {
	values []operand
	body   []node
}

type caseNode struct {
	src     string
	subject operand
	whens   []when
	els     []node
	free    []string
}

type forNode struct {
	src      string
	name     string
	source   operand
	limit    int
	offset   int
	reversed bool
	body     []node
	els      []node
	free     []string
}

type assignNode struct {
	name string
	expr pipeline
}

type captureNode struct {
	name string
	body []node
}

// Template is a parsed template.
type Template struct {
	nodes    []node
	problems []Problem
	keys     []string
}

// Parse reads src. It never fails: what does not parse is kept as text and reported in
// Problems.
func Parse(src string) *Template {
	p := &parser{t: &Template{}, toks: lex(src), src: src, globals: map[string]bool{}}
	nodes, end := p.nodes()
	if end != nil {
		// nodes() returns only at a stop word or the end; at the top there are none.
		nodes = append(nodes, textNode(end.raw))
	}
	p.t.nodes = nodes
	return p.t
}

type parser struct {
	t       *Template
	toks    []token
	i       int
	src     string
	scopes  []map[string]bool
	globals map[string]bool
	depth   int
}

func (p *parser) problem(msg, tok string) {
	p.t.problems = append(p.t.problems, Problem{Message: msg, Token: tok})
}

func (p *parser) local(key string) bool {
	root, _, _ := strings.Cut(key, ".")
	if p.globals[root] {
		return true
	}
	for _, s := range p.scopes {
		if s[root] {
			return true
		}
	}
	return false
}

// read records the variables an expression reads and returns the free ones.
func (p *parser) read(keys []string) []string {
	var free []string
	for _, k := range keys {
		if p.local(k) {
			continue
		}
		p.t.keys = append(p.t.keys, k)
		free = append(free, k)
	}
	return free
}

func word(inner string) (string, string) {
	w, rest, _ := strings.Cut(inner, " ")
	return w, strings.TrimSpace(rest)
}

// nodes parses until one of the stop words (returned) or the end (nil).
func (p *parser) nodes(stops ...string) ([]node, *token) {
	var out []node
	for p.i < len(p.toks) {
		tk := p.toks[p.i]
		p.i++
		switch tk.kind {
		case 't':
			out = append(out, textNode(tk.raw))
		case 'v':
			pl, err := parsePipeline(tk.inner)
			if err != "" {
				p.problem(err, tk.raw)
				out = append(out, textNode(tk.raw))
				continue
			}
			out = append(out, &outNode{raw: tk.raw, expr: pl, free: p.read(pl.keys())})
		case 'g':
			w, rest := word(tk.inner)
			for _, s := range stops {
				if s == w {
					return out, &tk
				}
			}
			out = append(out, p.tag(tk, w, rest)...)
		}
	}
	return out, nil
}

// unclosed reports a block without its end and keeps what it held, as written.
func (p *parser) unclosed(tk token, end string, parts ...[]node) []node {
	p.problem(tk.raw+" is missing {% "+end+" %}.", tk.raw)
	out := []node{textNode(tk.raw)}
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func (p *parser) span(start token, end *token) string { return p.src[start.pos : end.pos+len(end.raw)] }

func (p *parser) tag(tk token, w, rest string) []node {
	text := []node{textNode(tk.raw)}
	if p.depth >= maxDepth {
		p.problem("Blocks are nested too deeply.", tk.raw)
		return text
	}
	switch w {
	case "if", "unless":
		c, err := parseCond(rest)
		if err != "" {
			p.problem(err, tk.raw)
			return text
		}
		if w == "unless" {
			inner := c
			c = cond{not: true, inner: &inner}
		}
		n := &ifNode{}
		free := p.read(c.keys())
		endWord := "end" + w
		stops := []string{"elsif", "else", endWord}
		if w == "unless" {
			stops = []string{"else", endWord}
		}
		p.depth++
		defer func() { p.depth-- }()
		var seen [][]node
		for {
			body, end := p.nodes(stops...)
			seen = append(seen, body)
			if end == nil {
				return p.unclosed(tk, endWord, seen...)
			}
			n.branches = append(n.branches, branch{cond: c, body: body})
			ew, erest := word(end.inner)
			switch ew {
			case "elsif":
				if c, err = parseCond(erest); err != "" {
					p.problem(err, end.raw)
					c = cond{lit: &operand{str: "", isLit: true}}
				}
				free = append(free, p.read(c.keys())...)
				continue
			case "else":
				els, last := p.nodes(endWord)
				if last == nil {
					return p.unclosed(tk, endWord, append(seen, els)...)
				}
				n.els, n.src, n.free = els, p.span(tk, last), free
				return []node{n}
			}
			n.src, n.free = p.span(tk, end), free
			return []node{n}
		}
	case "case":
		subj, err := parseOperand(rest)
		if err != "" {
			p.problem(err, tk.raw)
			return text
		}
		n := &caseNode{subject: subj, free: p.read(subj.keys())}
		p.depth++
		defer func() { p.depth-- }()
		_, end := p.nodes("when", "else", "endcase") // anything before the first when is ignored
		for end != nil {
			ew, erest := word(end.inner)
			switch ew {
			case "when":
				vals, err := parseOperands(erest)
				if err != "" {
					p.problem(err, end.raw)
				}
				for _, v := range vals {
					n.free = append(n.free, p.read(v.keys())...)
				}
				var body []node
				body, end = p.nodes("when", "else", "endcase")
				n.whens = append(n.whens, when{values: vals, body: body})
				continue
			case "else":
				var last *token
				n.els, last = p.nodes("endcase")
				if last == nil {
					return p.unclosed(tk, "endcase")
				}
				n.src = p.span(tk, last)
				return []node{n}
			}
			n.src = p.span(tk, end)
			return []node{n}
		}
		return p.unclosed(tk, "endcase")
	case "for":
		n, err := parseFor(rest)
		if err != "" {
			p.problem(err, tk.raw)
			return text
		}
		n.free = p.read(n.source.keys())
		p.depth++
		p.scopes = append(p.scopes, map[string]bool{n.name: true, "forloop": true})
		defer func() {
			p.depth--
			p.scopes = p.scopes[:len(p.scopes)-1]
		}()
		body, end := p.nodes("else", "endfor")
		if end == nil {
			return p.unclosed(tk, "endfor", body)
		}
		n.body = body
		if ew, _ := word(end.inner); ew == "else" {
			els, last := p.nodes("endfor")
			if last == nil {
				return p.unclosed(tk, "endfor", body, els)
			}
			n.els, end = els, last
		}
		n.src = p.span(tk, end)
		return []node{n}
	case "assign":
		name, expr, ok := strings.Cut(rest, "=")
		name = strings.TrimSpace(name)
		if !ok || !identRe.MatchString(name) {
			p.problem("Write it like {% assign greeting = contact.first_name | default: \"there\" %}.", tk.raw)
			return text
		}
		pl, err := parsePipeline(strings.TrimSpace(expr))
		if err != "" {
			p.problem(err, tk.raw)
			return text
		}
		p.read(pl.keys())
		p.globals[name] = true
		return []node{&assignNode{name: name, expr: pl}}
	case "capture":
		if !identRe.MatchString(rest) {
			p.problem("Write it like {% capture greeting %}…{% endcapture %}.", tk.raw)
			return text
		}
		p.depth++
		body, end := p.nodes("endcapture")
		p.depth--
		if end == nil {
			return p.unclosed(tk, "endcapture", body)
		}
		p.globals[rest] = true
		return []node{&captureNode{name: rest, body: body}}
	case "raw":
		var b strings.Builder
		for p.i < len(p.toks) {
			t := p.toks[p.i]
			p.i++
			if t.kind == 'g' && t.inner == "endraw" {
				return []node{textNode(b.String())}
			}
			b.WriteString(t.raw)
		}
		p.problem("{% raw %} is missing {% endraw %}.", tk.raw)
		return []node{textNode(tk.raw + b.String())}
	case "comment":
		for p.i < len(p.toks) {
			t := p.toks[p.i]
			p.i++
			if t.kind == 'g' && t.inner == "endcomment" {
				return nil
			}
		}
		p.problem("{% comment %} is missing {% endcomment %}.", tk.raw)
		return text
	case "elsif", "else", "endif", "endunless", "endcase", "endfor", "when", "endcapture", "endraw", "endcomment":
		p.problem("{% "+w+" %} has no matching opening tag.", tk.raw)
		return text
	}
	if hint := didYouMean(w, tagWords); hint != "" {
		p.problem("There is no tag called "+w+"."+hint, tk.raw)
	} else {
		p.problem("There is no tag called "+w+". Use if, elsif, else, unless, case, when, for, assign, capture, raw or comment.", tk.raw)
	}
	return text
}

var identRe = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

func parseFor(rest string) (*forNode, string) {
	const how = "Write it like {% for label in ticket.labels %}…{% endfor %}."
	name, src, ok := strings.Cut(rest, " in ")
	name = strings.TrimSpace(name)
	if !ok || !identRe.MatchString(name) {
		return nil, how
	}
	toks, err := tokenize(src)
	if err != "" || len(toks) == 0 {
		return nil, how
	}
	n := &forNode{name: name}
	var srcToks []etok
	i := 0
	for ; i < len(toks); i++ {
		t := toks[i]
		if t.kind == 'i' && (t.s == "limit" || t.s == "offset" || t.s == "reversed") {
			break
		}
		srcToks = append(srcToks, t)
	}
	op, e := operandFrom(srcToks)
	if e != "" {
		return nil, how
	}
	n.source = op
	for i < len(toks) {
		t := toks[i]
		switch {
		case t.kind == 'i' && t.s == "reversed":
			n.reversed = true
			i++
		case t.kind == 'i' && (t.s == "limit" || t.s == "offset") && i+2 < len(toks) && toks[i+1].kind == ':' && toks[i+2].kind == 'n':
			v, _ := strconv.Atoi(toks[i+2].s)
			if t.s == "limit" {
				n.limit = v
			} else {
				n.offset = v
			}
			i += 3
		case t.kind == ',':
			i++
		default:
			return nil, "After the list, a for loop takes limit: n, offset: n and reversed."
		}
	}
	return n, ""
}

// --- Rendering ---

// Values are what a template is filled with. Known reports whether a key exists at all:
// a known key without a value fills empty, an unknown one stays as written.
type Values struct {
	Text  map[string]string
	HTML  map[string]string   // sanitized HTML: agent.signature and slots
	Lists map[string][]string // labels and multi-select fields
	Known func(key string) bool
	// Location is the workspace's timezone, for the date filter.
	Location *time.Location
}

type value struct {
	s      string
	list   []string
	isList bool
	html   bool
}

func textValue(s string) value { return value{s: s} }

func (v value) str() string {
	if v.isList {
		return strings.Join(v.list, ", ")
	}
	if v.html {
		return TextOf(v.s)
	}
	return v.s
}

func (v value) empty() bool {
	if v.isList {
		return len(v.list) == 0
	}
	if v.html {
		return strings.TrimSpace(TextOf(v.s)) == "" && !strings.Contains(v.s, "<img")
	}
	return strings.TrimSpace(v.s) == ""
}

func (v *Values) knows(key string) bool {
	if v == nil {
		return false
	}
	if _, ok := v.HTML[key]; ok {
		return true
	}
	if _, ok := v.Lists[key]; ok {
		return true
	}
	if _, ok := v.Text[key]; ok {
		return true
	}
	return v.Known != nil && v.Known(key)
}

func (v *Values) get(key string) value {
	if v == nil {
		return value{}
	}
	if s, ok := v.HTML[key]; ok {
		return value{s: s, html: true}
	}
	if l, ok := v.Lists[key]; ok {
		return value{list: l, isList: true}
	}
	return textValue(v.Text[key])
}

type renderer struct {
	v      *Values
	mode   Mode
	b      strings.Builder
	scopes []map[string]value
	vars   map[string]value
	iters  int
}

func (r *renderer) lookup(key string) (value, bool) {
	for i := len(r.scopes) - 1; i >= 0; i-- {
		if val, ok := r.scopes[i][key]; ok {
			return val, true
		}
	}
	if val, ok := r.vars[key]; ok {
		return val, true
	}
	if r.v.knows(key) {
		return r.v.get(key), true
	}
	return value{}, false
}

func (r *renderer) allKnown(keys []string) bool {
	for _, k := range keys {
		if !r.v.knows(k) {
			return false
		}
	}
	return true
}

func (r *renderer) write(s string) {
	if r.b.Len()+len(s) > maxOutput {
		return
	}
	r.b.WriteString(s)
}

// Render fills the template.
func (t *Template) Render(v *Values, mode Mode) string {
	r := &renderer{v: v, mode: mode, vars: map[string]value{}}
	r.nodes(t.nodes)
	return r.b.String()
}

// Render parses and fills src.
func Render(src string, v *Values, mode Mode) string {
	if !HasTags(src) {
		return src
	}
	return Parse(src).Render(v, mode)
}

func (r *renderer) out(v value) string {
	switch {
	case v.html && r.mode == HTML:
		return v.s
	case r.mode == HTML:
		return html.EscapeString(v.str())
	}
	return v.str()
}

func (r *renderer) nodes(nodes []node) {
	for _, n := range nodes {
		switch n := n.(type) {
		case textNode:
			r.write(string(n))
		case *outNode:
			if !r.allKnown(n.free) {
				r.write(n.raw)
				continue
			}
			r.write(r.out(r.pipeline(n.expr)))
		case *ifNode:
			if !r.allKnown(n.free) {
				r.write(n.src)
				continue
			}
			done := false
			for _, br := range n.branches {
				if r.cond(br.cond) {
					r.nodes(br.body)
					done = true
					break
				}
			}
			if !done {
				r.nodes(n.els)
			}
		case *caseNode:
			if !r.allKnown(n.free) {
				r.write(n.src)
				continue
			}
			subj := r.operand(n.subject)
			done := false
			for _, w := range n.whens {
				for _, val := range w.values {
					if equal(subj, r.operand(val)) {
						r.nodes(w.body)
						done = true
						break
					}
				}
				if done {
					break
				}
			}
			if !done {
				r.nodes(n.els)
			}
		case *forNode:
			if !r.allKnown(n.free) {
				r.write(n.src)
				continue
			}
			r.loop(n)
		case *assignNode:
			r.vars[n.name] = r.pipeline(n.expr)
		case *captureNode:
			sub := &renderer{v: r.v, mode: r.mode, scopes: r.scopes, vars: r.vars, iters: r.iters}
			sub.nodes(n.body)
			r.iters = sub.iters
			if r.mode == HTML {
				r.vars[n.name] = value{s: sub.b.String(), html: true}
			} else {
				r.vars[n.name] = textValue(sub.b.String())
			}
		}
	}
}

func (r *renderer) loop(n *forNode) {
	src := r.operand(n.source)
	items := src.list
	if !src.isList {
		if s := strings.TrimSpace(src.str()); s != "" {
			items = []string{s}
		}
	}
	if n.offset > 0 {
		if n.offset >= len(items) {
			items = nil
		} else {
			items = items[n.offset:]
		}
	}
	if n.limit > 0 && n.limit < len(items) {
		items = items[:n.limit]
	}
	if len(items) > maxLoop {
		items = items[:maxLoop]
	}
	if n.reversed {
		rev := make([]string, len(items))
		for i, it := range items {
			rev[len(items)-1-i] = it
		}
		items = rev
	}
	if len(items) == 0 {
		r.nodes(n.els)
		return
	}
	for i, it := range items {
		if r.iters >= maxIterations {
			return
		}
		r.iters++
		yes := func(b bool) value {
			if b {
				return textValue("true")
			}
			return textValue("")
		}
		r.scopes = append(r.scopes, map[string]value{
			n.name:            textValue(it),
			"forloop.index":   textValue(strconv.Itoa(i + 1)),
			"forloop.index0":  textValue(strconv.Itoa(i)),
			"forloop.length":  textValue(strconv.Itoa(len(items))),
			"forloop.first":   yes(i == 0),
			"forloop.last":    yes(i == len(items)-1),
			"forloop.rindex":  textValue(strconv.Itoa(len(items) - i)),
			"forloop.rindex0": textValue(strconv.Itoa(len(items) - i - 1)),
		})
		r.nodes(n.body)
		r.scopes = r.scopes[:len(r.scopes)-1]
	}
}

func (r *renderer) operand(o operand) value {
	switch {
	case o.isLit:
		return textValue(o.str)
	case o.isRange:
		from, to := r.operandInt(*o.from), r.operandInt(*o.to)
		if to-from >= maxLoop {
			to = from + maxLoop - 1
		}
		var list []string
		for i := from; i <= to; i++ {
			list = append(list, strconv.Itoa(i))
		}
		return value{list: list, isList: true}
	}
	v, _ := r.lookup(o.path)
	return v
}

func (r *renderer) operandInt(o operand) int {
	n, _ := strconv.Atoi(strings.TrimSpace(r.operand(o).str()))
	return n
}

func (r *renderer) pipeline(p pipeline) value {
	v := r.operand(p.head)
	for _, f := range p.filters {
		args := make([]value, len(f.args))
		for i, a := range f.args {
			args[i] = r.operand(a)
		}
		v = applyFilter(f.name, v, args, r.v)
	}
	return v
}

func (r *renderer) cond(c cond) bool {
	switch {
	case c.not:
		return !r.cond(*c.inner)
	case c.and != nil:
		return r.cond(c.and[0]) && r.cond(c.and[1])
	case c.or != nil:
		return r.cond(c.or[0]) || r.cond(c.or[1])
	case c.lit != nil:
		return !r.operand(*c.lit).empty()
	}
	left := r.operand(c.left)
	if c.op == "" {
		return !left.empty() && !strings.EqualFold(strings.TrimSpace(left.str()), "false")
	}
	if c.right.isLit && (c.right.str == "\x00empty") {
		eq := left.empty()
		if c.op == "!=" {
			return !eq
		}
		return eq
	}
	right := r.operand(c.right)
	switch c.op {
	case "==":
		return equal(left, right)
	case "!=", "<>":
		return !equal(left, right)
	case "contains":
		needle := strings.ToLower(strings.TrimSpace(right.str()))
		if left.isList {
			for _, it := range left.list {
				if strings.ToLower(strings.TrimSpace(it)) == needle {
					return true
				}
			}
			return false
		}
		return needle != "" && strings.Contains(strings.ToLower(left.str()), needle)
	}
	a, aok := number(left.str())
	b, bok := number(right.str())
	if !aok || !bok {
		// Not numbers: compare as text.
		x, y := strings.ToLower(left.str()), strings.ToLower(right.str())
		switch c.op {
		case ">":
			return x > y
		case "<":
			return x < y
		case ">=":
			return x >= y
		case "<=":
			return x <= y
		}
		return false
	}
	switch c.op {
	case ">":
		return a > b
	case "<":
		return a < b
	case ">=":
		return a >= b
	case "<=":
		return a <= b
	}
	return false
}

func equal(a, b value) bool {
	if x, ok := number(a.str()); ok {
		if y, ok := number(b.str()); ok {
			return x == y
		}
	}
	return strings.EqualFold(strings.TrimSpace(a.str()), strings.TrimSpace(b.str()))
}

func number(s string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f, err == nil
}

// Problems are the syntax problems; Check adds unknown variables and missing slots.
func (t *Template) Problems() []Problem { return t.problems }

// Keys are the variables and slots the template reads (not its own assigned or loop
// names), in order, with repeats.
func (t *Template) Keys() []string { return t.keys }

// Uses reports whether the template reads key.
func (t *Template) Uses(key string) bool {
	for _, k := range t.keys {
		if k == key {
			return true
		}
	}
	return false
}

// HasTags reports whether s contains anything that looks like a template tag.
func HasTags(s string) bool { return strings.Contains(s, "{{") || strings.Contains(s, "{%") }
