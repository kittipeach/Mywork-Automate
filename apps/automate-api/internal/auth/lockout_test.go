package auth

import (
	"testing"
	"time"
)

// mockClock is an advanceable clock for lockout tests.
type mockClock struct{ t time.Time }

func (c *mockClock) now() time.Time      { return c.t }
func (c *mockClock) add(d time.Duration) { c.t = c.t.Add(d) }

func TestLockout_LocksAfterFiveFailures(t *testing.T) {
	clk := &mockClock{t: time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)}
	l := NewLockoutTracker(clk.now)
	const key = "user@x"

	// First 4 failures do not lock.
	for i := 1; i <= 4; i++ {
		locked, _ := l.RecordFailure(key)
		if locked {
			t.Fatalf("locked after %d failures, want lock only at %d", i, MaxFailedAttempts)
		}
		if isLocked, _ := l.IsLocked(key); isLocked {
			t.Fatalf("IsLocked true after %d failures", i)
		}
	}
	// 5th failure locks.
	locked, until := l.RecordFailure(key)
	if !locked {
		t.Fatal("not locked after 5th failure")
	}
	wantUntil := clk.now().Add(LockoutDuration)
	if !until.Equal(wantUntil) {
		t.Errorf("until = %v, want %v", until, wantUntil)
	}
	if isLocked, gotUntil := l.IsLocked(key); !isLocked || !gotUntil.Equal(wantUntil) {
		t.Errorf("IsLocked = (%v, %v), want (true, %v)", isLocked, gotUntil, wantUntil)
	}
}

func TestLockout_UnlocksAfter15Min(t *testing.T) {
	clk := &mockClock{t: time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)}
	l := NewLockoutTracker(clk.now)
	const key = "user@x"

	for i := 0; i < MaxFailedAttempts; i++ {
		l.RecordFailure(key)
	}
	if locked, _ := l.IsLocked(key); !locked {
		t.Fatal("expected locked after 5 failures")
	}

	// Just before the window elapses: still locked.
	clk.add(LockoutDuration - time.Second)
	if locked, _ := l.IsLocked(key); !locked {
		t.Fatal("unlocked too early")
	}

	// At/after the window: unlocked, and the counter is reset.
	clk.add(time.Second)
	if locked, _ := l.IsLocked(key); locked {
		t.Fatal("still locked after 15 minutes")
	}
	// A single failure now must not immediately re-lock (counter reset).
	locked, _ := l.RecordFailure(key)
	if locked {
		t.Fatal("failure counter not reset after unlock")
	}
}

func TestLockout_RecordSuccessResets(t *testing.T) {
	clk := &mockClock{t: time.Now()}
	l := NewLockoutTracker(clk.now)
	const key = "user@x"

	for i := 0; i < 4; i++ {
		l.RecordFailure(key)
	}
	l.RecordSuccess(key)
	// After a success, it should take 5 fresh failures to lock again.
	for i := 1; i <= 4; i++ {
		if locked, _ := l.RecordFailure(key); locked {
			t.Fatalf("locked after %d failures post-success, counter not reset", i)
		}
	}
	if locked, _ := l.RecordFailure(key); !locked {
		t.Fatal("expected lock on 5th failure after reset")
	}
}

func TestLockout_RecordFailureAfterExpiredLock_StartsFresh(t *testing.T) {
	clk := &mockClock{t: time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)}
	l := NewLockoutTracker(clk.now)
	const key = "user@x"

	for i := 0; i < MaxFailedAttempts; i++ {
		l.RecordFailure(key)
	}
	// Advance past the lock window, then record a failure directly (exercises the
	// reset branch inside RecordFailure, not just IsLocked).
	clk.add(LockoutDuration + time.Minute)
	locked, _ := l.RecordFailure(key)
	if locked {
		t.Fatal("RecordFailure re-locked immediately after expiry; counter not reset")
	}
}

func TestLockout_UnknownKeyNotLocked(t *testing.T) {
	l := NewLockoutTracker(nil) // defaults to time.Now
	if locked, _ := l.IsLocked("never-seen"); locked {
		t.Error("unknown key reported locked")
	}
}

func TestLockout_DefaultClock(t *testing.T) {
	// nil clock must default to time.Now without panicking.
	l := NewLockoutTracker(nil)
	if locked, _ := l.RecordFailure("k"); locked {
		t.Error("single failure locked with default clock")
	}
}
