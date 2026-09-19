package wargame

import (
	"testing"
)

func TestSyncAppliesConfigAsSourceOfTruth(t *testing.T) {
	s := tempStore(t)
	// pre-existing state: bandit 1,2,3 and a game config never mentions
	s.SetFlag("bandit", 1, "old1")
	s.SetFlag("bandit", 2, "old2")
	s.SetFlag("bandit", 3, "keep3")
	s.SetFlag("natas", 1, "leave-me-alone")

	// config: bandit level 1 fixed, level 2 removed (empty flag), level 3
	// kept, level 4 added. natas absent -> untouched.
	if _, err := s.Sync(map[string]map[int]string{
		"bandit": {1: "new1", 2: "", 3: "keep3", 4: "new4"},
	}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// the board is exactly {1,3,4}: SubmitFlag walks levels in order, so
	// a fresh player passes 1, then hits "no flag set" at level 2.
	if ok, lvl, _ := s.SubmitFlag("fp", "bandit", "new1"); !ok || lvl != 1 {
		t.Fatalf("level 1: ok=%v lvl=%d, want the config's new flag accepted", ok, lvl)
	}
	if _, _, err := s.SubmitFlag("fp", "bandit", "old2"); err == nil {
		t.Fatal("removed level 2 still has a flag")
	}
	// the removed slot is skipped by re-adding via level 3's flag directly:
	// progress is at 1, so submitting keep3 fails (expected next is 2)...
	// instead assert membership through ListFlags.
	levels := s.ListFlags("bandit")
	got := map[int]bool{}
	for _, l := range levels {
		got[l] = true
	}
	if !got[1] || got[2] || !got[3] || !got[4] {
		t.Fatalf("board = %v, want {1,3,4}", levels)
	}
	// natas untouched
	if ok, _, _ := s.SubmitFlag("fpn", "natas", "leave-me-alone"); !ok {
		t.Fatal("sync clobbered a game it does not manage")
	}
}

func TestSyncIsIdempotent(t *testing.T) {
	s := tempStore(t)
	games := map[string]map[int]string{"bandit": {1: "a", 2: "b"}}
	if _, err := s.Sync(games); err != nil {
		t.Fatal(err)
	}
	n, err := s.Sync(games)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("second sync changed %d rows, want 0", n)
	}
}
