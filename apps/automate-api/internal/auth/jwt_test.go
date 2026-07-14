package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestNewTokenIssuer_EmptySecret(t *testing.T) {
	if _, err := NewTokenIssuer(""); !errors.Is(err, ErrEmptySecret) {
		t.Errorf("err = %v, want ErrEmptySecret", err)
	}
}

func TestJWT_RoundTrip(t *testing.T) {
	ti, err := NewTokenIssuer("test-secret")
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}
	tok, err := ti.Issue("usr_1", []string{"admin", "designer"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	claims, err := ti.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != "usr_1" {
		t.Errorf("subject = %q, want usr_1", claims.Subject)
	}
	if claims.Issuer != LocalIssuer {
		t.Errorf("issuer = %q, want %q", claims.Issuer, LocalIssuer)
	}
	if len(claims.Roles) != 2 || claims.Roles[0] != "admin" || claims.Roles[1] != "designer" {
		t.Errorf("roles = %v, want [admin designer]", claims.Roles)
	}
	if claims.ExpiresAt == nil {
		t.Fatal("ExpiresAt claim missing")
	}
	if d := claims.ExpiresAt.Sub(claims.IssuedAt.Time); d != TokenTTL {
		t.Errorf("token lifetime = %v, want %v", d, TokenTTL)
	}
}

func TestJWT_Expired(t *testing.T) {
	ti, _ := NewTokenIssuer("test-secret")
	// Issue a token 9h in the past → already expired (TTL is 8h).
	past := time.Now().Add(-9 * time.Hour)
	expiredIssuer := ti.WithClock(func() time.Time { return past })
	tok, err := expiredIssuer.Issue("usr_1", []string{"viewer"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	// Verify with the real (now) clock.
	_, err = ti.Verify(tok)
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, want ErrInvalidToken (expired)", err)
	}
}

func TestJWT_BadSignature(t *testing.T) {
	issuer, _ := NewTokenIssuer("secret-A")
	tok, _ := issuer.Issue("usr_1", []string{"admin"})

	other, _ := NewTokenIssuer("secret-B")
	if _, err := other.Verify(tok); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, want ErrInvalidToken (bad signature)", err)
	}
}

func TestJWT_WrongIssuer(t *testing.T) {
	secret := "shared-secret"
	// Hand-craft a token with the right signing key but a foreign issuer.
	claims := Claims{
		Roles: []string{"admin"},
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "some-other-issuer",
			Subject:   "usr_1",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	ti, _ := NewTokenIssuer(secret)
	if _, err := ti.Verify(raw); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, want ErrInvalidToken (wrong issuer)", err)
	}
}

func TestJWT_WrongSigningMethod(t *testing.T) {
	// A token with alg=none must be rejected (WithValidMethods HS256 only).
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    LocalIssuer,
			Subject:   "usr_1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	ti, _ := NewTokenIssuer("secret")
	if _, err := ti.Verify(raw); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, want ErrInvalidToken (alg none)", err)
	}
}

func TestJWT_Garbage(t *testing.T) {
	ti, _ := NewTokenIssuer("secret")
	if _, err := ti.Verify("not.a.jwt"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, want ErrInvalidToken (garbage)", err)
	}
}
