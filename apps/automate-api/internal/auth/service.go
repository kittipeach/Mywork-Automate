package auth

import (
	"errors"
	"time"
)

// Sentinel errors from the auth Service, mapped by the handler to HTTP status
// codes and audit outcomes.
var (
	// ErrInvalidCredentials is returned when the email is unknown or the password
	// does not match. The same error is used for both so the endpoint does not
	// leak which emails exist.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrAccountLocked is returned when the account is currently locked out.
	ErrAccountLocked = errors.New("account locked due to too many failed attempts")
)

// Service orchestrates a local login: lockout check → user lookup → password
// verify → JWT issue, updating the lockout tracker on each outcome. It is the
// single entry point the HTTP handler calls.
type Service struct {
	users   UserStore
	tokens  *TokenIssuer
	lockout *LockoutTracker
}

// NewService wires the collaborators. All three are required.
func NewService(users UserStore, tokens *TokenIssuer, lockout *LockoutTracker) *Service {
	return &Service{users: users, tokens: tokens, lockout: lockout}
}

// LoginResult is the outcome of a successful Authenticate call.
type LoginResult struct {
	Token  string
	UserID string
	Roles  []string
}

// Authenticate verifies email+password and returns a signed JWT on success.
//
// Order of checks (docs/spec/07 §1.2):
//  1. If the account is locked, return ErrAccountLocked (with the unlock time)
//     without touching the password — avoids extending the lock on every retry
//     and avoids leaking timing.
//  2. Look up the user; on miss, record a failure and return
//     ErrInvalidCredentials (same error as a bad password, no user enumeration).
//  3. Verify the password; on mismatch, record a failure and return
//     ErrInvalidCredentials.
//  4. On success, reset the failure counter and issue a token.
//
// The returned time is the unlock time and is only meaningful with
// ErrAccountLocked.
func (s *Service) Authenticate(email, password string) (LoginResult, time.Time, error) {
	if locked, until := s.lockout.IsLocked(email); locked {
		return LoginResult{}, until, ErrAccountLocked
	}

	user, err := s.users.Find(email)
	if err != nil {
		locked, until := s.lockout.RecordFailure(email)
		if locked {
			return LoginResult{}, until, ErrAccountLocked
		}
		return LoginResult{}, time.Time{}, ErrInvalidCredentials
	}

	ok, err := VerifyPassword(password, user.PasswordHash)
	if err != nil || !ok {
		locked, until := s.lockout.RecordFailure(email)
		if locked {
			return LoginResult{}, until, ErrAccountLocked
		}
		return LoginResult{}, time.Time{}, ErrInvalidCredentials
	}

	s.lockout.RecordSuccess(email)

	token, err := s.tokens.Issue(user.ID, user.Roles)
	if err != nil {
		return LoginResult{}, time.Time{}, err
	}
	return LoginResult{Token: token, UserID: user.ID, Roles: user.Roles}, time.Time{}, nil
}

// Verify exposes token verification for the RBAC middleware.
func (s *Service) Verify(token string) (*Claims, error) {
	return s.tokens.Verify(token)
}
