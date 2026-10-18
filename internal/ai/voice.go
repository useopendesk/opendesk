package ai

import (
	"regexp"
	"strings"
)

// The voice (contracts/ai.md "Voice"): everything the AI writes for a customer reads like a
// capable person on the team typed it. The prompts carry the rules; TidyReply then removes
// the strongest tells a model still leaves, with transforms that never change meaning.
// Word swaps, contractions and exclamation caps stay in the prompt only: a mechanical swap
// can hit a product name, and the bot's persona may ask for a lighter tone.

// writingStyle is how a reply reads, for prompts that write to a customer from scratch.
const writingStyle = `
## How it reads
Write like a capable person on the team typing a careful answer between other tickets, not like a template or a bot.
- Contractions (it's, don't, you'll, we've) unless the tone asks for formal writing. Active voice with a clear subject: "we refunded the order", not "the order has been refunded".
- Vary sentence length: a short sentence next to a longer one, never three of the same shape in a row. One idea per sentence, everyday words.
- Specific beats vague: name the setting, the step, the date or the number instead of "a number of options" or "in some cases".
- Get to the point in the first sentence after any greeting. Stop when the answer is complete, with at most one clear next step.` + writingBans

// ReplyStyle is writingStyle for the assistant's replies to customers.
const ReplyStyle = writingStyle

// writingBans are what customer-facing text never contains, for every prompt that writes
// or edits it.
const writingBans = `
## Never write
- Em dashes, or en dashes used as dashes. Use a period, a comma, a colon or parentheses.
- AI vocabulary: delve, leverage, utilize, robust, seamless(ly), elevate, streamline, comprehensive, foster, showcase, empower, unlock, tapestry, realm, pivotal, testament, multifaceted, moreover, furthermore, additionally.
- Stock openers and closers: "I hope this email finds you well", "Thank you for reaching out", "Great question", "I understand your frustration", "I hope this helps", "Don't hesitate to reach out", "Feel free to reach out if you have any other questions", "Looking forward to hearing from you".
- Hedging filler: "It's important to note", "It's worth mentioning", "Please be advised", "Rest assured", "at your earliest convenience".
- Negative parallelism ("it's not just X, it's Y"), neat rule-of-three lists of adjectives, and summary lines that repeat the answer ("In summary", "Overall").
- Exclamation marks in every other sentence, or apologies stacked on apologies: one "sorry" when something went wrong is enough.
These rules are written for English. In another language, follow their spirit with that language's own natural punctuation and phrasing.`

var (
	// codeSpan is markdown code, which is copied as written.
	codeSpan = regexp.MustCompile("(?s)```.*?```|`[^`\n]*`")
	// dashJoin is an em dash, or a spaced en dash or double hyphen, used to join clauses.
	// An unspaced en dash is a range (9–5, 2025–2026) and stays.
	dashJoin = regexp.MustCompile(`[ \t]*—[ \t]*|[ \t]+(?:–|--)[ \t]+`)
	// cyrillic text keeps its dashes: there a dash is grammar (a copula), not a tell.
	cyrillic = regexp.MustCompile(`\p{Cyrillic}`)
	// stockSentence is a whole stock sentence: it starts a line or follows another
	// sentence, and ends with its own punctuation, so a longer sentence that begins the
	// same way ("Thank you for reaching out about the refund.") is kept.
	stockSentence = regexp.MustCompile(`(?im)(^|[.!?][ \t]+)(?:` + strings.Join([]string{
		`I hope (?:this|my) (?:e-?mail|message|note) finds you well`,
		`I hope you(?:['’]re| are) (?:doing )?well`,
		`(?:Thank you|Thanks) (?:so much )?for reaching out(?: to us)?`,
		`Great question`,
		`I hope (?:this|that) helps`,
		`(?:Please )?(?:don['’]t|do not) hesitate to (?:reach out|contact us|get in touch)(?: if you have any (?:other |more |further )?questions)?`,
		`(?:Please )?feel free to reach out if you have any (?:other |more |further )?questions`,
	}, "|") + `)[.!]+[ \t]*`)
	blankRun  = regexp.MustCompile(`\n[ \t]*\n(?:[ \t]*\n)+`)
	lineEnd   = regexp.MustCompile(`[ \t]+\n`)
	htmlTag   = regexp.MustCompile(`<[^>]*>`)
	codeOpen  = regexp.MustCompile(`(?i)^<(code|pre)[\s>]`)
	codeClose = regexp.MustCompile(`(?i)^</(code|pre)\s*>`)
)

