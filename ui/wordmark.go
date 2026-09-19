package ui

import "strings"

// Wordmark logos: a single-line `logo:` value in the config (e.g. "ARCH")
// is rendered as a five-row block-letter wordmark. Anything multi-line is
// treated as raw ASCII art, as before. The font is 5 rows tall; glyphs vary
// in width and join with a single space.
var wordFont = map[rune][]string{
	'A': {" ██ ", "█  █", "████", "█  █", "█  █"},
	'B': {"███ ", "█  █", "███ ", "█  █", "███ "},
	'C': {" ███", "█   ", "█   ", "█   ", " ███"},
	'D': {"███ ", "█  █", "█  █", "█  █", "███ "},
	'E': {"████", "█   ", "███ ", "█   ", "████"},
	'F': {"████", "█   ", "███ ", "█   ", "█   "},
	'G': {" ███", "█   ", "█  █", "█  █", " ███"},
	'H': {"█  █", "█  █", "████", "█  █", "█  █"},
	'I': {"███", " █ ", " █ ", " █ ", "███"},
	'J': {"████", "   █", "   █", "█  █", "███ "},
	'K': {"█  █", "█ █ ", "██  ", "█ █ ", "█  █"},
	'L': {"█  ", "█  ", "█  ", "█  ", "███"},
	'M': {"█   █", "██ ██", "█ █ █", "█   █", "█   █"},
	'N': {"█  █", "██ █", "█ ██", "█  █", "█  █"},
	'O': {" ██ ", "█  █", "█  █", "█  █", " ██ "},
	'P': {"███ ", "█  █", "███ ", "█   ", "█   "},
	'Q': {" ██ ", "█  █", "█  █", "█ █ ", " ██ "},
	'R': {"███ ", "█  █", "███ ", "█ █ ", "█  █"},
	'S': {" ███", "█   ", " ██ ", "   █", "███ "},
	'T': {"████", " ██ ", " ██ ", " ██ ", " ██ "},
	'U': {"█  █", "█  █", "█  █", "█  █", " ██ "},
	'V': {"█  █", "█  █", "█  █", " ██ ", " █  "},
	'W': {"█   █", "█   █", "█ █ █", "██ ██", " ██ "},
	'X': {"█   █", " █ █ ", "  █  ", " █ █ ", "█   █"},
	'Y': {"█   █", " █ █ ", "  █  ", "  █  ", "  █  "},
	'Z': {"████", "  █ ", " █  ", "█   ", "████"},
	'0': {" ██ ", "█  █", "█  █", "█  █", " ██ "},
	'1': {" █ ", "██ ", " █ ", " █ ", "███"},
	'2': {"███ ", "   █", " ██ ", "█   ", "████"},
	'3': {"████", "   █", " ██ ", "   █", "███ "},
	'4': {"█  █", "█  █", "████", "   █", "   █"},
	'5': {"████", "█   ", "███ ", "   █", "███ "},
	'6': {" ██ ", "█   ", "███ ", "█  █", " ██ "},
	'7': {"████", "   █", "  █ ", " █  ", " █  "},
	'8': {" ██ ", "█  █", " ██ ", "█  █", " ██ "},
	'9': {" ██ ", "█  █", " ███", "   █", " ██ "},
	' ': {" ", " ", " ", " ", " "},
	'-': {"    ", "    ", "████", "    ", "    "},
}

// IsWordmark reports whether a logo value should be rendered as a
// block-letter word: a single short line of letters/digits/spaces.
func IsWordmark(logo string) bool {
	if logo == "" || strings.ContainsAny(logo, "\n") {
		return false
	}
	if len(logo) > 10 {
		return false
	}
	for _, r := range strings.ToUpper(logo) {
		if _, ok := wordFont[r]; !ok {
			return false
		}
	}
	return true
}

// Wordmark renders text as five block-letter rows, uppercased. Unknown
// characters are skipped.
func Wordmark(text string) []string {
	rows := []string{"", "", "", "", ""}
	for i, r := range strings.ToUpper(strings.TrimSpace(text)) {
		g, ok := wordFont[r]
		if !ok {
			continue
		}
		for row := range rows {
			if i > 0 {
				rows[row] += " "
			}
			rows[row] += g[row]
		}
	}
	return rows
}
