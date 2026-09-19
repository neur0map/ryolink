// Package guard is the application-layer firewall for ryolink.
//
// The SSH server deliberately accepts any public key (the key IS the user's
// identity — there are no accounts), so the abuse controls a real sshd would
// provide must live here instead:
//
//   - concurrent connection budget (global + per-IP), reserved at TCP accept
//   - new-connection rate limit per IP (sliding one-minute window)
//   - config deny/allow CIDR lists
//   - auth-failure tracking per IP → temporary network ban
//   - probe detection: short-lived connections from addresses that never
//     complete a handshake (internet scanners cycling the port) → ban
//   - persistent deny list (config CIDRs + admin --deny + auto-bans), stored
//     in SQLite so bans survive restarts
package guard

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// Policy is the numeric knobs, resolved from config at startup.
type Policy struct {
	MaxConns       int
	MaxConnsPerIP  int
	NewConnsPerMin int
	MaxAuthFails   int
	BanDuration    time.Duration
	ProbeBan       time.Duration
	DenyCIDRs      []string
	AllowCIDRs     []string
}

func (p Policy) withDefaults() Policy {
	if p.MaxConns <= 0 {
		p.MaxConns = 512
	}
	if p.MaxConnsPerIP <= 0 {
		p.MaxConnsPerIP = 4
	}
	if p.NewConnsPerMin <= 0 {
		p.NewConnsPerMin = 30
	}
	if p.MaxAuthFails <= 0 {
		p.MaxAuthFails = 10
	}
	if p.BanDuration <= 0 {
		p.BanDuration = 30 * time.Minute
	}
	if p.ProbeBan <= 0 {
		p.ProbeBan = 60 * time.Minute
	}
	return p
}

// DenyEntry is one row of the persistent deny list.
type DenyEntry struct {
	CIDR   string
	Until  time.Time // zero = permanent
	Reason string
}

// Store is the persistent half of the deny list (SQLite in production, nil in
// tests). Keeping it an interface frees guard from database imports.
type Store interface {
	LoadActiveDenies(now time.Time) map[string]time.Time
	AddDeny(cidr string, until *time.Time, reason string) error
	RemoveDeny(cidr string) error
	ListDenies(now time.Time) []DenyEntry
}

type ipRecord struct {
	conns         int
	window        []time.Time // recent new-connection timestamps
	failWindow    []time.Time // recent auth-failure timestamps
	probeWindow   []time.Time // recent zero-handshake short-lived closes
	lastHandshake time.Time   // last time this address completed a real handshake
}

// probeTTL: a connection from a never-handshaked address younger than this is
// one probe. probeBanThreshold: probes within a minute that trigger a ban.
// legitTTL: how long a completed handshake exempts an address from probing.
const (
	probeTTL          = 5 * time.Second
	probeBanThreshold = 3
	legitTTL          = 24 * time.Hour
)

// prune drops the sliding windows back to entries newer than one minute.
func (r *ipRecord) prune(now time.Time) {
	cut := now.Add(-time.Minute)
	r.window = keepAfter(r.window, cut)
	r.failWindow = keepAfter(r.failWindow, cut)
	r.probeWindow = keepAfter(r.probeWindow, cut)
}

func keepAfter(ts []time.Time, cut time.Time) []time.Time {
	out := ts[:0]
	for _, t := range ts {
		if t.After(cut) {
			out = append(out, t)
		}
	}
	return out
}

// Guard is safe for concurrent use.
type Guard struct {
	pol   Policy
	st    Store
	mu    sync.Mutex
	recs  map[string]*ipRecord
	bans  map[string]time.Time // cidr/IP key -> ban expiry (zero = permanent)
	inUse int                  // global concurrent connections
	deny  []*net.IPNet         // always-blocked ranges (from config)
	allow []*net.IPNet         // when non-empty, ONLY these may connect
}

