package secrets

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingResolver is an in-memory Resolver that records how many times Resolve
// is invoked. It lets the caching tests prove real behaviour (hit vs miss vs
// expiry) instead of asserting on a mock's expectations.
type countingResolver struct {
	mu     sync.Mutex
	values map[string]string
	err    error        // if non-nil, Resolve returns this instead of a lookup
	calls  atomic.Int64 // total Resolve invocations
}

func (c *countingResolver) Resolve(_ context.Context, name string) (string, error) {
	c.calls.Add(1)
	if c.err != nil {
		return "", c.err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.values[name]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (c *countingResolver) count() int64 { return c.calls.Load() }

// --- FileResolver -----------------------------------------------------------

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.local.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func TestNewFileResolver_LoadOK(t *testing.T) {
	path := writeTempFile(t, "db-password: s3cr3t\napi-key: abc123\n")
	r, err := NewFileResolver(path)
	if err != nil {
		t.Fatalf("NewFileResolver: unexpected error: %v", err)
	}

	tests := []struct {
		name    string
		key     string
		want    string
		wantErr error
	}{
		{name: "known key db", key: "db-password", want: "s3cr3t"},
		{name: "known key api", key: "api-key", want: "abc123"},
		{name: "unknown key", key: "missing", wantErr: ErrNotFound},
		{name: "empty key", key: "", wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Resolve(context.Background(), tt.key)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Resolve(%q) err = %v, want Is %v", tt.key, err, tt.wantErr)
				}
				if got != "" {
					t.Fatalf("Resolve(%q) value = %q, want empty on error", tt.key, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve(%q) unexpected error: %v", tt.key, err)
			}
			if got != tt.want {
				t.Fatalf("Resolve(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestNewFileResolver_MissingFile(t *testing.T) {
	_, err := NewFileResolver(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("NewFileResolver: expected error for missing file, got nil")
	}
}

func TestNewFileResolver_BadYAML(t *testing.T) {
	// A YAML sequence cannot be unmarshalled into a map[string]string.
	path := writeTempFile(t, "- this\n- is\n- a list\n")
	_, err := NewFileResolver(path)
	if err == nil {
		t.Fatal("NewFileResolver: expected error for invalid YAML, got nil")
	}
}

func TestNewFileResolver_EmptyFile(t *testing.T) {
	// An empty file parses to a nil map; every lookup must return ErrNotFound.
	path := writeTempFile(t, "")
	r, err := NewFileResolver(path)
	if err != nil {
		t.Fatalf("NewFileResolver: unexpected error for empty file: %v", err)
	}
	if _, err := r.Resolve(context.Background(), "anything"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Resolve on empty file err = %v, want Is ErrNotFound", err)
	}
}

// --- CachingResolver --------------------------------------------------------

func TestCachingResolver_HitCallsInnerOnce(t *testing.T) {
	inner := &countingResolver{values: map[string]string{"k": "v"}}
	clock := time.Unix(0, 0)
	c := NewCachingResolver(inner, time.Minute, func() time.Time { return clock })

	for i := 0; i < 5; i++ {
		got, err := c.Resolve(context.Background(), "k")
		if err != nil {
			t.Fatalf("Resolve #%d: unexpected error: %v", i, err)
		}
		if got != "v" {
			t.Fatalf("Resolve #%d = %q, want %q", i, got, "v")
		}
	}
	if n := inner.count(); n != 1 {
		t.Fatalf("inner called %d times, want 1 (subsequent reads served from cache)", n)
	}
}

func TestCachingResolver_ExpiryRefetches(t *testing.T) {
	inner := &countingResolver{values: map[string]string{"k": "v"}}
	clock := time.Unix(0, 0)
	c := NewCachingResolver(inner, time.Minute, func() time.Time { return clock })

	if _, err := c.Resolve(context.Background(), "k"); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	// Still within TTL: served from cache.
	clock = clock.Add(59 * time.Second)
	if _, err := c.Resolve(context.Background(), "k"); err != nil {
		t.Fatalf("within-ttl resolve: %v", err)
	}
	if n := inner.count(); n != 1 {
		t.Fatalf("within TTL: inner called %d times, want 1", n)
	}
	// Past TTL: must refetch.
	clock = clock.Add(2 * time.Second) // now 61s, > 60s ttl
	if _, err := c.Resolve(context.Background(), "k"); err != nil {
		t.Fatalf("post-ttl resolve: %v", err)
	}
	if n := inner.count(); n != 2 {
		t.Fatalf("after expiry: inner called %d times, want 2 (refetch)", n)
	}
}

func TestCachingResolver_ErrorNotCached(t *testing.T) {
	// A resolver whose values map has no key returns ErrNotFound each time; the
	// cache must not remember the failure and must re-hit inner.
	inner := &countingResolver{values: map[string]string{}}
	clock := time.Unix(0, 0)
	c := NewCachingResolver(inner, time.Minute, func() time.Time { return clock })

	for i := 0; i < 3; i++ {
		_, err := c.Resolve(context.Background(), "missing")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("Resolve #%d err = %v, want Is ErrNotFound", i, err)
		}
	}
	if n := inner.count(); n != 3 {
		t.Fatalf("errors cached: inner called %d times, want 3 (errors never cached)", n)
	}
}

func TestCachingResolver_ArbitraryErrorNotCached(t *testing.T) {
	sentinel := errors.New("backend unavailable")
	inner := &countingResolver{err: sentinel}
	c := NewCachingResolver(inner, time.Minute, nil) // nil now => time.Now

	for i := 0; i < 2; i++ {
		_, err := c.Resolve(context.Background(), "k")
		if !errors.Is(err, sentinel) {
			t.Fatalf("Resolve #%d err = %v, want Is sentinel", i, err)
		}
	}
	if n := inner.count(); n != 2 {
		t.Fatalf("arbitrary error cached: inner called %d times, want 2", n)
	}
}

func TestCachingResolver_DefaultTTL(t *testing.T) {
	// ttl <= 0 must fall back to the 5-minute default from the spec.
	inner := &countingResolver{values: map[string]string{"k": "v"}}
	clock := time.Unix(0, 0)
	c := NewCachingResolver(inner, 0, func() time.Time { return clock })

	if _, err := c.Resolve(context.Background(), "k"); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	// 4m59s later: still cached under the 5-minute default.
	clock = clock.Add(5*time.Minute - time.Second)
	if _, err := c.Resolve(context.Background(), "k"); err != nil {
		t.Fatalf("within-default-ttl resolve: %v", err)
	}
	if n := inner.count(); n != 1 {
		t.Fatalf("default ttl: inner called %d times within 5m, want 1", n)
	}
	// Just past 5 minutes: refetch.
	clock = clock.Add(2 * time.Second)
	if _, err := c.Resolve(context.Background(), "k"); err != nil {
		t.Fatalf("post-default-ttl resolve: %v", err)
	}
	if n := inner.count(); n != 2 {
		t.Fatalf("default ttl expiry: inner called %d times, want 2", n)
	}
}

func TestCachingResolver_NilNowUsesWallClock(t *testing.T) {
	// With nil now and a real TTL, back-to-back reads are cache hits (wall clock
	// barely advances), proving the nil path defaults to time.Now and caches.
	inner := &countingResolver{values: map[string]string{"k": "v"}}
	c := NewCachingResolver(inner, time.Hour, nil)

	for i := 0; i < 4; i++ {
		if _, err := c.Resolve(context.Background(), "k"); err != nil {
			t.Fatalf("Resolve #%d: %v", i, err)
		}
	}
	if n := inner.count(); n != 1 {
		t.Fatalf("nil now: inner called %d times, want 1", n)
	}
}

func TestCachingResolver_DistinctKeysCachedIndependently(t *testing.T) {
	inner := &countingResolver{values: map[string]string{"a": "1", "b": "2"}}
	clock := time.Unix(0, 0)
	c := NewCachingResolver(inner, time.Minute, func() time.Time { return clock })

	for _, k := range []string{"a", "b", "a", "b"} {
		if _, err := c.Resolve(context.Background(), k); err != nil {
			t.Fatalf("Resolve(%q): %v", k, err)
		}
	}
	// Two distinct keys => two inner calls, then all cache hits.
	if n := inner.count(); n != 2 {
		t.Fatalf("distinct keys: inner called %d times, want 2", n)
	}
}

func TestCachingResolver_Concurrent(t *testing.T) {
	// Run under `go test -race` to catch data races on the shared cache map.
	inner := &countingResolver{values: map[string]string{"k": "v"}}
	clock := time.Unix(0, 0)
	c := NewCachingResolver(inner, time.Minute, func() time.Time { return clock })

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				got, err := c.Resolve(context.Background(), "k")
				if err != nil || got != "v" {
					t.Errorf("concurrent Resolve = %q, %v; want %q, nil", got, err, "v")
					return
				}
			}
		}()
	}
	wg.Wait()
}

// Compile-time assertions that both concrete types satisfy Resolver.
var (
	_ Resolver = (*FileResolver)(nil)
	_ Resolver = (*CachingResolver)(nil)
)
