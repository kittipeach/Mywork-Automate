package filestore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// hexSHA256 is the reference checksum computation the implementation must match.
func hexSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func newStore(t *testing.T) (*LocalFileStore, string) {
	t.Helper()
	dir := t.TempDir()
	fs, err := NewLocalFileStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileStore(%q) error = %v", dir, err)
	}
	return fs, dir
}

func TestNewLocalFileStore_CreatesMissingDir(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "nested", "created", "here")
	fs, err := NewLocalFileStore(root)
	if err != nil {
		t.Fatalf("NewLocalFileStore error = %v", err)
	}
	if fs == nil {
		t.Fatal("NewLocalFileStore returned nil store")
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatalf("root not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("root %q is not a directory", root)
	}
}

func TestNewLocalFileStore_EmptyDir(t *testing.T) {
	if _, err := NewLocalFileStore(""); err == nil {
		t.Fatal("NewLocalFileStore(\"\") expected error, got nil")
	}
}

func TestNewLocalFileStore_DirIsFile(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "iamafile")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLocalFileStore(file); err == nil {
		t.Fatal("NewLocalFileStore(existing file) expected error, got nil")
	}
}

func TestPutGet_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		path string
		data []byte
	}{
		{"simple", "hello.txt", []byte("hello world")},
		{"nested", "a/b/c/deep.bin", []byte("nested payload data")},
		{"empty", "empty.dat", []byte{}},
		{"binary", "img/blob.bin", []byte{0x00, 0x01, 0xff, 0xfe, 0x7f, 0x80}},
		{"leading-slash", "/rooted/key.txt", []byte("rooted content")},
		{"large", "big.bin", bytes.Repeat([]byte("A"), 1<<16)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs, _ := newStore(t)
			ctx := context.Background()

			ref, err := fs.Put(ctx, tt.path, bytes.NewReader(tt.data))
			if err != nil {
				t.Fatalf("Put error = %v", err)
			}
			if ref.Size != int64(len(tt.data)) {
				t.Errorf("ref.Size = %d, want %d", ref.Size, len(tt.data))
			}
			want := hexSHA256(tt.data)
			if ref.Checksum != want {
				t.Errorf("ref.Checksum = %q, want %q", ref.Checksum, want)
			}
			if len(ref.Checksum) != 64 {
				t.Errorf("checksum length = %d, want 64 hex chars", len(ref.Checksum))
			}

			rc, err := fs.Get(ctx, tt.path)
			if err != nil {
				t.Fatalf("Get error = %v", err)
			}
			got, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("ReadAll error = %v", err)
			}
			if err := rc.Close(); err != nil {
				t.Errorf("Close error = %v", err)
			}
			if !bytes.Equal(got, tt.data) {
				t.Errorf("round-trip bytes = %q, want %q", got, tt.data)
			}
			// Independently verify the on-disk bytes hash to the reported checksum.
			if hexSHA256(got) != ref.Checksum {
				t.Errorf("checksum of retrieved bytes %q != ref.Checksum %q", hexSHA256(got), ref.Checksum)
			}
		})
	}
}

func TestPut_ChecksumMatchesCryptoSHA256(t *testing.T) {
	fs, _ := newStore(t)
	data := []byte("banking-grade checksum verification payload \x00\x01\x02")
	ref, err := fs.Put(context.Background(), "check.bin", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Put error = %v", err)
	}
	sum := sha256.Sum256(data)
	if ref.Checksum != hex.EncodeToString(sum[:]) {
		t.Errorf("Checksum = %q, want %q", ref.Checksum, hex.EncodeToString(sum[:]))
	}
}

func TestPut_CreatesNestedDirs(t *testing.T) {
	fs, dir := newStore(t)
	_, err := fs.Put(context.Background(), "x/y/z/file.txt", strings.NewReader("data"))
	if err != nil {
		t.Fatalf("Put error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x", "y", "z", "file.txt")); err != nil {
		t.Fatalf("nested file not written: %v", err)
	}
}

func TestPut_Overwrite(t *testing.T) {
	fs, _ := newStore(t)
	ctx := context.Background()
	if _, err := fs.Put(ctx, "f.txt", strings.NewReader("first")); err != nil {
		t.Fatal(err)
	}
	ref, err := fs.Put(ctx, "f.txt", strings.NewReader("second-longer"))
	if err != nil {
		t.Fatalf("overwrite Put error = %v", err)
	}
	if ref.Checksum != hexSHA256([]byte("second-longer")) {
		t.Errorf("overwrite checksum mismatch")
	}
	rc, err := fs.Get(ctx, "f.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "second-longer" {
		t.Errorf("after overwrite = %q, want %q", got, "second-longer")
	}
}

