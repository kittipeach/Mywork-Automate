// Package secrets resolves named secrets to their values for MyWork Automate
// (spec 04 §2.6, spec 07 §3, story E1-S5). The connections table stores only a
// keyvault_secret_name; the worker resolves the value at execute time and
// caches it for at most five minutes.
//
// Two production resolvers exist behind a single Resolver interface, selected by
// environment:
//
//   - Azure Key Vault (SIT and above). It reads secrets over AKS Workload
//     Identity (federated credential — no secret material in the cluster). That
//     concrete implementation requires a live Azure Workload Identity endpoint
//     and therefore cannot be built or tested in this environment; it is
//     intentionally deferred and added when Workload Identity is available. It
//     implements the same Resolver interface as the resolvers below, so it slots
//     in behind CachingResolver with no other change.
//   - FileResolver (dev/local). It reads a gitignored secrets.local.yaml file.
//
// CachingResolver wraps any Resolver with the spec-mandated five-minute TTL.
//
// Secret values are never logged or embedded in errors by this package.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ErrNotFound is returned when a named secret does not exist. Callers should
// match it with errors.Is.
var ErrNotFound = errors.New("secrets: not found")

// defaultTTL is the maximum cache lifetime for a resolved secret (spec 04 §2.6:
// worker resolves at execute with cache <= 5 min).
const defaultTTL = 5 * time.Minute

// Resolver resolves a named secret to its value. Implementations: Azure Key
// Vault (SIT+, added with Workload Identity) and FileResolver (dev).
type Resolver interface {
	Resolve(ctx context.Context, name string) (string, error)
}

// Writer persists a named secret. In prod this is Key Vault (a privileged,
// audited path); in dev FileResolver implements it against secrets.local.yaml so
// the admin UI can save an external-DB password without a Key Vault. Storing a
// secret via Writer is the ONLY sanctioned way credentials enter the system —
// they must never be written to the control-plane database.
type Writer interface {
	Write(ctx context.Context, name, value string) error
}

// FileResolver resolves (and, in dev, writes) secrets from a flat "name: value"
// YAML map. It is intended only for dev/local use; the file (secrets.local.yaml)
// is gitignored and never committed. Safe for concurrent Resolve/Write.
type FileResolver struct {
	mu     sync.RWMutex
	path   string
	values map[string]string
}

// NewFileResolver loads a gitignored YAML file mapping secret name -> value.
// Returns an error if the file cannot be read or parsed. The whole file is read
// eagerly so later Resolve calls never touch disk and never fail on I/O.
func NewFileResolver(path string) (*FileResolver, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is operator-provided config, not user input.
	if err != nil {
		return nil, fmt.Errorf("secrets: read file %q: %w", path, err)
	}
	values := make(map[string]string)
	if err := yaml.Unmarshal(data, &values); err != nil {
		// The parse error may quote offending YAML; it never contains a resolved
		// secret value keyed by this package, but we still avoid echoing content.
		return nil, fmt.Errorf("secrets: parse file %q: %w", path, err)
	}
	return &FileResolver{path: path, values: values}, nil
}

// Resolve returns the value for name, or ErrNotFound if the name is absent.
func (r *FileResolver) Resolve(_ context.Context, name string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.values[name]
	if !ok {
		return "", fmt.Errorf("secrets: %q: %w", name, ErrNotFound)
	}
	return v, nil
}

// Write upserts name=value in memory and persists the whole map back to the YAML
// file (0600). A subsequent Resolve on this resolver sees it immediately.
func (r *FileResolver) Write(_ context.Context, name, value string) error {
	if name == "" {
		return fmt.Errorf("secrets: write: empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[name] = value
	data, err := yaml.Marshal(r.values)
	if err != nil {
		return fmt.Errorf("secrets: marshal for write: %w", err)
	}
	if err := os.WriteFile(r.path, data, 0o600); err != nil {
		delete(r.values, name) // roll back the in-memory change on persist failure
		return fmt.Errorf("secrets: write file %q: %w", r.path, err)
	}
	return nil
}

// cacheEntry is a resolved value together with the instant it expires.
type cacheEntry struct {
	value     string
	expiresAt time.Time
}

// CachingResolver wraps an inner Resolver with a TTL cache. Successful resolves
// are cached for the TTL; errors are never cached. It is safe for concurrent
// use.
type CachingResolver struct {
	inner Resolver
	ttl   time.Duration
	now   func() time.Time

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// NewCachingResolver wraps inner with a TTL cache. `now` is injectable for tests
// (pass nil to use time.Now). Cache-miss and expired entries call inner; hits do
// not. A ttl <= 0 falls back to the five-minute default. Concurrency-safe.
func NewCachingResolver(inner Resolver, ttl time.Duration, now func() time.Time) *CachingResolver {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	if now == nil {
		now = time.Now
	}
	return &CachingResolver{
		inner: inner,
		ttl:   ttl,
		now:   now,
		cache: make(map[string]cacheEntry),
	}
}

// Resolve returns the value for name, serving unexpired cached hits without
// calling inner. On a miss or an expired entry it calls inner; successful
// results are cached for the TTL and errors are propagated without caching.
func (r *CachingResolver) Resolve(ctx context.Context, name string) (string, error) {
	now := r.now()

	r.mu.Lock()
	if e, ok := r.cache[name]; ok && now.Before(e.expiresAt) {
		r.mu.Unlock()
		return e.value, nil
	}
	r.mu.Unlock()

	// Miss or expired: fetch from inner outside the lock so a slow backend does
	// not serialize all callers. Errors are returned as-is and never cached, so
	// a transient failure or ErrNotFound re-hits inner on the next call.
	value, err := r.inner.Resolve(ctx, name)
	if err != nil {
		return "", err
	}

	r.mu.Lock()
	r.cache[name] = cacheEntry{value: value, expiresAt: r.now().Add(r.ttl)}
	r.mu.Unlock()

	return value, nil
}
