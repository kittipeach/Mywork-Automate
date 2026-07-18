package entra

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testIss = "https://login.microsoftonline.com/tenant/v2.0"
	testAud = "api://automate"
	testKid = "test-kid-1"
)

func genKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// sign builds an RS256 token with the given kid + claims.
func sign(t *testing.T, priv *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss":                testIss,
		"aud":                testAud,
		"sub":                "user-123",
		"preferred_username": "somchai@ttb.local",
		"roles":              []any{"admin", "designer"},
		"exp":                time.Now().Add(time.Hour).Unix(),
		"nbf":                time.Now().Add(-time.Minute).Unix(),
	}
}

func staticFetcher(kid string, pub *rsa.PublicKey) KeyFetcher {
	return func(context.Context) (map[string]*rsa.PublicKey, error) {
		return map[string]*rsa.PublicKey{kid: pub}, nil
	}
}

func newTestVerifier(priv *rsa.PrivateKey, opts ...Option) *Verifier {
	cfg := Config{JWKSURL: "https://jwks.example", Issuer: testIss, Audience: testAud}
	base := []Option{WithKeyFetcher(staticFetcher(testKid, &priv.PublicKey))}
	return New(cfg, append(base, opts...)...)
}

func TestVerify_Valid(t *testing.T) {
	priv := genKey(t)
	v := newTestVerifier(priv)
	claims, err := v.Verify(context.Background(), sign(t, priv, testKid, validClaims()))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != "user-123" {
		t.Errorf("subject = %q", claims.Subject)
	}
	if claims.Email != "somchai@ttb.local" {
		t.Errorf("email = %q", claims.Email)
	}
	if len(claims.Roles) != 2 || claims.Roles[0] != "admin" {
		t.Errorf("roles = %v", claims.Roles)
	}
}

func TestVerify_Rejections(t *testing.T) {
	priv := genKey(t)
	other := genKey(t)

	tests := []struct {
		name  string
		token func() string
	}{
		{"wrong issuer", func() string { c := validClaims(); c["iss"] = "https://evil"; return sign(t, priv, testKid, c) }},
		{"wrong audience", func() string { c := validClaims(); c["aud"] = "api://other"; return sign(t, priv, testKid, c) }},
		{"expired", func() string {
			c := validClaims()
			c["exp"] = time.Now().Add(-time.Hour).Unix()
			return sign(t, priv, testKid, c)
		}},
		{"no exp", func() string { c := validClaims(); delete(c, "exp"); return sign(t, priv, testKid, c) }},
		{"wrong signature", func() string { return sign(t, other, testKid, validClaims()) }},
		{"unknown kid", func() string { return sign(t, priv, "nope", validClaims()) }},
		{"no kid", func() string {
			tok := jwt.NewWithClaims(jwt.SigningMethodRS256, validClaims())
			s, _ := tok.SignedString(priv)
			return s
		}},
		{"garbage", func() string { return "not.a.jwt" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestVerifier(priv)
			if _, err := v.Verify(context.Background(), tc.token()); err == nil {
				t.Fatalf("%s: expected rejection", tc.name)
			}
		})
	}
}

func TestVerify_RejectsNonRS256(t *testing.T) {
	priv := genKey(t)
	// An HS256 token must be rejected (alg confusion defence via WithValidMethods).
	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims())
	hs.Header["kid"] = testKid
	s, _ := hs.SignedString([]byte("secret"))
	v := newTestVerifier(priv)
	if _, err := v.Verify(context.Background(), s); err == nil {
		t.Fatal("HS256 token must be rejected")
	}
}

func TestVerify_NilAndNotConfigured(t *testing.T) {
	var v *Verifier
	if _, err := v.Verify(context.Background(), "x"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil verifier err = %v, want ErrNotConfigured", err)
	}
	if New(Config{Issuer: testIss}) != nil {
		t.Fatal("New with incomplete config should return nil")
	}
	if !(Config{JWKSURL: "a", Issuer: "b", Audience: "c"}).Configured() {
		t.Fatal("fully-set config should be Configured")
	}
}

func TestVerify_RolesAbsentOrWrongType(t *testing.T) {
	priv := genKey(t)
	v := newTestVerifier(priv)
	c := validClaims()
	delete(c, "roles")
	c["roles"] = "admin" // wrong type (string, not array)
	claims, err := v.Verify(context.Background(), sign(t, priv, testKid, c))
	if err != nil {
		t.Fatal(err)
	}
	if claims.Roles != nil {
		t.Errorf("roles = %v, want nil for a non-array claim", claims.Roles)
	}
}