// traversalPaths are keys that attempt to escape the store root. Every store
// operation must reject them.
var traversalPaths = []struct {
	name string
	path string
}{
	{"classic", "../../etc/passwd"},
	{"embedded", "a/b/../../../../etc/passwd"},
	{"dotdot-only", ".."},
	{"trailing-dotdot", "a/.."},
	{"leading-dotdot", "../inside"},
	{"absolute-escape-via-slash", "/../../etc/passwd"},
}

func TestPut_RejectsTraversal(t *testing.T) {
	fs, dir := newStore(t)
	ctx := context.Background()
	for _, tt := range traversalPaths {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fs.Put(ctx, tt.path, strings.NewReader("evil"))
			if err == nil {
				t.Fatalf("Put(%q) expected traversal rejection, got nil", tt.path)
			}
			if errors.Is(err, ErrNotFound) {
				t.Errorf("Put(%q) traversal should not be ErrNotFound", tt.path)
			}
		})
	}
	// Sanity: /etc/passwd on the host must not have been touched, and nothing
	// escaped the root. The temp dir's parent must contain no stray files
	// created by the traversal attempts.
	parent := filepath.Dir(dir)
	if _, err := os.Stat(filepath.Join(parent, "etc", "passwd")); err == nil {
		t.Fatal("traversal wrote outside root")
	}
}

func TestGet_RejectsTraversal(t *testing.T) {
	fs, _ := newStore(t)
	ctx := context.Background()
	for _, tt := range traversalPaths {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fs.Get(ctx, tt.path)
			if err == nil {
				t.Fatalf("Get(%q) expected traversal rejection, got nil", tt.path)
			}
		})
	}
}

func TestDelete_RejectsTraversal(t *testing.T) {
	fs, _ := newStore(t)
	ctx := context.Background()
	for _, tt := range traversalPaths {
		t.Run(tt.name, func(t *testing.T) {
			err := fs.Delete(ctx, tt.path)
			if err == nil {
				t.Fatalf("Delete(%q) expected traversal rejection, got nil", tt.path)
			}
		})
	}
}

func TestSignedURL_RejectsTraversal(t *testing.T) {
	fs, _ := newStore(t)
	ctx := context.Background()
	for _, tt := range traversalPaths {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fs.SignedURL(ctx, tt.path, time.Minute)
			if err == nil {
				t.Fatalf("SignedURL(%q) expected traversal rejection, got nil", tt.path)
			}
		})
	}
}

func TestPut_RejectsEmptyPath(t *testing.T) {
	fs, _ := newStore(t)
	if _, err := fs.Put(context.Background(), "", strings.NewReader("x")); err == nil {
		t.Fatal("Put(\"\") expected error, got nil")
	}
}

func TestGet_MissingReturnsNotFound(t *testing.T) {
	fs, _ := newStore(t)
	_, err := fs.Get(context.Background(), "does/not/exist.txt")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get missing err = %v, want ErrNotFound", err)
	}
}

func TestDelete_RoundTrip(t *testing.T) {
	fs, _ := newStore(t)
	ctx := context.Background()
	if _, err := fs.Put(ctx, "gone.txt", strings.NewReader("bye")); err != nil {
		t.Fatal(err)
	}
	if err := fs.Delete(ctx, "gone.txt"); err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	if _, err := fs.Get(ctx, "gone.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after Delete Get err = %v, want ErrNotFound", err)
	}
}

func TestDelete_MissingReturnsNotFound(t *testing.T) {
	fs, _ := newStore(t)
	err := fs.Delete(context.Background(), "nope.txt")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete missing err = %v, want ErrNotFound", err)
	}
}

func TestSignedURL_OK(t *testing.T) {
	fs, dir := newStore(t)
	ctx := context.Background()
	if _, err := fs.Put(ctx, "docs/report.pdf", strings.NewReader("pdf-bytes")); err != nil {
		t.Fatal(err)
	}
	raw, err := fs.SignedURL(ctx, "docs/report.pdf", 15*time.Minute)
	if err != nil {
		t.Fatalf("SignedURL error = %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("SignedURL returned unparseable URL %q: %v", raw, err)
	}
	if u.Scheme != "file" {
		t.Errorf("scheme = %q, want file", u.Scheme)
	}
	wantPath := filepath.Join(dir, "docs", "report.pdf")
	if u.Path != wantPath {
		t.Errorf("url path = %q, want %q", u.Path, wantPath)
	}
	// The URL must point at the real, readable file.
	if _, err := os.Stat(u.Path); err != nil {
		t.Errorf("SignedURL path does not exist: %v", err)
	}
}

func TestSignedURL_MissingReturnsNotFound(t *testing.T) {
	fs, _ := newStore(t)
	_, err := fs.SignedURL(context.Background(), "missing.txt", time.Minute)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("SignedURL missing err = %v, want ErrNotFound", err)
	}
}

