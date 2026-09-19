package identity

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"strings"
)

var adjectives = []string{
	"dusty", "quiet", "wandering", "sleepy", "hooded",
	"scarred", "weary", "shadowy", "lone", "grizzled",
	"nimble", "silent", "copper", "ashen", "veiled",
	"stout", "swift", "rusty", "hollow", "mossy",
}

var nouns = []string{
	"pilgrim", "drifter", "bard", "ranger", "rogue",
	"sage", "merchant", "smith", "herald", "scout",
	"monk", "keeper", "hunter", "scribe", "warden",
	"brewer", "hermit", "jester", "rider", "ghost",
}

// DefaultNickname returns a ryolink-themed name with a unique discriminator.
// Format: adjective_noun#0000 (e.g. "dusty_pilgrim#4827")
func DefaultNickname(fingerprint string) string {
	hash := sha256.Sum256([]byte(fingerprint))

	adj := adjectives[int(hash[0])%len(adjectives)]
	noun := nouns[int(hash[1])%len(nouns)]
	disc := binary.BigEndian.Uint16(hash[2:4]) % 10000
	return fmt.Sprintf("%s_%s#%04d", adj, noun, disc)
}

// RandomNickname returns a randomly generated ryolink-themed name.
// Format: adjective_noun#0000 (e.g. "nimble_drifter#3966")
func RandomNickname() string {
	adj := adjectives[rand.IntN(len(adjectives))]
	noun := nouns[rand.IntN(len(nouns))]
	disc := rand.IntN(10000)
	return fmt.Sprintf("%s_%s#%04d", adj, noun, disc)
}

// ColorIndex returns 0-11 for mapping a fingerprint to one of 12 cantina colors.
func ColorIndex(fingerprint string) int {
	hash := sha256.Sum256([]byte(fingerprint))
	return int(hash[0]) % 12
}

// HasFlair returns true if the user has connected 3+ times this week.
func HasFlair(visitCount int) bool {
	return visitCount >= 3
}

// IsOwnerFingerprint returns true if fingerprint matches the configured owner.
// fingerprint is the server's hex sha256 of the public-key blob;
// ownerFingerprint may be either that same hex form, or the
// "SHA256:<base64url>" form printed by `ssh-keygen -lf` (documented in
// SETUP.md). Both encodings are the same digest, so normalize before compare.
func IsOwnerFingerprint(fingerprint, ownerFingerprint string) bool {
	if fingerprint == "" || ownerFingerprint == "" {
		return false
	}
	if fingerprint == ownerFingerprint {
		return true
	}
	if b64, ok := strings.CutPrefix(ownerFingerprint, "SHA256:"); ok {
		raw, err := base64.RawURLEncoding.DecodeString(b64)
		if err != nil {
			raw, err = base64.URLEncoding.DecodeString(b64)
		}
		if err != nil {
			raw, err = base64.StdEncoding.DecodeString(b64)
		}
		if err != nil {
			return false
		}
		return fingerprint == hex.EncodeToString(raw)
	}
	return false
}

// OwnerDisplayName returns the special display name for the owner.
func OwnerDisplayName(ownerName string) string {
	return "★ " + ownerName
}
