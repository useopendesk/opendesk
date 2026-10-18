package templates

import (
	"strings"
)

// Expressions inside tags: operands (a variable, a quoted string, a number, a range
// like (1..3)), filter pipelines and conditions.

type etok struct {
	kind byte // 's' string, 'n' number, 'i' name, 'o' comparison, 'r' "..", or the punctuation itself: | : , ( )
	s    string
}

func tokenize(s string) ([]etok, string) {
	var out []etok
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ':
			i++
		case c == '"' || c == '\'':
			j := strings.IndexByte(s[i+1:], c)
			if j < 0 {
				return nil, "A quote is not closed."
			}
			out = append(out, etok{kind: 's', s: s[i+1 : i+1+j]})
			i += j + 2
		case c >= '0' && c <= '9' || (c == '-' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' && (len(out) == 0 || out[len(out)-1].kind != 'n' && out[len(out)-1].kind != 'i')):
			j := i + 1
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.' && !(j+1 < len(s) && s[j+1] == '.')) {
				j++
			}
			out = append(out, etok{kind: 'n', s: s[i:j]})
			i = j
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			j := i + 1
			for j < len(s) && (s[j] == '_' || s[j] == '.' && !(j+1 < len(s) && s[j+1] == '.') || s[j] >= 'a' && s[j] <= 'z' || s[j] >= 'A' && s[j] <= 'Z' || s[j] >= '0' && s[j] <= '9') {
				j++
			}
			out = append(out, etok{kind: 'i', s: s[i:j]})
			i = j
		case strings.HasPrefix(s[i:], ".."):
			out = append(out, etok{kind: 'r', s: ".."})
			i += 2
		case strings.HasPrefix(s[i:], "==") || strings.HasPrefix(s[i:], "!=") || strings.HasPrefix(s[i:], "<>") || strings.HasPrefix(s[i:], ">=") || strings.HasPrefix(s[i:], "<="):
			out = append(out, etok{kind: 'o', s: s[i : i+2]})
			i += 2
		case c == '>' || c == '<':
			out = append(out, etok{kind: 'o', s: string(c)})
			i++
		case c == '|' || c == ':' || c == ',' || c == '(' || c == ')':
			out = append(out, etok{kind: c, s: string(c)})
			i++
		default:
			return nil, "\"" + string(c) + "\" is not allowed here."
		}
	}
	return out, ""
}

// operand is one value in an expression.
type operand struct {
	path     string
	str      string
	isLit    bool
	isRange  bool
	from, to *operand
}

func (o operand) keys() []string {
	switch {
	case o.isRange:
		return append(o.from.keys(), o.to.keys()...)
	case o.isLit, o.path == "":
		return nil
	}
	return []string{o.path}
}

// emptyLit stands for the keywords empty, blank and nil.
const emptyLit = "\x00empty"

func operandFrom(toks []etok) (operand, string) {
	if len(toks) == 5 && toks[0].kind == '(' && toks[2].kind == 'r' && toks[4].kind == ')' {
		from, e1 := operandFrom(toks[1:2])
		to, e2 := operandFrom(toks[3:4])
		if e1 != "" || e2 != "" {
			return operand{}, "A range is written like (1..5)."
		}
		return operand{isRange: true, from: &from, to: &to}, ""
	}
	if len(toks) != 1 {
		return operand{}, "Expected one value here."
	}
	t := toks[0]
	switch t.kind {
	case 's', 'n':
		return operand{str: t.s, isLit: true}, ""
	case 'i':
		switch t.s {
		case "true":
			return operand{str: "true", isLit: true}, ""
		case "false":
			return operand{str: "", isLit: true}, ""
		case "empty", "blank", "nil":
			return operand{str: emptyLit, isLit: true}, ""
		}
		if !pathRe.MatchString(t.s) {
			return operand{}, "Variable names are lower case, like contact.first_name."
		}
		return operand{path: t.s}, ""
	}
	return operand{}, "Expected a variable, a quoted text or a number."
}

func parseOperand(s string) (operand, string) {
	toks, err := tokenize(s)
	if err != "" {
		return operand{}, err
	}
	return operandFrom(toks)
}

// parseOperands reads a comma (or "or") separated list, as in {% when "a", "b" %}.
func parseOperands(s string) ([]operand, string) {
	toks, err := tokenize(s)
	if err != "" {
		return nil, err
	}
	var out []operand
	var cur []etok
	flush := func() string {
		if len(cur) == 0 {
			return "Expected a value."
		}
		o, err := operandFrom(cur)
		if err != "" {
			return err
		}
		out = append(out, o)
		cur = nil
		return ""
	}
	for _, t := range toks {
		if t.kind == ',' || t.kind == 'i' && t.s == "or" {
			if err := flush(); err != "" {
				return out, err
			}
			continue
		}
		cur = append(cur, t)
	}
	if err := flush(); err != "" {
		return out, err
	}
	return out, ""
}

type filterCall struct {
	name string
	args []operand
}

// pipeline is a value and the filters it goes through.
type pipeline struct {
	head    operand
	filters []filterCall
}

func (p pipeline) keys() []string {
	k := p.head.keys()
	for _, f := range p.filters {
		for _, a := range f.args {
			k = append(k, a.keys()...)
		}
	}
	return k
}

