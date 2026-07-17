package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// LocalIssuer is the JWT issuer for local (dev) logins (docs/spec/07 §1.2). The
// API distinguishes providers by this claim; Entra-issued tokens carry a
// different issuer and are validated separately (E2-S1).
const LocalIssuer = "mywork-automate-local"

// TokenTTL is the local JWT lifetime (docs/spec/07 §1.2: "อายุสั้น 8 ชม.").
const TokenTTL = 8 * time.Hour

// Sentinel errors from the token functions.
var (
	// ErrEmptySecret is returned by NewTokenIssuer when no signing secret is set.
	ErrEmptySecret = errors.New("jwt signing secret must not be empty")
	// ErrInvalidToken wraps any verification failure (bad signature, expiry,
	// wrong issuer, malformed token). Callers should treat it as 401.
	ErrInvalidToken = errors.New("invalid token")
)

// Claims is the local JWT payload: the standard registered claims plus the
// caller's roles.
type Claims struct {
	Roles []string `json:"roles"`
	jwt.RegisteredClaims
}

// TokenIssuer issues and verifies local HS256 JWTs. It is safe for concurrent
// use. The secret is injected (never hardcoded — main reads AUTH_JWT_SECRET).
type TokenIssuer struct {
	secret []byte
	now    func() time.Time
}

// NewTokenIssuer builds a TokenIssuer signing with secret. It errors if secret
// is empty. The clock defaults to time.Now; tests override it via WithClock.
func NewTokenIssuer(secret string) (*TokenIssuer, error) {
	if secret == "" {
		return nil, ErrEmptySecret
	}
	return &TokenIssuer{secret: []byte(secret), now: time.Now}, nil
}

// WithClock returns a copy of the issuer using clk as its time source. Used by
// tests to produce expired tokens or advance validation time deterministically.
func (ti *TokenIssuer) WithClock(clk func() time.Time) *TokenIssuer {
	cp := *ti
	cp.now = clk
	return &cp
}

// Issue mints a signed HS256 token for userID with the given roles. subject is
// the userID; iss is LocalIssuer; exp is now+TokenTTL.
func (ti *TokenIssuer) Issue(userID string, roles []string) (string, error) {
	now := ti.now()
	claims := Claims{
		Roles: roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    LocalIssuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(TokenTTL)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(ti.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// Verify parses and validates a token string, returning its claims. It enforces
// the HS256 signing method, the LocalIssuer, and expiry (using the issuer's
// clock). Any failure returns a value wrapping ErrInvalidToken.
func (ti *TokenIssuer) Verify(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(LocalIssuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(ti.now),
	)
	_, err := parser.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		return ti.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	return claims, nil
}
