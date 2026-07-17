package auth

import (
	"sync"
	"time"
)

// Lockout policy (docs/spec/07 §1.2: "lockout 5 ครั้ง/15 นาที").
const (
	// MaxFailedAttempts is the number of consecutive failures that locks an
	// account.
	MaxFailedAttempts = 5
	// LockoutDuration is how long an account stays locked after the threshold is
	// reached.
	LockoutDuration = 15 * time.Minute
)

// LockoutTracker records failed login attempts per key (the user email/id) and
// reports whether a key is currently locked. It is in-memory only for MVP
// (documented; a DB-backed store lands with E2-S5 audit persistence) and safe
// for concurrent use. The clock is injectable so tests can advance time without
// sleeping.
type LockoutTracker struct {
	mu      sync.Mutex
	entries map[string]*lockEntry
	now     func() time.Time
}

type lockEntry struct {
	failures   int
	lockedTill time.Time // zero when not locked
}

// NewLockoutTracker builds a tracker. Pass nil for clk to use time.Now.
func NewLockoutTracker(clk func() time.Time) *LockoutTracker {
	if clk == nil {
		clk = time.Now
	}
	return &LockoutTracker{
		entries: make(map[string]*lockEntry),
		now:     clk,
	}
}

// IsLocked reports whether key is currently locked and, if so, until when. A
// lock that has expired is lazily cleared (its failure counter resets) so the
// next attempt starts fresh.
func (l *LockoutTracker) IsLocked(key string) (bool, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[key]
	if e == nil || e.lockedTill.IsZero() {
		return false, time.Time{}
	}
	if !l.now().Before(e.lockedTill) {
		// Lock window elapsed: reset so the account is usable again.
		delete(l.entries, key)
		return false, time.Time{}
	}
	return true, e.lockedTill
}

// RecordFailure registers a failed attempt for key. On reaching
// MaxFailedAttempts it locks the key for LockoutDuration. It returns the current
// locked state and, when locked, the unlock time.
func (l *LockoutTracker) RecordFailure(key string) (locked bool, until time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	e := l.entries[key]
	if e == nil {
		e = &lockEntry{}
		l.entries[key] = e
	}
	// If a previous lock has already expired, start a fresh count.
	if !e.lockedTill.IsZero() && !l.now().Before(e.lockedTill) {
		e.failures = 0
		e.lockedTill = time.Time{}
	}

	e.failures++
	if e.failures >= MaxFailedAttempts {
		e.lockedTill = l.now().Add(LockoutDuration)
		return true, e.lockedTill
	}
	return false, time.Time{}
}

// RecordSuccess clears any recorded failures for key (a successful login resets
// the counter).
func (l *LockoutTracker) RecordSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}