// TidyReply removes the strongest AI tells from a reply written for a customer, in the
// light markdown the prompts ask for (or HTML, when a model answered in HTML). Code is
// left as written. A reply that would be left empty is returned unchanged.
func TidyReply(s string) string {
	if htmlLike.MatchString(s) {
		return tidyHTML(s)
	}
	out := outsideCode(s, func(t string) string { return dropStock(fixDashes(t)) })
	out = strings.TrimSpace(blankRun.ReplaceAllString(lineEnd.ReplaceAllString(out, "\n"), "\n\n"))
	if out == "" {
		return s
	}
	return out
}

// tidyText fixes the dashes of a single line, such as a title.
func tidyText(s string) string { return fixDashes(s) }

// tidyHTML fixes the dashes in the text of HTML, never inside tags or code.
func tidyHTML(s string) string {
	var b strings.Builder
	inCode := 0
	last := 0
	for _, loc := range htmlTag.FindAllStringIndex(s, -1) {
		text := s[last:loc[0]]
		if inCode == 0 {
			text = joinDashes(text, false)
		}
		b.WriteString(text)
		tag := s[loc[0]:loc[1]]
		switch {
		case codeOpen.MatchString(tag):
			inCode++
		case codeClose.MatchString(tag) && inCode > 0:
			inCode--
		}
		b.WriteString(tag)
		last = loc[1]
	}
	rest := s[last:]
	if inCode == 0 {
		rest = joinDashes(rest, false)
	}
	b.WriteString(rest)
	return b.String()
}

// outsideCode applies f to the parts of markdown that are not code.
func outsideCode(s string, f func(string) string) string {
	var b strings.Builder
	last := 0
	for _, loc := range codeSpan.FindAllStringIndex(s, -1) {
		b.WriteString(f(s[last:loc[0]]))
		b.WriteString(s[loc[0]:loc[1]])
		last = loc[1]
	}
	b.WriteString(f(s[last:]))
	return b.String()
}

// fixDashes turns a clause-joining dash into a comma.
func fixDashes(s string) string { return joinDashes(s, true) }

// joinDashes is fixDashes; lines says a dash opening a line is markdown (a list item, a
// quote attribution, a rule), which is false for text between HTML tags.
func joinDashes(s string, lines bool) string {
	if !strings.ContainsAny(s, "—–-") || cyrillic.MatchString(s) {
		return s
	}
	rows := strings.Split(s, "\n")
	for i, ln := range rows {
		if !dashJoin.MatchString(ln) {
			continue
		}
		indent, body := "", ln
		if lines {
			body = strings.TrimLeft(ln, " \t")
			indent = ln[:len(ln)-len(body)]
		}
		// A line that opens with a dash is a list item or a quote attribution, not a join.
		lead := ""
		if lines && (strings.HasPrefix(body, "—") || strings.HasPrefix(body, "–") || strings.HasPrefix(body, "--")) {
			n := strings.IndexFunc(body, func(r rune) bool { return r != '—' && r != '–' && r != '-' })
			if n < 0 {
				continue
			}
			lead, body = body[:n], body[n:]
		}
		body = dashJoin.ReplaceAllString(body, ", ")
		body = strings.NewReplacer(", ,", ",", ", .", ".", ", :", ":", ", ;", ";", ", )", ")", "(, ", "(").Replace(body)
		if lines {
			body = strings.TrimSuffix(strings.TrimRight(body, " "), ",")
		}
		rows[i] = indent + lead + body
	}
	return strings.Join(rows, "\n")
}

// dropStock removes whole stock sentences. A match consumes the space before the next
// sentence, so two stock sentences in a row take a second pass.
func dropStock(s string) string {
	for range 3 {
		next := stockSentence.ReplaceAllStringFunc(s, func(m string) string {
			lead := stockSentence.FindStringSubmatch(m)[1]
			if lead == "" {
				return ""
			}
			return strings.TrimRight(lead, " \t") + " "
		})
		if next == s {
			break
		}
		s = next
	}
	return s
}
