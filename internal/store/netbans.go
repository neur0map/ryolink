package store

import (
	"database/sql"
	"time"

	"ryolink/internal/guard"
)

// Network bans (the guard's persistent deny list) live in net_bans and
// survive purges — an abuse ban that a weekly purge wipes is no ban at all.
// These methods implement guard.Store.

// scanExpiry reads an expires_at column that modernc may hand back either as
// a time.Time (DATETIME affinity) or a string, mapping both to our zero =
// permanent convention.
func scanExpiry(src any) time.Time {
	switch v := src.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		if v.IsZero() {
			return time.Time{}
		}
		return v.UTC()
	case string:
		if t, err := time.Parse("2006-01-02 15:04:05", v); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	return time.Time{}
}

// LoadActiveDenies returns every non-expired ban: cidr -> expiry (zero time =
// permanent). Expired rows are swept as a side effect.
func (s *Store) LoadActiveDenies(now time.Time) map[string]time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec(`DELETE FROM net_bans WHERE expires_at IS NOT NULL AND expires_at < ?`,
		now.UTC().Format("2006-01-02 15:04:05"))
	rows, err := s.db.Query(`SELECT cidr, expires_at FROM net_bans`)
	if err != nil {
		return map[string]time.Time{}
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var cidr string
		var exp any
		if err := rows.Scan(&cidr, &exp); err != nil {
			continue
		}
		out[cidr] = scanExpiry(exp)
	}
	return out
}

// AddDeny upserts a ban. until == nil or zero time = permanent.
func (s *Store) AddDeny(cidr string, until *time.Time, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var exp interface{}
	if until != nil && !until.IsZero() {
		exp = until.UTC().Format("2006-01-02 15:04:05")
	}
	_, err := s.db.Exec(`
		INSERT INTO net_bans (cidr, reason, banned_at, expires_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, ?)
		ON CONFLICT(cidr) DO UPDATE SET
			reason = excluded.reason,
			banned_at = excluded.banned_at,
			expires_at = excluded.expires_at
	`, cidr, reason, exp)
	return err
}

// RemoveDeny lifts a ban.
func (s *Store) RemoveDeny(cidr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM net_bans WHERE cidr = ?`, cidr)
	return err
}

// ListDenies returns active bans, newest first, for status output.
func (s *Store) ListDenies(now time.Time) []guard.DenyEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`
		SELECT cidr, reason, expires_at FROM net_bans
		WHERE expires_at IS NULL OR expires_at > ?
		ORDER BY banned_at DESC`,
		now.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []guard.DenyEntry
	for rows.Next() {
		var e guard.DenyEntry
		var reason sql.NullString
		var exp any
		if err := rows.Scan(&e.CIDR, &reason, &exp); err != nil {
			continue
		}
		e.Reason = reason.String
		e.Until = scanExpiry(exp)
		out = append(out, e)
	}
	return out
}
