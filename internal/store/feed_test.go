package store

import (
	"reflect"
	"testing"
)

func TestSyncFeedSubreddits(t *testing.T) {
	s := tempStore(t)
	// live-added subs (as --feed-add would)
	if err := s.AddFeedSubreddit("oldsub", "boss"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddFeedSubreddit("keepme", "boss"); err != nil {
		t.Fatal(err)
	}

	// config list: keeps one, adds two, drops oldsub
	if err := s.SyncFeedSubreddits([]string{"keepme", "archlinux", "unixporn"}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	got := s.FeedSubreddits()
	// added_at ordering: keepme first (older row survives), then config adds
	if len(got) != 3 || got[0] != "keepme" {
		t.Fatalf("after sync = %v, want [keepme archlinux unixporn]", got)
	}
	for _, want := range []string{"archlinux", "unixporn"} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s missing from %v", want, got)
		}
	}

	// idempotent: same list changes nothing
	if err := s.SyncFeedSubreddits([]string{"keepme", "archlinux", "unixporn"}); err != nil {
		t.Fatal(err)
	}
	if again := s.FeedSubreddits(); !reflect.DeepEqual(again, got) {
		t.Fatalf("second sync reordered/changed the list: %v vs %v", again, got)
	}

	// empty config list is a no-op (sync only runs when the block is set)
	// — verify the guard behavior at the caller, not here: SyncFeedSubreddits
	// with an empty slice removes everything, which is why main.go checks
	// len(cfg.Feed.Subreddits) > 0 first.
}