func parsePipeline(s string) (pipeline, string) {
	toks, err := tokenize(s)
	if err != "" {
		return pipeline{}, err
	}
	var parts [][]etok
	cur := []etok{}
	for _, t := range toks {
		if t.kind == '|' {
			parts = append(parts, cur)
			cur = []etok{}
			continue
		}
		cur = append(cur, t)
	}
	parts = append(parts, cur)
	head, e := operandFrom(parts[0])
	if e != "" {
		if len(parts[0]) == 1 && parts[0][0].kind == 'i' {
			return pipeline{}, "This is not a variable name."
		}
		return pipeline{}, e
	}
	p := pipeline{head: head}
	for _, part := range parts[1:] {
		if len(part) == 0 || part[0].kind != 'i' {
			return pipeline{}, "A filter goes after |, like | default: \"there\"."
		}
		spec, ok := filterSpecs[part[0].s]
		if !ok {
			return pipeline{}, "There is no helper called " + part[0].s + "." + didYouMean(part[0].s, filterNames())
		}
		f := filterCall{name: part[0].s}
		if len(part) > 1 {
			if part[1].kind != ':' {
				return pipeline{}, "Put a colon between a filter and its value, like | truncate: 20."
			}
			args, err := splitArgs(part[2:])
			if err != "" {
				return pipeline{}, err
			}
			f.args = args
		}
		if len(f.args) < spec.MinArgs || len(f.args) > spec.MaxArgs {
			return pipeline{}, spec.usage()
		}
		p.filters = append(p.filters, f)
	}
	return p, ""
}

func splitArgs(toks []etok) ([]operand, string) {
	var out []operand
	var cur []etok
	for _, t := range append(toks, etok{kind: ','}) {
		if t.kind == ',' {
			if len(cur) == 0 {
				return nil, "A filter value is missing."
			}
			o, err := operandFrom(cur)
			if err != "" {
				return nil, err
			}
			out = append(out, o)
			cur = nil
			continue
		}
		cur = append(cur, t)
	}
	return out, ""
}

// cond is a condition: a value (true when not empty), a comparison, or and/or/not of
// conditions. and binds tighter than or.
type cond struct {
	left, right operand
	op          string
	lit         *operand
	not         bool
	inner       *cond
	and, or     []cond
}

func (c cond) keys() []string {
	switch {
	case c.not:
		return c.inner.keys()
	case c.and != nil:
		return append(c.and[0].keys(), c.and[1].keys()...)
	case c.or != nil:
		return append(c.or[0].keys(), c.or[1].keys()...)
	case c.lit != nil:
		return c.lit.keys()
	}
	return append(c.left.keys(), c.right.keys()...)
}

const condHow = "This condition does not parse. Write it like {% if agent.phone %} or {% if ticket.priority == \"Urgent\" and contact.name %}."

func parseCond(s string) (cond, string) {
	toks, err := tokenize(s)
	if err != "" {
		return cond{}, err
	}
	if len(toks) == 0 {
		return cond{}, condHow
	}
	return condOr(toks)
}

func splitWord(toks []etok, w string) [][]etok {
	var out [][]etok
	cur := []etok{}
	depth := 0
	for _, t := range toks {
		switch t.kind {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 && t.kind == 'i' && t.s == w {
			out = append(out, cur)
			cur = []etok{}
			continue
		}
		cur = append(cur, t)
	}
	return append(out, cur)
}

func condOr(toks []etok) (cond, string) {
	parts := splitWord(toks, "or")
	c, err := condAnd(parts[0])
	if err != "" {
		return c, err
	}
	for _, p := range parts[1:] {
		next, err := condAnd(p)
		if err != "" {
			return c, err
		}
		c = cond{or: []cond{c, next}}
	}
	return c, ""
}

func condAnd(toks []etok) (cond, string) {
	parts := splitWord(toks, "and")
	c, err := condOne(parts[0])
	if err != "" {
		return c, err
	}
	for _, p := range parts[1:] {
		next, err := condOne(p)
		if err != "" {
			return c, err
		}
		c = cond{and: []cond{c, next}}
	}
	return c, ""
}

func condOne(toks []etok) (cond, string) {
	if len(toks) == 0 {
		return cond{}, condHow
	}
	if toks[0].kind == 'i' && toks[0].s == "not" {
		inner, err := condOne(toks[1:])
		if err != "" {
			return inner, err
		}
		return cond{not: true, inner: &inner}, ""
	}
	isRange := len(toks) == 5 && toks[2].kind == 'r'
	if toks[0].kind == '(' && toks[len(toks)-1].kind == ')' && !isRange {
		return condOr(toks[1 : len(toks)-1])
	}
	for i, t := range toks {
		if t.kind == 'o' || t.kind == 'i' && t.s == "contains" {
			left, e1 := operandFrom(toks[:i])
			right, e2 := operandFrom(toks[i+1:])
			if e1 != "" || e2 != "" {
				return cond{}, condHow
			}
			return cond{left: left, op: t.s, right: right}, ""
		}
	}
	left, err := operandFrom(toks)
	if err != "" {
		return cond{}, condHow
	}
	if left.isLit {
		return cond{lit: &left}, ""
	}
	return cond{left: left}, ""
}
