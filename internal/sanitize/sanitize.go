package sanitize

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Clean strips control characters and invalid UTF-8 while preserving
// printable ASCII, emojis, and international characters.
func Clean(input string) string {
	var b strings.Builder
	b.Grow(len(input))
	for _, r := range input {
		if r == utf8.RuneError {
			continue
		}
		// Allow printable ASCII (space through tilde)
		if r >= 0x20 && r <= 0x7E {
			b.WriteRune(r)
			continue
		}
		// Allow Unicode letters, numbers, symbols, punctuation, and marks
		// (covers emojis, CJK, accented chars, etc.)
		// Block control characters, format chars, and surrogates.
		if unicode.IsPrint(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ReservedNicks are nicknames that cannot be claimed by regular users.
var ReservedNicks = map[string]bool{
	"admin":     true,
	"ryolink":   true,
	"system":    true,
	"bartender": true,
	"Mika":      true,
}

var ownerNick string

// SetOwnerNick registers the owner's nickname as reserved.
// Must be called before any concurrent access to CleanNick.
func SetOwnerNick(nick string) {
	ownerNick = nick
}

// CleanNick sanitizes a nickname. Must be 2-20 runes after cleaning.
// Nicknames are ASCII-only to keep them easy to type and @-mention.
func CleanNick(nick string) (string, error) {
	var b strings.Builder
	for _, r := range nick {
		if r >= 0x20 && r <= 0x7E {
			b.WriteRune(r)
		}
	}
	cleaned := b.String()
	if utf8.RuneCountInString(cleaned) < 2 || utf8.RuneCountInString(cleaned) > 20 {
		return "", errors.New("nickname must be 2-20 characters")
	}
	if ReservedNicks[strings.ToLower(cleaned)] {
		return "", errors.New("that nickname is reserved")
	}
	if ownerNick != "" && strings.EqualFold(cleaned, ownerNick) {
		return "", errors.New("that nickname is reserved")
	}
	return cleaned, nil
}

// CleanChat sanitizes a chat message. Strips control chars, caps at 500 runes.
func CleanChat(msg string) string {
	cleaned := Clean(msg)
	if utf8.RuneCountInString(cleaned) > 500 {
		runes := []rune(cleaned)
		cleaned = string(runes[:500])
	}
	return cleaned
}

// CleanNote sanitizes a gallery note. Strips control chars, caps at 280 runes.
func CleanNote(msg string) string {
	cleaned := Clean(msg)
	if utf8.RuneCountInString(cleaned) > 280 {
		runes := []rune(cleaned)
		cleaned = string(runes[:280])
	}
	return cleaned
}
