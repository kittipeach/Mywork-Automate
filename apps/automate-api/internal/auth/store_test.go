package auth

import (
	"errors"
	"testing"
)

func TestMemUserStore_FindCaseInsensitive(t *testing.T) {
	store := NewMemUserStore(User{ID: "u1", Email: "Alice@Example.com", Roles: []string{"admin"}})
	for _, email := range []string{"Alice@Example.com", "alice@example.com", "ALICE@EXAMPLE.COM"} {
		u, err := store.Find(email)
		if err != nil {
			t.Fatalf("Find(%q): %v", email, err)
		}
		if u.ID != "u1" {
			t.Errorf("Find(%q) id = %q, want u1", email, u.ID)
		}
	}
}

func TestMemUserStore_NotFound(t *testing.T) {
	store := NewMemUserStore()
	if _, err := store.Find("nobody@x"); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
}

func TestSeedDevAdmin(t *testing.T) {
	store, err := SeedDevAdmin()
	if err != nil {
		t.Fatalf("SeedDevAdmin: %v", err)
	}
	u, err := store.Find(DevAdminEmail)
	if err != nil {
		t.Fatalf("seeded admin not found: %v", err)
	}
	if len(u.Roles) != 1 || u.Roles[0] != "admin" {
		t.Errorf("roles = %v, want [admin]", u.Roles)
	}
	ok, err := VerifyPassword(DevAdminPassword, u.PasswordHash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Error("seeded admin password does not verify")
	}
	// Wrong password must not verify.
	if ok, _ := VerifyPassword("wrong-password-xx", u.PasswordHash); ok {
		t.Error("seeded admin verified a wrong password")
	}
}
