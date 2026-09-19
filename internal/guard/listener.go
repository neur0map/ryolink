package guard

import (
	"net"
	"sync"
	"time"
)

// NewListener wraps ln with the guard's admission policy: every accepted
// connection is checked against the deny list and the connection budget, and
// its lifetime is reported back on close so port scanners get banned
// automatically while authenticated addresses are exempt.
func (g *Guard) NewListener(ln net.Listener) net.Listener {
	return &guardListener{Listener: ln, guard: g}
}

type guardedConn struct {
	net.Conn
	start  time.Time
	guard  *Guard
	mu     sync.Mutex
	closed bool
}

func (c *guardedConn) Close() error {
	err := c.Conn.Close()
	c.mu.Lock()
	reap := !c.closed
	c.closed = true
	c.mu.Unlock()
	if reap {
		addr := c.RemoteAddr().String()
		c.guard.finishConn(addr, time.Since(c.start))
	}
	return err
}

type guardListener struct {
	net.Listener
	guard *Guard
}

func (l *guardListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		addr := c.RemoteAddr().String()
		ok, reason := l.guard.AllowConn(addr)
		if !ok {
			// Refuse politely: write the reason, then hang up. Scanners do
			// not read it; it exists for humans who ssh -v out of curiosity.
			_ = c.SetWriteDeadline(time.Now().Add(2 * time.Second))
			_, _ = c.Write([]byte("ryolink: " + reason + "\n"))
			_ = c.Close()
			continue
		}
		return &guardedConn{Conn: c, start: time.Now(), guard: l.guard}, nil
	}
}

// finishConn is the single close-time hook: give back the budget slot and
// judge the connection's life for probe scoring.
func (g *Guard) finishConn(addr string, lived time.Duration) {
	g.ReleaseConn(addr)
	g.judgeProbe(addr, lived)
}

// judgeProbe treats a connection that died young from an address that has
// never completed an SSH handshake as one scanner probe; probeBanThreshold
// probes inside a rolling minute ban the address.
func (g *Guard) judgeProbe(addr string, lived time.Duration) {
	if lived > probeTTL {
		return
	}
	ip, ok := parseAddr(addr)
	if !ok {
		return
	}
	key := keyFor(ip)
	now := time.Now()
	var banUntil *time.Time

	g.mu.Lock()
	defer g.mu.Unlock()
	rec := g.recs[key]
	if rec == nil {
		rec = &ipRecord{}
		g.recs[key] = rec
	}
	if now.Sub(rec.lastHandshake) < legitTTL {
		// this address authenticates for real — flaky reconnects are not scans
		return
	}
	rec.prune(now)
	rec.probeWindow = append(rec.probeWindow, now)
	if len(rec.probeWindow) >= probeBanThreshold {
		until := now.Add(g.pol.ProbeBan)
		g.bans[key] = until
		rec.probeWindow = nil
		banUntil = &until
	}

	if banUntil != nil && g.st != nil {
		_ = g.st.AddDeny(key, banUntil, "scanner probes")
	}
}
