package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashPassword_And_Verify_HappyPath(t *testing.T) {
	const pw = "correct horse battery staple" // ≥12 chars
	hash, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=") {
		t.Fatalf("hash not argon2id PHC-encoded: %q", hash)
	}
	ok, err := VerifyPassword(pw, hash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Error("VerifyPassword = false for the correct password")
	}
}

func TestHashPassword_SaltsAreUnique(t *testing.T) {
	const pw = "correct horse battery staple"
	h1, _ := HashPassword(pw)
	h2, _ := HashPassword(pw)
	if h1 == h2 {
		t.Error("two hashes of the same password are identical; salt not random")
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	hash, err := HashPassword("the-right-password-1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := VerifyPassword("the-wrong-password-9", hash)
	if err != nil {
		t.Fatalf("VerifyPassword returned error for well-formed hash: %v", err)
	}
	if ok {
		t.Error("VerifyPassword = true for the wrong password")
	}
}

func TestHashPassword_ShortRejected(t *testing.T) {
	tests := []string{"", "short", "elevenchars"} // all < 12
	for _, pw := range tests {
		_, err := HashPassword(pw)
		if !errors.Is(err, ErrPasswordTooShort) {
			t.Errorf("HashPassword(%q) err = %v, want ErrPasswordTooShort", pw, err)
		}
	}
	// exactly 12 is allowed
	if _, err := HashPassword("abcdefghijkl"); err != nil {
		t.Errorf("HashPassword(12 chars) err = %v, want nil", err)
	}
}

func TestVerifyPassword_MalformedHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{"empty", ""},
		{"not phc", "plaintext"},
		{"wrong algo", "$argon2i$v=19$m=65536,t=1,p=4$c2FsdA$aGFzaA"},
		{"missing parts", "$argon2id$v=19$m=65536,t=1,p=4$c2FsdA"},
		{"bad version field", "$argon2id$vX$m=65536,t=1,p=4$c2FsdA$aGFzaA"},
		{"bad params field", "$argon2id$v=19$mX$c2FsdA$aGFzaA"},
		{"bad salt b64", "$argon2id$v=19$m=65536,t=1,p=4$!!!!$aGFzaA"},
		{"bad hash b64", "$argon2id$v=19$m=65536,t=1,p=4$c2FsdA$!!!!"},
		{"empty salt/hash", "$argon2id$v=19$m=65536,t=1,p=4$$"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := VerifyPassword("whatever", tt.hash)
			if ok {
				t.Error("VerifyPassword ok = true for malformed hash")
			}
			if !errors.Is(err, ErrInvalidHash) {
				t.Errorf("err = %v, want ErrInvalidHash", err)
			}
		})
	}
}

func TestVerifyPassword_IncompatibleVersion(t *testing.T) {
	// A syntactically valid hash claiming a future argon2 version.
	hash := "$argon2id$v=99$m=65536,t=1,p=4$c2FsdHNhbHQ$aGFzaGhhc2g"
	_, err := VerifyPassword("whatever", hash)
	if !errors.Is(err, ErrIncompatibleVersion) {
		t.Errorf("err = %v, want ErrIncompatibleVersion", err)
	}
}