func TestSignedURL_RejectsNonPositiveTTL(t *testing.T) {
	fs, _ := newStore(t)
	ctx := context.Background()
	if _, err := fs.Put(ctx, "a.txt", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	for _, ttl := range []time.Duration{0, -time.Second} {
		if _, err := fs.SignedURL(ctx, "a.txt", ttl); err == nil {
			t.Errorf("SignedURL ttl=%v expected error, got nil", ttl)
		}
	}
}

// errReader always fails; used to exercise the Put copy error path.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestPut_ReaderError(t *testing.T) {
	fs, dir := newStore(t)
	_, err := fs.Put(context.Background(), "bad.txt", errReader{})
	if err == nil {
		t.Fatal("Put with failing reader expected error, got nil")
	}
	// A failed Put must not leave a partial file behind.
	if _, statErr := os.Stat(filepath.Join(dir, "bad.txt")); statErr == nil {
		t.Error("partial file left behind after failed Put")
	}
}

func TestOperations_ContextCancelled(t *testing.T) {
	fs, _ := newStore(t)
	if _, err := fs.Put(context.Background(), "seed.txt", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := fs.Put(ctx, "a.txt", strings.NewReader("x")); !errors.Is(err, context.Canceled) {
		t.Errorf("Put cancelled err = %v, want context.Canceled", err)
	}
	if _, err := fs.Get(ctx, "seed.txt"); !errors.Is(err, context.Canceled) {
		t.Errorf("Get cancelled err = %v, want context.Canceled", err)
	}
	if _, err := fs.SignedURL(ctx, "seed.txt", time.Minute); !errors.Is(err, context.Canceled) {
		t.Errorf("SignedURL cancelled err = %v, want context.Canceled", err)
	}
	if err := fs.Delete(ctx, "seed.txt"); !errors.Is(err, context.Canceled) {
		t.Errorf("Delete cancelled err = %v, want context.Canceled", err)
	}
}

// TestPut_MkdirParentIsFile exercises the MkdirAll error path in Put: a parent
// path component already exists as a regular file, so the store cannot create
// the directory tree for the key.
func TestPut_MkdirParentIsFile(t *testing.T) {
	fs, _ := newStore(t)
	ctx := context.Background()
	// "a" becomes a file; then "a/b.txt" needs "a" to be a directory.
	if _, err := fs.Put(ctx, "a", strings.NewReader("i am a file")); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Put(ctx, "a/b.txt", strings.NewReader("x")); err == nil {
		t.Fatal("Put under a file parent expected error, got nil")
	}
}

// TestPut_KeyIsExistingDir exercises the OpenFile error path in Put: the key
// resolves to a path that already exists as a directory, so it cannot be opened
// for writing.
func TestPut_KeyIsExistingDir(t *testing.T) {
	fs, dir := newStore(t)
	if err := os.MkdirAll(filepath.Join(dir, "adir"), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Put(context.Background(), "adir", strings.NewReader("x")); err == nil {
		t.Fatal("Put to a key that is a directory expected error, got nil")
	}
}

// TestErrorPaths_NonNotExist exercises the non-ErrNotExist branches of Get,
// SignedURL and Delete by making a directory entry that the OS reports with a
// different error class than "does not exist".
func TestErrorPaths_NonNotExist(t *testing.T) {
	fs, dir := newStore(t)
	ctx := context.Background()

	// Get on a path that resolves to a directory: os.Open succeeds but reading
	// fails; more importantly Delete on a non-empty directory returns ENOTEMPTY
	// (not ErrNotExist), and Stat/Open on a permission-denied path returns
	// EACCES. We provoke EACCES by removing all permission bits on a subdir.
	if _, err := fs.Put(ctx, "locked/inside.txt", strings.NewReader("secret")); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(dir, "locked")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) })

	// Running as root bypasses permission bits; skip the assertion if so.
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits are bypassed")
	}

	if _, err := fs.Get(ctx, "locked/inside.txt"); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("Get on permission-denied path err = %v, want a non-NotFound error", err)
	}
	if _, err := fs.SignedURL(ctx, "locked/inside.txt", time.Minute); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("SignedURL on permission-denied path err = %v, want a non-NotFound error", err)
	}
	if err := fs.Delete(ctx, "locked/inside.txt"); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("Delete on permission-denied path err = %v, want a non-NotFound error", err)
	}
}

// FileStore is implemented by LocalFileStore.
var _ FileStore = (*LocalFileStore)(nil)