func New(pol Policy, st Store) *Guard {
	pol = pol.withDefaults()
	g := &Guard{pol: pol, st: st, recs: map[string]*ipRecord{}, bans: map[string]time.Time{}}
	for _, cidr := range pol.DenyCIDRs {
		if n, err := parseNet(cidr); err == nil {
			g.deny = append(g.deny, n)
		}
	}
	for _, cidr := range pol.AllowCIDRs {
		if n, err := parseNet(cidr); err == nil {
			g.allow = append(g.allow, n)
		}
	}
	if st != nil {
		for cidr, until := range st.LoadActiveDenies(time.Now()) {
			g.bans[cidr] = until
		}
	}
	return g
}

// ValidCIDR reports whether s is a CIDR prefix or bare IP address.
func ValidCIDR(s string) error {
	_, err := parseNet(s)
	return err
}

func parseNet(cidr string) (*net.IPNet, error) {
	if _, n, err := net.ParseCIDR(cidr); err == nil {
		return n, nil
	}
	if ip := net.ParseIP(cidr); ip != nil {
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		return &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits*8)}, nil
	}
	return nil, fmt.Errorf("invalid CIDR: %s", cidr)
}

// parseAddr pulls the IP out of a host:port (or bare) address, tolerating
// zone identifiers. ok=false on unparseable input — callers fail OPEN so a
// weird address format can never lock out real users.
func parseAddr(addr string) (net.IP, bool) {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	if zoneIdx := strings.IndexByte(host, '%'); zoneIdx >= 0 {
		host = host[:zoneIdx]
	}
	ip := net.ParseIP(host)
	return ip, ip != nil
}

// keyFor normalizes an IP to its deny-list key: v4 exact; v6 aggregated to
// /64 so a whole SLA/CPE prefix counts as one client (v6 users rotate within
// their /64 constantly).
func keyFor(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	if ip.To16() == nil {
		return ip.String()
	}
	base := make(net.IP, net.IPv6len)
	copy(base, ip.To16()[:8])
	return base.String() + "/64"
}

// AllowConn decides, at TCP accept time, whether addr may have another
// connection. It reserves a slot when returning true; the caller MUST pair
// every true with exactly one ReleaseConn(addr) when that connection closes.
func (g *Guard) AllowConn(addr string) (bool, string) {
	ip, ok := parseAddr(addr)
	if !ok {
		return true, ""
	}
	key := keyFor(ip)
	now := time.Now()

	g.mu.Lock()
	defer g.mu.Unlock()

	if netContains(g.deny, ip) {
		return false, "your address is denied by this ryolink"
	}
	if len(g.allow) > 0 && !netContains(g.allow, ip) {
		return false, "this ryolink is not accepting connections from your address"
	}
	if until, banned := g.isBannedLocked(ip, now); banned {
		return false, banReason(until)
	}
	if g.inUse >= g.pol.MaxConns {
		return false, "server is at capacity"
	}
	rec := g.recs[key]
	if rec == nil {
		rec = &ipRecord{}
		g.recs[key] = rec
	}
	rec.prune(now)
	if rec.conns >= g.pol.MaxConnsPerIP {
		return false, "too many connections from your address"
	}
	if len(rec.window) >= g.pol.NewConnsPerMin {
		return false, "connecting too fast"
	}
	rec.window = append(rec.window, now)
	rec.conns++
	g.inUse++
	return true, ""
}

// ReleaseConn gives back the slot reserved by AllowConn.
func (g *Guard) ReleaseConn(addr string) {
	ip, ok := parseAddr(addr)
	if !ok {
		return
	}
	key := keyFor(ip)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inUse > 0 {
		g.inUse--
	}
	if rec := g.recs[key]; rec != nil && rec.conns > 0 {
		rec.conns--
	}
}

