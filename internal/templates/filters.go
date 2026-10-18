package templates

import (
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// FilterSpec describes a filter for the editor and the docs.
type FilterSpec struct {
	Name        string `json:"name"`
	Group       string `json:"group" enum:"text,list,number,date"`
	Args        string `json:"args" doc:"What it takes after the colon, or empty"`
	Description string `json:"description"`
	Example     string `json:"example"`
	MinArgs     int    `json:"-"`
	MaxArgs     int    `json:"-"`
}

func (f FilterSpec) usage() string {
	if f.Args == "" {
		return f.Name + " takes no value."
	}
	return "Write it like | " + f.Name + ": " + f.Args + "."
}

// Filters are every filter, in the order the editor lists them.
var Filters = []FilterSpec{
	{"default", "text", `"fallback"`, "The fallback when the value is empty.", `{{ contact.first_name | default: "there" }}`, 1, 1},
	{"upcase", "text", "", "Upper case.", `{{ team.name | upcase }}`, 0, 0},
	{"downcase", "text", "", "Lower case.", `{{ contact.email | downcase }}`, 0, 0},
	{"capitalize", "text", "", "The first letter upper case.", `{{ ticket.status | capitalize }}`, 0, 0},
	{"titlecase", "text", "", "Every word starting upper case.", `{{ contact.name | titlecase }}`, 0, 0},
	{"strip", "text", "", "Spaces at both ends removed.", `{{ contact.name | strip }}`, 0, 0},
	{"truncate", "text", `20, "…"`, "Cut to a number of characters, the ending included.", `{{ ticket.subject | truncate: 40 }}`, 1, 2},
	{"truncatewords", "text", `10, "…"`, "Cut to a number of words.", `{{ ticket.subject | truncatewords: 5 }}`, 1, 2},
	{"append", "text", `"text"`, "Text added at the end.", `{{ ticket.number | append: " (open)" }}`, 1, 1},
	{"prepend", "text", `"text"`, "Text added at the start.", `{{ ticket.number | prepend: "#" }}`, 1, 1},
	{"replace", "text", `"old", "new"`, "Every match replaced.", `{{ ticket.subject | replace: "Re: ", "" }}`, 2, 2},
	{"replace_first", "text", `"old", "new"`, "The first match replaced.", `{{ ticket.subject | replace_first: "-", ":" }}`, 2, 2},
	{"remove", "text", `"text"`, "Every match removed.", `{{ ticket.subject | remove: "[urgent]" }}`, 1, 1},
	{"slice", "text", "0, 10", "Characters from a position, with an optional length.", `{{ contact.name | slice: 0, 1 }}`, 1, 2},
	{"url_encode", "text", "", "Safe inside a link.", `{{ contact.email | url_encode }}`, 0, 0},
	{"split", "list", `", "`, "Text into a list, at each separator.", `{% for part in contact.fields.notes | split: "," %}`, 1, 1},
	{"join", "list", `", "`, "A list into text, with the separator (default \", \").", `{{ ticket.labels | join: " / " }}`, 0, 1},
	{"first", "list", "", "The first item, or the first word of text.", `{{ ticket.labels | first }}`, 0, 0},
	{"last", "list", "", "The last item, or the last word of text.", `{{ contact.name | last }}`, 0, 0},
	{"size", "list", "", "How many items, or characters of text.", `{{ ticket.labels | size }}`, 0, 0},
	{"sort", "list", "", "Items in alphabetical order.", `{{ ticket.labels | sort | join }}`, 0, 0},
	{"uniq", "list", "", "Repeated items removed.", `{{ contact.labels | uniq | join }}`, 0, 0},
	{"plus", "number", "1", "Adds a number.", `{{ ticket.number | plus: 1 }}`, 1, 1},
	{"minus", "number", "1", "Subtracts a number.", `{{ forloop.index | minus: 1 }}`, 1, 1},
	{"times", "number", "2", "Multiplies.", `{{ ticket.fields.seats | times: 12 }}`, 1, 1},
	{"divided_by", "number", "2", "Divides.", `{{ ticket.fields.amount | divided_by: 100 }}`, 1, 1},
	{"round", "number", "2", "Rounds, to a number of decimals (default 0).", `{{ ticket.fields.score | round: 1 }}`, 0, 1},
	{"date", "date", `"%e %B %Y"`, "A date in another format (strftime: %Y %m %d %e %B %b %A %a %H %I %M %p). \"now\" is the current time.", `{{ ticket.created_date | date: "%A, %b %e" }}`, 1, 1},
}

var filterSpecs = func() map[string]FilterSpec {
	m := map[string]FilterSpec{}
	for _, f := range Filters {
		m[f.Name] = f
	}
	return m
}()

func formatNumber(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func listOf(v value) []string {
	if v.isList {
		return v.list
	}
	return strings.Fields(v.str())
}

func applyFilter(name string, v value, args []value, vals *Values) value {
	arg := func(i int) string {
		if i < len(args) {
			return args[i].str()
		}
		return ""
	}
	argInt := func(i, def int) int {
		if i >= len(args) {
			return def
		}
		n, err := strconv.Atoi(strings.TrimSpace(arg(i)))
		if err != nil {
			return def
		}
		return n
	}
	math2 := func(op func(a, b float64) float64) value {
		a, ok1 := number(v.str())
		b, ok2 := number(arg(0))
		if !ok1 || !ok2 {
			return v
		}
		r := op(a, b)
		if math.IsInf(r, 0) || math.IsNaN(r) {
			return textValue("")
		}
		return textValue(formatNumber(r))
	}
	if name == "default" {
		if v.empty() {
			return args[0]
		}
		return v
	}
	if v.html {
		v = textValue(TextOf(v.s))
	}
	s := v.str()
	switch name {
	case "upcase":
		return textValue(strings.ToUpper(s))
	case "downcase":
		return textValue(strings.ToLower(s))
	case "capitalize":
		if r, size := utf8.DecodeRuneInString(s); size > 0 {
			return textValue(string(unicode.ToUpper(r)) + s[size:])
		}
		return v
	case "titlecase":
		words := strings.Fields(s)
		for i, w := range words {
			r, size := utf8.DecodeRuneInString(w)
			words[i] = string(unicode.ToUpper(r)) + w[size:]
		}
		return textValue(strings.Join(words, " "))
	case "strip":
		return textValue(strings.TrimSpace(s))
	case "truncate":
		n, end := argInt(0, 50), "…"
		if len(args) > 1 {
			end = arg(1)
		}
		r := []rune(s)
		if len(r) <= n {
			return textValue(s)
		}
		keep := n - utf8.RuneCountInString(end)
		if keep < 0 {
			keep = 0
		}
		return textValue(strings.TrimRight(string(r[:keep]), " ") + end)
	case "truncatewords":
		n, end := argInt(0, 15), "…"
		if len(args) > 1 {
			end = arg(1)
		}
		words := strings.Fields(s)
		if len(words) <= n {
			return textValue(s)
		}
		if n < 1 {
			n = 1
		}
		return textValue(strings.Join(words[:n], " ") + end)
	case "append":
		return textValue(s + arg(0))
	case "prepend":
		return textValue(arg(0) + s)
	case "replace":
		if arg(0) == "" {
			return textValue(s)
		}
		return textValue(strings.ReplaceAll(s, arg(0), arg(1)))
	case "replace_first":
		if arg(0) == "" {
			return textValue(s)
		}
		return textValue(strings.Replace(s, arg(0), arg(1), 1))
	case "remove":
		if arg(0) == "" {
			return textValue(s)
		}
		return textValue(strings.ReplaceAll(s, arg(0), ""))
	case "slice":
		r := []rune(s)
		start := argInt(0, 0)
		if start < 0 {
			start += len(r)
		}
		length := argInt(1, 1)
		if start < 0 || start >= len(r) || length <= 0 {
			return textValue("")
		}
		end := start + length
		if end > len(r) {
			end = len(r)
		}
		return textValue(string(r[start:end]))
	case "url_encode":
		return textValue(url.QueryEscape(s))
	case "split":
		sep := arg(0)
		var parts []string
		if sep == "" {
			for _, r := range s {
				parts = append(parts, string(r))
			}
		} else {
			for _, p := range strings.Split(s, sep) {
				if p = strings.TrimSpace(p); p != "" {
					parts = append(parts, p)
				}
			}
		}
		return value{list: parts, isList: true}
	case "join":
		sep := ", "
		if len(args) > 0 {
			sep = arg(0)
		}
		return textValue(strings.Join(listOf(v), sep))
	case "first":
		if l := listOf(v); len(l) > 0 {
			return textValue(l[0])
		}
		return textValue("")
	case "last":
		if l := listOf(v); len(l) > 0 {
			return textValue(l[len(l)-1])
		}
		return textValue("")
	case "size":
		if v.isList {
			return textValue(strconv.Itoa(len(v.list)))
		}
		return textValue(strconv.Itoa(utf8.RuneCountInString(s)))
	case "sort":
		l := append([]string{}, listOf(v)...)
		sort.SliceStable(l, func(i, j int) bool { return strings.ToLower(l[i]) < strings.ToLower(l[j]) })
		return value{list: l, isList: true}
	case "uniq":
		var l []string
		seen := map[string]bool{}
		for _, it := range listOf(v) {
			if k := strings.ToLower(it); !seen[k] {
				seen[k] = true
				l = append(l, it)
			}
		}
		return value{list: l, isList: true}
	case "plus":
		return math2(func(a, b float64) float64 { return a + b })
	case "minus":
		return math2(func(a, b float64) float64 { return a - b })
	case "times":
		return math2(func(a, b float64) float64 { return a * b })
	case "divided_by":
		return math2(func(a, b float64) float64 { return a / b })
	case "round":
		f, ok := number(s)
		if !ok {
			return v
		}
		p := math.Pow(10, float64(argInt(0, 0)))
		return textValue(formatNumber(math.Round(f*p) / p))
	case "date":
		loc := time.UTC
		if vals != nil && vals.Location != nil {
			loc = vals.Location
		}
		t, ok := parseDate(s, loc)
		if !ok {
			return v
		}
		return textValue(strftime(t.In(loc), arg(0)))
	}
	return v
}

func parseDate(s string, loc *time.Location) (time.Time, bool) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "now", "today":
		return time.Now(), true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	for _, layout := range []string{"2006-01-02", "2 January 2006", "January 2, 2006", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// strftime formats t with the common strftime directives.
func strftime(t time.Time, f string) string {
	var b strings.Builder
	for i := 0; i < len(f); i++ {
		if f[i] != '%' || i+1 >= len(f) {
			b.WriteByte(f[i])
			continue
		}
		i++
		switch f[i] {
		case 'Y':
			b.WriteString(strconv.Itoa(t.Year()))
		case 'y':
			b.WriteString(t.Format("06"))
		case 'm':
			b.WriteString(t.Format("01"))
		case 'd':
			b.WriteString(t.Format("02"))
		case 'e':
			b.WriteString(strconv.Itoa(t.Day()))
		case 'B':
			b.WriteString(t.Month().String())
		case 'b':
			b.WriteString(t.Format("Jan"))
		case 'A':
			b.WriteString(t.Weekday().String())
		case 'a':
			b.WriteString(t.Format("Mon"))
		case 'H':
			b.WriteString(t.Format("15"))
		case 'I':
			b.WriteString(t.Format("03"))
		case 'l':
			b.WriteString(t.Format("3"))
		case 'M':
			b.WriteString(t.Format("04"))
		case 'p':
			b.WriteString(t.Format("PM"))
		case 'Z':
			b.WriteString(t.Format("MST"))
		case 'j':
			b.WriteString(strconv.Itoa(t.YearDay()))
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(f[i])
		}
	}
	return b.String()
}
