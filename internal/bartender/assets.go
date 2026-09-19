package bartender

import (
	_ "embed"
	"os"
	"path/filepath"
)

// soul.md ships inside the binary so a fresh install has a working
// bartender with zero asset setup. A bartender/soul.md in the data dir (or
// working dir) overrides it for owners who want to tune the character.
//
//go:embed soul.md
var embeddedSoul []byte

// DefaultSoul is the last-resort persona if nothing else is available.
const DefaultSoul = "You are a gruff bartender in a ryolink chatroom. Keep replies to 1-2 sentences."

// LoadSoul resolves the bartender persona: data-dir override, then working
// dir, then the embedded copy, then DefaultSoul.
func LoadSoul(dataDir string) string {
	for _, dir := range []string{dataDir, "."} {
		if dir == "" {
			continue
		}
		for _, name := range []string{"bartender/soul.md", "soul.md"} {
			if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil && len(b) > 0 {
				return string(b)
			}
		}
	}
	return string(embeddedSoul)
}
