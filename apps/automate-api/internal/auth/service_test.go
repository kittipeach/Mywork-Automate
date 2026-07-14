package auth

import (
	"errors"
	"testing"
	"time"
)

// newTestService builds a Service with a seeded user and an advanceable clock
// for the lockout tracker.
func newTestService(t *testing.T, clk func() time.Time) (*Service, string, string) {
	t.Helper()
	const email = "user@mywork.local"
	const pw = "correct-horse-1234"
	hash, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	users := NewMemUserStore(User{ID: "usr_1", Email: email, PasswordHash: hash, Roles: []string{"designer"}})
	tokens, err := NewTokenIssuer("svc-secret")
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}
	lock := NewLockoutTracker(clk)
	return NewService(users, tokens, lock), email, pw
}

func TestService_Authenticate_Success(t *testing.T) {
	svc, email, pw := newTestService(t, nil)
	res, _, err := svc.Authenticate(email, pw)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if res.UserID != "usr_1" {
		t.Errorf("userID = %q, want usr_1", res.UserID)
	}
	if res.Token == "" {
		t.Error("empty token on success")
	}
	claims, err := svc.Verify(res.Token)
	if err != nil {
		t.Fatalf("Verify issued token: %v", err)
	}
	if claims.Subject != "usr_1" || len(claims.Roles) != 1 || claims.Roles[0] != "designer" {
		t.Errorf("claims = %+v", claims)
	}
}

func TestService_Authenticate_WrongPassword(t *testing.T) {
	svc, email, _ := newTestService(t, nil)
	_, _, err := svc.Authenticate(email, "totally-wrong-99")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestService_Authenticate_UnknownUser(t *testing.T) {
	svc, _, _ := newTestService(t, nil)
	_, _, err := svc.Authenticate("ghost@x", "whatever-1234")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials (no user enumeration)", err)
	}
}

func TestService_Authenticate_LockoutAfterFive_UnlockAfter15m(t *testing.T) {
	clk := &mockClock{t: time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)}
	svc, email, pw := newTestService(t, clk.now)

	// 4 wrong attempts → still ErrInvalidCredentials.
	for i := 1; i <= 4; i++ {
		_, _, err := svc.Authenticate(email, "bad-password-000")
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d err = %v, want ErrInvalidCredentials", i, err)
		}
	}
	// 5th wrong attempt trips the lock.
	_, until, err := svc.Authenticate(email, "bad-password-000")
	if !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("5th attempt err = %v, want ErrAccountLocked", err)
	}
	if !until.Equal(clk.now().Add(LockoutDuration)) {
		t.Errorf("until = %v, want %v", until, clk.now().Add(LockoutDuration))
	}

	// Even the CORRECT password is refused while locked.
	if _, _, err := svc.Authenticate(email, pw); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("correct pw during lock err = %v, want ErrAccountLocked", err)
	}

	// Advance past the lockout window → correct password now succeeds.
	clk.add(LockoutDuration + time.Second)
	res, _, err := svc.Authenticate(email, pw)
	if err != nil {
		t.Fatalf("Authenticate after unlock: %v", err)
	}
	if res.Token == "" {
		t.Error("empty token after unlock")
	}
}

func TestService_Authenticate_UnknownUserCanTriggerLock(t *testing.T) {
	clk := &mockClock{t: time.Now()}
	svc, _, _ := newTestService(t, clk.now)
	var lastErr error
	for i := 0; i < MaxFailedAttempts; i++ {
		_, _, lastErr = svc.Authenticate("ghost@x", "bad-000000000")
	}
	if !errors.Is(lastErr, ErrAccountLocked) {
		t.Errorf("err = %v, want ErrAccountLocked after 5 unknown-user attempts", lastErr)
	}
}

func TestService_Authenticate_SuccessResetsFailureCounter(t *testing.T) {
	clk := &mockClock{t: time.Now()}
	svc, email, pw := newTestService(t, clk.now)
	// 4 bad attempts, then a good one, then 4 more bad ones must not lock.
	for i := 0; i < 4; i++ {
		svc.Authenticate(email, "bad-000000000")
	}
	if _, _, err := svc.Authenticate(email, pw); err != nil {
		t.Fatalf("good login: %v", err)
	}
	for i := 1; i <= 4; i++ {
		if _, _, err := svc.Authenticate(email, "bad-000000000"); errors.Is(err, ErrAccountLocked) {
			t.Fatalf("locked after %d post-success failures; counter not reset", i)
		}
	}
}
