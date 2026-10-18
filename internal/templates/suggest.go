package templates

import "strings"

// closest is the candidate nearest to s when it is close enough to be a typo, else "".
func closest(s string, candidates []string) string {
	best, bestD := "", 1<<30
	for _, c := range candidates {
		if d := distance(s, c); d < bestD {
			best, bestD = c, d
		}
	}
	limit := 2
	if n := len(s) / 4; n > limit {
		limit = n
	}
	if bestD == 0 || bestD > limit {
		return ""
	}
	return best
}

// didYouMean is " Did you mean x?" for the nearest candidate, or "".
func didYouMean(s string, candidates []string) string {
	if c := closest(s, candidates); c != "" {
		return " Did you mean " + c + "?"
	}
	return ""
}

// distance is the Levenshtein distance of a and b.
func distance(a, b string) int {
	ra, rb := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

func filterNames() []string {
	out := make([]string, len(Filters))
	for i, f := range Filters {
		out[i] = f.Name
	}
	return out
}

var tagWords = []string{"if", "elsif", "else", "endif", "unless", "endunless", "case", "when", "endcase", "for", "endfor", "assign", "capture", "endcapture", "comment", "endcomment", "raw", "endraw"}
