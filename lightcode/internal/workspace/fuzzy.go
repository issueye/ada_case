package workspace

import "strings"

// Match reports whether pat matches s as a subsequence, ignoring case, and
// how good a match it is: higher scores start earlier, run consecutively
// and hit word boundaries. It is the filter of quick open and the command
// palette.
func Match(pat, s string) (score int, ok bool) {
	if pat == "" {
		return 0, true
	}
	p := []rune(strings.ToLower(pat))
	// Spaces in a query match any separator, as "foo bar" matches
	// "foo/bar.go".
	var gaps int
	si := 0
	runes := []rune(s)
	for pi := 0; pi < len(p); pi++ {
		c := p[pi]
		if c == ' ' {
			continue
		}
		for {
			if si >= len(runes) {
				return 0, false
			}
			r := runes[si]
			si++
			if strings.ToLower(string(r)) != string(c) {
				gaps++
				continue
			}
			break
		}
	}
	// Shorter candidates are better, and a match that starts early too.
	score = 1000 - gaps
	if len(runes) > 0 {
		score += 200 * (len(p) / len(runes))
	}
	first := strings.Index(strings.ToLower(s), string(p[0]))
	if first == 0 {
		score += 300
	} else if first > 0 && first < len(runes) && (runes[first-1] == '/' || runes[first-1] == '\\' || runes[first-1] == '_' || runes[first-1] == '.') {
		score += 200
	}
	return score, true
}
