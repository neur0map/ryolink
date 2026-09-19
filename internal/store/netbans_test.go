package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNetBansRoundTrip(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	perm := time.Time{}
	expiry := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err := st.AddDeny("1.2.3.4", &perm, "manual"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddDeny("2001:db8::/64", &expiry, "probing"); err != nil {
		t.Fatal(err)
	}

	active := st.LoadActiveDenies(time.Now())
	if got, ok := active["1.2.3.4"]; !ok || !got.IsZero() {
		t.Fatalf("permanent ban missing or wrong expiry: %v", active)
	}
	got, ok := active["2001:db8::/64"]
	if !ok || got.Sub(expiry) > time.Second || got.Sub(expiry) < -time.Second {
		t.Fatalf("timed ban round-trip wrong: %v vs %v", got, expiry)
	}

	// upsert replaces the previous row for the same cidr
	if err := st.AddDeny("1.2.3.4", &expiry, "downgraded to temp"); err != nil {
		t.Fatal(err)
	}
	list := st.ListDenies(time.Now())
	if len(list) != 2 {
		t.Fatalf("want 2 denies, got %d", len(list))
	}
	for _, d := range list {
		if d.CIDR == "1.2.3.4" && (d.Until.IsZero() || d.Reason != "downgraded to temp") {
			t.Fatalf("upsert did not replace: %+v", d)
		}
	}

	// expired rows disappear from both views
	if err := st.AddDeny("5.6.7.8", ptrTime(time.Now().Add(-time.Minute)), "gone"); err != nil {
		t.Fatal(err)
	}
	if len(st.ListDenies(time.Now())) != 2 {
		t.Fatal("expired ban must not be listed")
	}
	if _, ok := st.LoadActiveDenies(time.Now())["5.6.7.8"]; ok {
		t.Fatal("expired ban must be swept on load")
	}

	// removal
	if err := st.RemoveDeny("1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.LoadActiveDenies(time.Now())["1.2.3.4"]; ok {
		t.Fatal("removed ban must be gone")
	}
}

func TestNetBansSurvivePurge(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	perm := time.Time{}
	if err := st.AddDeny("9.9.9.9", &perm, "troll net"); err != nil {
		t.Fatal(err)
	}
	if err := st.PurgeAll(); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.LoadActiveDenies(time.Now())["9.9.9.9"]; !ok {
		t.Fatal("network bans must survive a weekly purge")
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
