package guard

import (
	"net"
	"testing"
	"time"
)

// memStore is the in-memory twin of the SQLite deny store.
type memStore struct {
	denies map[string]DenyEntry
}

func newMemStore() *memStore { return &memStore{denies: map[string]DenyEntry{}} }

func (m *memStore) LoadActiveDenies(now time.Time) map[string]time.Time {
	out := map[string]time.Time{}
	for cidr, e := range m.denies {
		if e.Until.IsZero() || e.Until.After(now) {
			out[cidr] = e.Until
		}
	}
	return out
}

func (m *memStore) AddDeny(cidr string, until *time.Time, reason string) error {
	e := DenyEntry{CIDR: cidr, Reason: reason}
	if until != nil {
		e.Until = *until
	}
	m.denies[cidr] = e
	return nil
}

func (m *memStore) RemoveDeny(cidr string) error {
	delete(m.denies, cidr)
	return nil
}

func (m *memStore) ListDenies(now time.Time) []DenyEntry {
	var out []DenyEntry
	for _, e := range m.denies {
		if e.Until.IsZero() || e.Until.After(now) {
			out = append(out, e)
		}
	}
	return out
}

func testGuard(t *testing.T, pol Policy) (*Guard, *memStore) {
	t.Helper()
	ms := newMemStore()
	return New(pol, ms), ms
}

func TestPerIPConcurrentCap(t *testing.T) {
	g, _ := testGuard(t, Policy{MaxConns: 100, MaxConnsPerIP: 2, NewConnsPerMin: 1000})
	for i := 0; i < 2; i++ {
		if ok, why := g.AllowConn("1.2.3.4:5555"); !ok {
			t.Fatalf("conn %d should be allowed: %s", i, why)
		}
	}
	if ok, _ := g.AllowConn("1.2.3.4:5556"); ok {
		t.Fatal("third concurrent conn from one IP must be refused")
	}
	// a different IP is unaffected
	if ok, _ := g.AllowConn("5.6.7.8:1234"); !ok {
		t.Fatal("different IP must not share the budget")
	}
	// releasing frees a slot for the capped IP
	g.ReleaseConn("1.2.3.4:5555")
	if ok, why := g.AllowConn("1.2.3.4:5557"); !ok {
		t.Fatalf("after release the IP must be admitted again: %s", why)
	}
}

func TestGlobalCapacity(t *testing.T) {
	g, _ := testGuard(t, Policy{MaxConns: 3, MaxConnsPerIP: 100, NewConnsPerMin: 1000})
	addrs := []string{"10.0.0.1:1", "10.0.0.2:1", "10.0.0.3:1", "10.0.0.4:1"}
	for _, a := range addrs[:3] {
		if ok, why := g.AllowConn(a); !ok {
			t.Fatalf("%s should fit under cap: %s", a, why)
		}
	}
	if ok, _ := g.AllowConn(addrs[3]); ok {
		t.Fatal("conn beyond MaxConns must be refused")
	}
	if g.Online() != 3 {
		t.Fatalf("Online()=%d want 3", g.Online())
	}
}

func TestAuthFailureBan(t *testing.T) {
	g, ms := testGuard(t, Policy{
		MaxConns: 100, MaxConnsPerIP: 100, NewConnsPerMin: 1000,
		MaxAuthFails: 5, BanDuration: time.Hour,
	})
	for i := 0; i < 4; i++ {
		g.RecordAuthFailure("9.9.9.9:1234")
	}
	if ok, _ := g.AllowConn("9.9.9.9:1235"); !ok {
		t.Fatal("4 failures must not yet ban")
	}
	g.RecordAuthFailure("9.9.9.9:1235")
	if ok, why := g.AllowConn("9.9.9.9:1236"); ok {
		t.Fatal("crossing MaxAuthFails must ban the address")
	} else if why == "" {
		t.Fatal("ban must carry a reason")
	}
	// ban is persisted so restarts keep it
	if len(ms.denies) == 0 {
		t.Fatal("auth-fail ban must be written to the store")
	}
	// the ban has an expiry
	for _, e := range ms.denies {
		if e.Until.IsZero() {
			t.Fatal("auth-fail ban must be temporary, not permanent")
		}
	}
}

func TestProbeBanAndLegitExemption(t *testing.T) {
	g, _ := testGuard(t, Policy{MaxConns: 100, MaxConnsPerIP: 100, NewConnsPerMin: 1000, ProbeBan: time.Hour})
	// scanner: three short-lived connections, never a handshake
	for i := 0; i < 3; i++ {
		g.finishConn("8.8.8.8:4000", time.Second)
	}
	if ok, _ := g.AllowConn("8.8.8.8:4001"); ok {
		t.Fatal("3 handshake-less short conns must probe-ban")
	}
	// legit user: same reconnect pattern, but they authenticated once
	g.RecordHandshake("44.44.44.44:4000")
	for i := 0; i < 5; i++ {
		g.finishConn("44.44.44.44:4000", time.Second)
	}
	if ok, _ := g.AllowConn("44.44.44.44:4001"); !ok {
		t.Fatal("a reconnecting authenticated user must never be probe-banned")
	}
	// a long-lived connection is not a probe regardless
	g.finishConn("55.55.55.55:4000", time.Hour)
	g.finishConn("55.55.55.55:4000", time.Hour)
	g.finishConn("55.55.55.55:4000", time.Hour)
	if ok, _ := g.AllowConn("55.55.55.55:4001"); !ok {
		t.Fatal("long-lived zero-channel conns must not count as probes")
	}
}

