// Package entra validates Microsoft Entra ID (Azure AD) access tokens for
// production SSO (E2-S1). A token is a JWT signed by Entra with RS256; this
// package verifies the signature against the tenant's published JWKS (matched by
// `kid`), enforces the issuer, audience and expiry, and extracts the caller's
// app roles + identity. It is the prod counterpart to the dev local-auth issuer:
// resolveRole tries it when configured.
//
// The signing keys are fetched from the JWKS endpoint and cached; an unknown
// `kid` (a key rotation) or a stale cache triggers a refetch. The fetch is behind
// a seam (KeyFetcher) so the verifier is unit-tested without network or a real
// tenant.
package entra

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrNotConfigured is returned by a nil verifier's Verify so callers can treat
// "Entra not set up" uniformly.
var ErrNotConfigured = errors.New("entra: verifier not configured")

// Claims is the identity extracted from a verified Entra token.
type Claims struct {
	Subject string   // the `sub` claim (stable per user+app)
	Email   string   // preferred_username / upn / email, best-effort
	Roles   []string // Entra app roles (the `roles` claim)
}

// Config parameterises the verifier. All three are required in prod.
type Config struct {
	JWKSURL  string // e.g. https://login.microsoftonline.com/<tenant>/discovery/v2.0/keys
	Issuer   string // e.g. https://login.microsoftonline.com/<tenant>/v2.0
	Audience string // the API's application (client) id or api:// URI
}

// Configured reports whether all required fields are present.
func (c Config) Configured() bool {
	return c.JWKSURL != "" && c.Issuer != "" && c.Audience != ""
}

// KeyFetcher returns the current RSA signing keys by `kid`. Injected in tests;
// the default (httpKeyFetcher) fetches + parses the JWKS document.
type KeyFetcher func(ctx context.Context) (map[string]*rsa.PublicKey, error)

// Verifier validates Entra tokens against a cached key set.
type Verifier struct {
	cfg   Config
	fetch KeyFetcher
	ttl   time.Duration
	now   func() time.Time

	usingDefaultFetch bool // WithHTTPClient only rebuilds the default fetcher

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

// Option configures a Verifier.
type Option func(*Verifier)

// WithKeyFetcher overrides how signing keys are obtained (used by tests).
func WithKeyFetcher(f KeyFetcher) Option { return func(v *Verifier) { v.fetch = f } }

// WithNow overrides the clock (used by tests).
func WithNow(now func() time.Time) Option { return func(v *Verifier) { v.now = now } }

// WithCacheTTL overrides the key-cache lifetime (default 1h).
func WithCacheTTL(d time.Duration) Option { return func(v *Verifier) { v.ttl = d } }

// WithHTTPClient sets the client used by the default JWKS fetcher.
func WithHTTPClient(c *http.Client) Option {
	return func(v *Verifier) {
		if v.usingDefaultFetch {
			v.fetch = httpKeyFetcher(c, v.cfg.JWKSURL)
		}
	}
}

// New builds a Verifier. Returns nil when cfg is not fully configured, so callers
// can `if v := entra.New(cfg); v != nil` to enable Entra only in prod.
func New(cfg Config, opts ...Option) *Verifier {
	if !cfg.Configured() {
		return nil
	}
	v := &Verifier{
		cfg:               cfg,
		fetch:             httpKeyFetcher(http.DefaultClient, cfg.JWKSURL),
		ttl:               time.Hour,
		now:               time.Now,
		usingDefaultFetch: true,
	}
	for _, o := range opts {
		o(v)
	}
	return v
}

// Verify parses and validates token, returning the caller's claims. A nil
// receiver returns ErrNotConfigured.
func (v *Verifier) Verify(ctx context.Context, token string) (Claims, error) {
	if v == nil {
		return Claims{}, ErrNotConfigured
	}
	keyfunc := func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return v.key(ctx, kid)
	}
	parsed, err := jwt.Parse(token, keyfunc,
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(v.cfg.Issuer),
		jwt.WithAudience(v.cfg.Audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return Claims{}, fmt.Errorf("entra: token rejected: %w", err)
	}
	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return Claims{}, errors.New("entra: unexpected claims shape")
	}
	return Claims{
		Subject: strClaim(mc, "sub"),
		Email:   firstNonEmpty(strClaim(mc, "preferred_username"), strClaim(mc, "upn"), strClaim(mc, "email")),
		Roles:   stringsClaim(mc, "roles"),
	}, nil
}

// key returns the RSA public key for kid, refreshing the cache on a miss or when
// the cache is stale. A single retry after a refetch handles key rotation.
func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if kid == "" {
		return nil, errors.New("entra: token has no kid")
	}
	if k := v.cached(kid); k != nil {
		return k, nil
	}
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}
	if k := v.cached(kid); k != nil {
		return k, nil
	}
	return nil, fmt.Errorf("entra: no signing key for kid %q", kid)
}

func (v *Verifier) cached(kid string) *rsa.PublicKey {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.keys == nil || v.now().Sub(v.fetchedAt) > v.ttl {
		return nil
	}
	return v.keys[kid]
}

func (v *Verifier) refresh(ctx context.Context) error {
	keys, err := v.fetch(ctx)
	if err != nil {
		return fmt.Errorf("entra: fetch keys: %w", err)
	}
	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = v.now()
	v.mu.Unlock()
	return nil
}

// httpKeyFetcher fetches + parses the JWKS document from url. client is always
// non-nil (New / WithHTTPClient supply http.DefaultClient or an explicit one).
func httpKeyFetcher(client *http.Client, url string) KeyFetcher {
	return func(ctx context.Context) (map[string]*rsa.PublicKey, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("jwks status %d", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return nil, err
		}
		return ParseJWKS(body)
	}
}

// ParseJWKS decodes a JWKS document into RSA public keys by kid. Non-RSA and
// malformed keys are skipped. Exported so tests can build a fetcher from a
// generated JWKS.
func ParseJWKS(doc []byte) (map[string]*rsa.PublicKey, error) {
	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(doc, &jwks); err != nil {
		return nil, fmt.Errorf("parse jwks: %w", err)
	}
	out := make(map[string]*rsa.PublicKey)
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		nb, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eb, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		out[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: int(new(big.Int).SetBytes(eb).Int64())}
	}
	return out, nil
}

func strClaim(m jwt.MapClaims, k string) string {
	if s, ok := m[k].(string); ok {
		return s
	}
	return ""
}

func stringsClaim(m jwt.MapClaims, k string) []string {
	raw, ok := m[k].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if s, ok := r.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, s := range vals {
		if s != "" {
			return s
		}
	}
	return ""
}
