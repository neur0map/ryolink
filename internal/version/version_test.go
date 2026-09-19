package version

import (
	"os"
	"strings"
	"testing"
)

// The embedded copy must stay byte-identical to the root CHANGELOG.md.
// `make changelog` (and CI's check) refresh/enforce it; the release
// workflow regenerates it as part of the version bump commit.
func TestChangelogCopyInSync(t *testing.T) {
	root, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatalf("read root changelog: %v", err)
	}
	if string(root) != changelogMD {
		t.Fatal("internal/version/CHANGELOG.md drifted from CHANGELOG.md — run: make changelog")
	}
}

// The version const, the changelog's newest entry, and the parsed table
// must agree — this is what makes the TUI splash, the changelog modal, and
// the GitHub release notes say the same version.
func TestVersionMatchesChangelog(t *testing.T) {
	if len(Changelog) == 0 {
		t.Fatal("changelog parsed to zero entries — format broken")
	}
	if Changelog[0].Version != Version {
		t.Fatalf("version.go says %q, changelog top entry says %q", Version, Changelog[0].Version)
	}
	for _, e := range Changelog {
		if len(e.Changes) == 0 {
			t.Fatalf("v%s has no change lines", e.Version)
		}
	}
}

// Notes() must render exactly the top section — the release workflow posts
// this text verbatim, so a parser bug here ships wrong release notes.
func TestNotesIsTopSection(t *testing.T) {
	n := Notes()
	if !strings.HasPrefix(n, "## v"+Version) {
		t.Fatalf("notes start with %q, want the v%s header", n[:20], Version)
	}
	// no other version header may appear on any line of the notes
	for _, line := range strings.Split(n, "\n") {
		if strings.HasPrefix(line, "## v") && line != "## v"+Version &&
			!strings.HasPrefix(line, "## v"+Version+" ") {
			t.Fatalf("notes leaked a second version header: %q", line)
		}
	}
	// every bullet of the top entry must be present
	for _, c := range Changelog[0].Changes {
		if !strings.Contains(n, c) {
			t.Fatalf("notes missing line %q", c)
		}
	}
}

func TestParseHandlesSuffixes(t *testing.T) {
	entries := parse("# Changelog\n\n## v1.2 — the thing\n\n- did a thing\n\n## v1.1\n\n- older\n")
	if len(entries) != 2 || entries[0].Version != "1.2" || entries[1].Version != "1.1" {
		t.Fatalf("parse = %+v", entries)
	}
	if len(entries[0].Changes) != 1 || entries[0].Changes[0] != "did a thing" {
		t.Fatalf("changes = %+v", entries[0].Changes)
	}
}
