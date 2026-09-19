// Package version carries the release identity. The changelog is authored
// once, in CHANGELOG.md at the repo root; a byte-identical copy lives here
// (embedded, since the binary must be self-contained) and the sync test
// fails the build if the two ever drift — which is what keeps the TUI
// changelog, the repo file, and the GitHub release notes saying the same
// thing. `make changelog` refreshes the embedded copy.
package version

import (
	_ "embed"
	"strings"
)

// Entry holds a single version's changelog.
type Entry struct {
	Version string
	Changes []string
}

//go:embed CHANGELOG.md
var changelogMD string

// Changelog parses the embedded markdown into entries, newest first.
// Format contract (also what the release workflow emits):
//
//	# Changelog
//
//	## v0.9 — optional suffix
//
//	- one line per change
var Changelog = parse(changelogMD)

// Latest returns the newest changelog entry's version. The sync test
// asserts it equals the Version const — if a release bumps one file and
// forgets the other, the build gate fails.
func Latest() string {
	if len(Changelog) > 0 {
		return Changelog[0].Version
	}
	return "0.0"
}

// Notes returns the markdown section for the latest version, verbatim —
// this is what the release workflow publishes as GitHub release notes, so
// the tag page and the in-app modal show the same text.
func Notes() string {
	i := strings.Index(changelogMD, "## v")
	if i < 0 {
		return ""
	}
	rest := changelogMD[i:]
	if j := strings.Index(rest[strings.Index(rest, "\n")+1:], "\n## v"); j >= 0 {
		rest = rest[:strings.Index(rest, "\n")+1+j]
	}
	return strings.TrimSpace(rest)
}

func parse(md string) []Entry {
	var out []Entry
	var cur *Entry
	for _, line := range strings.Split(md, "\n") {
		if v, ok := strings.CutPrefix(line, "## v"); ok {
			if cur != nil {
				out = append(out, *cur)
			}
			// "0.9 — the store" -> "0.9"
			v = strings.TrimSpace(v)
			if sp := strings.IndexAny(v, " —-"); sp > 0 {
				v = v[:sp]
			}
			cur = &Entry{Version: v}
			continue
		}
		if cur == nil {
			continue
		}
		if t, ok := strings.CutPrefix(line, "- "); ok {
			cur.Changes = append(cur.Changes, strings.TrimSpace(t))
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}
