package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadWargamePurgeFeed(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ryolink.yaml")
	os.WriteFile(p, []byte(`
ryolink:
  name: test
  domain: test.dev
owner:
  name: boss
  fingerprint: "SHA256:abc"
rooms:
  - name: lounge
    type: chat
  - name: bandit
    type: wargame
wargame:
  games:
    bandit:
      1: "aaa"
      2: ""        # empty = remove this level
purge:
  weekday: saturday
  time: "04:00"
feed:
  subreddits: ["archlinux", "games"]
`), 0600)

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cfg.Wargame.Games["bandit"][1]; got != "aaa" {
		t.Fatalf("bandit level 1 = %q, want aaa", got)
	}
	if got, ok := cfg.Wargame.Games["bandit"][2]; !ok || got != "" {
		t.Fatalf("bandit level 2 = %q (present=%v), want empty-but-listed", got, ok)
	}
	day, hh, mm, err := cfg.PurgeTime()
	if err != nil || day != time.Saturday || hh != 4 || mm != 0 {
		t.Fatalf("PurgeTime = %v %d:%d %v, want Saturday 04:00", day, hh, mm, err)
	}
	if len(cfg.Feed.Subreddits) != 2 || cfg.Feed.Subreddits[0] != "archlinux" {
		t.Fatalf("feed = %v", cfg.Feed.Subreddits)
	}
}

func TestPurgeScheduleValidation(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) error {
		p := filepath.Join(dir, "r.yaml")
		os.WriteFile(p, []byte(`
ryolink: {name: t, domain: t.dev}
owner: {name: b, fingerprint: "SHA256:x"}
rooms: [{name: l, type: chat}]
`+body), 0600)
		_, err := Load(p)
		return err
	}
	if err := write("purge: {weekday: notaday}\n"); err == nil {
		t.Fatal("bad weekday accepted")
	}
	if err := write("purge: {time: \"99:99\"}\n"); err == nil {
		t.Fatal("bad time accepted")
	}
	if err := write("purge: {disabled: true, weekday: notaday}\n"); err != nil {
		t.Fatalf("disabled purge should skip schedule validation: %v", err)
	}
	if err := write("purge: {weekday: monday, time: \"00:30\"}\n"); err != nil {
		t.Fatalf("valid schedule rejected: %v", err)
	}
}

func TestWargameLevelValidation(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "r.yaml")
	os.WriteFile(p, []byte(`
ryolink: {name: t, domain: t.dev}
owner: {name: b, fingerprint: "SHA256:x"}
rooms: [{name: l, type: chat}]
wargame:
  games:
    bandit:
      0: "nope"
`), 0600)
	if _, err := Load(p); err == nil {
		t.Fatal("level 0 accepted; levels start at 1")
	}
}
