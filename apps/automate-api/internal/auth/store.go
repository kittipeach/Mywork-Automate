package auth

import (
	"errors"
	"strings"
)

// ErrUserNotFound is returned by UserStore.Find when no user matches the email.
var ErrUserNotFound = errors.New("user not found")

// User is a local (dev) login identity. PasswordHash is an argon2id encoded
// hash (see HashPassword). Roles are authz role strings.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	Roles        []string
}

// UserStore looks up local users by email. It is deliberately small: the local
// login path is dev-only, and the durable user directory in production is Entra
// (E2-S1). An in-memory implementation is provided below; a DB-backed one can
// satisfy the same interface later.
type UserStore interface {
	// Find returns the user with the given email (case-insensitive) or
	// ErrUserNotFound.
	Find(email string) (User, error)
}

// MemUserStore is an in-memory UserStore keyed by lower-cased email.
type MemUserStore struct {
	byEmail map[string]User
}

// NewMemUserStore builds a store from the given users.
func NewMemUserStore(users ...User) *MemUserStore {
	m := &MemUserStore{byEmail: make(map[string]User, len(users))}
	for _, u := range users {
		m.byEmail[strings.ToLower(u.Email)] = u
	}
	return m
}

// Find implements UserStore.
func (m *MemUserStore) Find(email string) (User, error) {
	u, ok := m.byEmail[strings.ToLower(email)]
	if !ok {
		return User{}, ErrUserNotFound
	}
	return u, nil
}

// DevAdminEmail is the seeded local admin's email (docs/spec/07 §1.2 seed).
const DevAdminEmail = "admin@mywork.local"

// DevAdminPassword is the seeded local admin's password. It satisfies the
// ≥12-char policy. This is a DEV-ONLY credential; the local-login path is
// unreachable in protected environments (config guard) so this never applies in
// SIT/UAT/prod.
const DevAdminPassword = "ChangeMe-Admin1"

// SeedDevAdmin builds an in-memory store containing a single admin user whose
// password is DevAdminPassword. It is called from main only when local auth is
// enabled. Returns an error only if hashing fails (never in practice).
func SeedDevAdmin() (*MemUserStore, error) {
	hash, err := HashPassword(DevAdminPassword)
	if err != nil {
		return nil, err
	}
	return NewMemUserStore(User{
		ID:           "usr_local_admin",
		Email:        DevAdminEmail,
		PasswordHash: hash,
		Roles:        []string{"admin"},
	}), nil
}