// RecordAuthFailure logs a failed authentication from addr and escalates to a
// temporary network ban once the per-minute budget is exhausted.
func (g *Guard) RecordAuthFailure(addr string) {
	ip, ok := parseAddr(addr)
	if !ok {
		return
	}
	key := keyFor(ip)
	now := time.Now()
	var banUntil *time.Time

	g.mu.Lock()
	rec := g.recs[key]
	if rec == nil {
		rec = &ipRecord{}
		g.recs[key] = rec
	}
	rec.prune(now)
	rec.failWindow = append(rec.failWindow, now)
	if len(rec.failWindow) >= g.pol.MaxAuthFails {
		until := now.Add(g.pol.BanDuration)
		g.bans[key] = until
		rec.failWindow = nil
		banUntil = &until
	}
	g.mu.Unlock()

	if banUntil != nil && g.st != nil {
		_ = g.st.AddDeny(key, banUntil, "auth failures")
	}
}

// RecordHandshake marks addr as having completed a real SSH handshake (auth
// accepted). A recently-legit address is never probe-banned — flaky clients
// that reconnect quickly after authenticating would look like scanners.
func (g *Guard) RecordHandshake(addr string) {
	ip, ok := parseAddr(addr)
	if !ok {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	key := keyFor(ip)
	rec := g.recs[key]
	if rec == nil {
		rec = &ipRecord{}
		g.recs[key] = rec
	}
	rec.lastHandshake = time.Now()
	rec.probeWindow = nil
	rec.failWindow = nil
}

// Reload re-syncs the in-memory ban map from the persistent store, so live
// admin --deny / --undeny take effect without a restart.
func (g *Guard) Reload() {
	if g.st == nil {
		return
	}
	active := g.st.LoadActiveDenies(time.Now())
	g.mu.Lock()
	defer g.mu.Unlock()
	for cidr := range g.bans {
		if _, ok := active[cidr]; !ok {
			delete(g.bans, cidr)
		}
	}
	for cidr, until := range active {
		g.bans[cidr] = until
	}
}

// BanIP adds a manual (permanent) network ban.
func (g *Guard) BanIP(cidr, reason string) error {
	until := time.Time{} // zero = permanent
	g.mu.Lock()
	g.bans[cidr] = until
	g.mu.Unlock()
	if g.st != nil {
		return g.st.AddDeny(cidr, &until, reason)
	}
	return nil
}

// UnbanIP removes a network ban (manual or automatic).
func (g *Guard) UnbanIP(cidr string) error {
	g.mu.Lock()
	delete(g.bans, cidr)
	g.mu.Unlock()
	if g.st != nil {
		return g.st.RemoveDeny(cidr)
	}
	return nil
}

// DenyList returns the active bans for status output.
func (g *Guard) DenyList() []DenyEntry {
	if g.st != nil {
		return g.st.ListDenies(time.Now())
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]DenyEntry, 0, len(g.bans))
	for cidr, until := range g.bans {
		out = append(out, DenyEntry{CIDR: cidr, Until: until})
	}
	return out
}

// Online reports the current live connection count (for status).
func (g *Guard) Online() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.inUse
}

func netContains(nets []*net.IPNet, ip net.IP) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// isBannedLocked reports whether ip matches any active ban, expiring stale
// entries as it goes. Caller holds g.mu.
func (g *Guard) isBannedLocked(ip net.IP, now time.Time) (time.Time, bool) {
	for cidr, until := range g.bans {
		if !until.IsZero() && now.After(until) {
			delete(g.bans, cidr)
			continue
		}
		if cidrContains(cidr, ip) {
			return until, true
		}
	}
	return time.Time{}, false
}

func cidrContains(cidr string, ip net.IP) bool {
	if _, network, err := net.ParseCIDR(cidr); err == nil {
		return network.Contains(ip)
	}
	return cidr == ip.String() || cidr == keyFor(ip)
}

func banReason(until time.Time) string {
	if until.IsZero() {
		return "your address is banned from this ryolink"
	}
	return fmt.Sprintf("your address is temporarily banned (until %s)",
		until.Format(time.RFC3339))
}