func TestRecordHandshakeClearsProbeState(t *testing.T) {
	g, _ := testGuard(t, Policy{MaxConns: 100, MaxConnsPerIP: 100, NewConnsPerMin: 1000, ProbeBan: time.Hour})
	g.finishConn("7.7.7.7:1", time.Second)
	g.finishConn("7.7.7.7:1", time.Second)
	g.RecordHandshake("7.7.7.7:1") // user authenticates
	g.finishConn("7.7.7.7:1", time.Second)
	g.finishConn("7.7.7.7:1", time.Second)
	if ok, _ := g.AllowConn("7.7.7.7:1"); !ok {
		t.Fatal("after a handshake, probe counter must not reach the ban threshold")
	}
}

func TestDenyAndAllowCIDRs(t *testing.T) {
	g, _ := testGuard(t, Policy{
		MaxConns: 100, MaxConnsPerIP: 100, NewConnsPerMin: 1000,
		DenyCIDRs: []string{"198.51.100.0/24"},
	})
	if ok, _ := g.AllowConn("198.51.100.7:1234"); ok {
		t.Fatal("deny CIDR must block")
	}
	if ok, _ := g.AllowConn("198.51.101.7:1234"); !ok {
		t.Fatal("adjacent range must be fine")
	}
	g2, _ := testGuard(t, Policy{
		MaxConns: 100, MaxConnsPerIP: 100, NewConnsPerMin: 1000,
		AllowCIDRs: []string{"10.0.0.0/8"},
	})
	if ok, _ := g2.AllowConn("10.1.2.3:1"); !ok {
		t.Fatal("allow-listed address must pass")
	}
	if ok, _ := g2.AllowConn("5.5.5.5:1"); ok {
		t.Fatal("address outside the allow list must be refused")
	}
}

func TestIPv6Aggregation(t *testing.T) {
	g, ms := testGuard(t, Policy{MaxConns: 100, MaxConnsPerIP: 100, NewConnsPerMin: 1000, BanDuration: time.Hour, MaxAuthFails: 1})
	g.RecordAuthFailure("[2001:db8:aaaa:bbbb::1]:1")
	// a different address in the SAME /64 is banned too
	if ok, _ := g.AllowConn("[2001:db8:aaaa:bbbb::ff]:1"); ok {
		t.Fatal("v6 /64 must be treated as one client")
	}
	// a different /64 is not
	if ok, _ := g.AllowConn("[2001:db8:aaaa:cccc::1]:1"); !ok {
		t.Fatal("adjacent /64 must be unaffected")
	}
	for cidr := range ms.denies {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			t.Fatalf("stored v6 key must be a CIDR, got %q", cidr)
		}
	}
}

func TestManualBanPersistsAndReloads(t *testing.T) {
	g, ms := testGuard(t, Policy{MaxConns: 100, MaxConnsPerIP: 100, NewConnsPerMin: 1000})
	if err := g.BanIP("203.0.113.5", "troll"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := g.AllowConn("203.0.113.5:1"); ok {
		t.Fatal("manual ban must block at accept")
	}
	// a fresh guard over the same store (restart) still bans
	g2 := New(Policy{MaxConns: 100, MaxConnsPerIP: 100, NewConnsPerMin: 1000}, ms)
	if ok, _ := g2.AllowConn("203.0.113.5:1"); ok {
		t.Fatal("ban must survive a restart via the store")
	}
	// live unban via Reload sees the removal
	if err := g2.UnbanIP("203.0.113.5"); err != nil {
		t.Fatal(err)
	}
	g.Reload()
	if ok, _ := g.AllowConn("203.0.113.5:1"); !ok {
		t.Fatal("after unban + Reload the address must be admitted")
	}
}

func TestExpiredAutoBanClears(t *testing.T) {
	g, _ := testGuard(t, Policy{MaxConns: 100, MaxConnsPerIP: 100, NewConnsPerMin: 1000, BanDuration: time.Millisecond, MaxAuthFails: 1})
	g.RecordAuthFailure("9.9.9.9:1")
	if ok, _ := g.AllowConn("9.9.9.9:2"); ok {
		t.Fatal("should be banned immediately")
	}
	time.Sleep(5 * time.Millisecond)
	if ok, _ := g.AllowConn("9.9.9.9:3"); !ok {
		t.Fatal("expired temp ban must stop blocking")
	}
}

func TestUnparseableAddrFailsOpen(t *testing.T) {
	g, _ := testGuard(t, Policy{MaxConns: 1, MaxConnsPerIP: 1, NewConnsPerMin: 1})
	if ok, _ := g.AllowConn("not-an-address"); !ok {
		t.Fatal("unparseable remote addr must fail OPEN, not lock out")
	}
}

func TestValidCIDR(t *testing.T) {
	for _, good := range []string{"10.0.0.0/8", "1.2.3.4", "::1", "2001:db8::/32"} {
		if err := ValidCIDR(good); err != nil {
			t.Errorf("%q should be valid: %v", good, err)
		}
	}
	for _, bad := range []string{"", "10.0.0.0/33", "abc", "1.2.3.4/33"} {
		if err := ValidCIDR(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}