func TestKeyCache_RefreshesWhenStale(t *testing.T) {
	priv := genKey(t)
	calls := 0
	now := time.Now()
	fetch := func(context.Context) (map[string]*rsa.PublicKey, error) {
		calls++
		return map[string]*rsa.PublicKey{testKid: &priv.PublicKey}, nil
	}
	v := newTestVerifier(priv, WithKeyFetcher(fetch), WithCacheTTL(time.Minute), WithNow(func() time.Time { return now }))

	tok := sign(t, priv, testKid, validClaims())
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 fetch while cache warm, got %d", calls)
	}
	// advance past the TTL → next verify refetches
	now = now.Add(2 * time.Minute)
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected a refetch after TTL, got %d fetches", calls)
	}
}

func TestKeyCache_FetchError(t *testing.T) {
	priv := genKey(t)
	v := newTestVerifier(priv, WithKeyFetcher(func(context.Context) (map[string]*rsa.PublicKey, error) {
		return nil, errors.New("boom")
	}))
	if _, err := v.Verify(context.Background(), sign(t, priv, testKid, validClaims())); err == nil {
		t.Fatal("expected error when key fetch fails")
	}
}

func TestParseJWKS(t *testing.T) {
	priv := genKey(t)
	pub := &priv.PublicKey
	doc := map[string]any{
		"keys": []map[string]any{
			{
				"kid": testKid, "kty": "RSA",
				"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			},
			{"kid": "ec", "kty": "EC"},                              // non-RSA skipped
			{"kid": "bad-n", "kty": "RSA", "n": "!!!", "e": "AQAB"}, // bad base64-n skipped
			{"kid": "bad-e", "kty": "RSA", "n": "AQAB", "e": "!!!"}, // bad base64-e skipped
		},
	}
	b, _ := json.Marshal(doc)
	keys, err := ParseJWKS(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[testKid] == nil {
		t.Fatalf("expected 1 RSA key, got %d", len(keys))
	}
	if keys[testKid].N.Cmp(pub.N) != 0 || keys[testKid].E != pub.E {
		t.Error("parsed key does not match the source public key")
	}
	// malformed doc
	if _, err := ParseJWKS([]byte("{")); err == nil {
		t.Error("expected error for malformed JWKS")
	}
}

func jwksJSON(t *testing.T, kid string, pub *rsa.PublicKey) []byte {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"keys": []map[string]any{{
		"kid": kid, "kty": "RSA",
		"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}})
	return b
}

func TestVerify_ThroughHTTPJWKS(t *testing.T) {
	priv := genKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(jwksJSON(t, testKid, &priv.PublicKey))
	}))
	defer srv.Close()

	v := New(Config{JWKSURL: srv.URL, Issuer: testIss, Audience: testAud}, WithHTTPClient(srv.Client()))
	claims, err := v.Verify(context.Background(), sign(t, priv, testKid, validClaims()))
	if err != nil {
		t.Fatalf("Verify through HTTP JWKS: %v", err)
	}
	if claims.Subject != "user-123" {
		t.Errorf("subject = %q", claims.Subject)
	}
}

func TestHTTPJWKS_ErrorStatuses(t *testing.T) {
	priv := genKey(t)
	// non-200
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	v := New(Config{JWKSURL: bad.URL, Issuer: testIss, Audience: testAud}, WithHTTPClient(bad.Client()))
	if _, err := v.Verify(context.Background(), sign(t, priv, testKid, validClaims())); err == nil {
		t.Fatal("expected error on non-200 JWKS")
	}
	// unreachable URL
	v2 := New(Config{JWKSURL: "http://127.0.0.1:0/nope", Issuer: testIss, Audience: testAud})
	if _, err := v2.Verify(context.Background(), sign(t, priv, testKid, validClaims())); err == nil {
		t.Fatal("expected error on unreachable JWKS")
	}
}

func TestVerify_EmailAbsent(t *testing.T) {
	priv := genKey(t)
	v := newTestVerifier(priv)
	c := validClaims()
	delete(c, "preferred_username")
	claims, err := v.Verify(context.Background(), sign(t, priv, testKid, c))
	if err != nil {
		t.Fatal(err)
	}
	if claims.Email != "" {
		t.Errorf("email = %q, want empty", claims.Email)
	}
}

func TestHTTPJWKS_MalformedURL(t *testing.T) {
	priv := genKey(t)
	v := New(Config{JWKSURL: "http://[::1]:namedport", Issuer: testIss, Audience: testAud})
	if _, err := v.Verify(context.Background(), sign(t, priv, testKid, validClaims())); err == nil {
		t.Fatal("expected error building request for a malformed JWKS URL")
	}
}
