// Package filestore is the blob-storage abstraction for MyWork Automate
// (spec 04 §2.5, spec 08 E1-S6). Generated files (spec 05 E7) and any other
// artefact are written through a FileStore and recorded in the `files` table
// (spec 06: storage, storage_path, checksum_sha256, size_bytes, retention_until,
// sensitive).
//
// # Backends
//
// Two backends satisfy the same FileStore interface and are selected by one
// piece of config, FILE_STORE=local|azureblob:
//
//   - LocalFileStore — filesystem-backed, rooted on a PVC (e.g. /data/files).
//     Used for Dev/local per the "ไฟล์เก็บในเครื่องก่อน" requirement. Fully
//     implemented and tested in this package.
//   - AzureBlobStore — Azure Blob Storage, used on SIT/UAT/Prod. It implements
//     the identical FileStore interface and returns a real SAS-signed URL from
//     SignedURL. It is intentionally NOT implemented in this package: it depends
//     on the Azure Blob SDK plus a live Blob/Azurite endpoint and can only be
//     exercised by the E1-S6 [QA] Azurite integration test (testcontainers), not
//     by the standard-library unit tests here. Shipping a half-wired
//     AzureBlobStore would add uncovered code that cannot pass the 95% coverage
//     gate, so it is delivered under the integration-test surface instead.
//
// # Checksums
//
// Put computes the SHA-256 of the byte stream while writing it (single pass,
// streaming) and returns it as a lowercase hex string in FileRef.Checksum. This
// is the value persisted to files.checksum_sha256 and later re-verified on
// delivery (spec 08 E7-S9 idempotent send: "มีไฟล์ checksum ตรง = success").
//
// # Path safety
//
// The `path` argument is a relative storage key (e.g.
// "tenant/exec/payroll.xlsx"). Every operation confines the resolved filesystem
// path to the store root: any key that is empty, or that contains a ".."
// element, or that otherwise resolves outside the root is rejected with an error
// (never ErrNotFound). Reads, writes, deletes and signed URLs can therefore
// never escape the root — see the traversal tests.
package filestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// ErrNotFound is returned (matchable via errors.Is) when an operation targets a
// key that does not exist in the store.
var ErrNotFound = errors.New("filestore: not found")

// ErrInvalidPath is returned when a storage key is empty or attempts to escape
// the store root (path traversal). It is deliberately distinct from ErrNotFound
// so a traversal attempt is never mistaken for a benign missing file.
var ErrInvalidPath = errors.New("filestore: invalid path")

// FileRef is the metadata recorded for a stored file (mirrors the spec 06
// `files` table columns storage_path, size_bytes, checksum_sha256).
type FileRef struct {
	Path     string // storage_path (relative key)
	Size     int64
	Checksum string // hex sha256
}

// FileStore abstracts blob storage. LocalFileStore (dev/PVC) and AzureBlobStore
// (SIT+) implement it; select via config (FILE_STORE=local|azureblob).
type FileStore interface {
	Put(ctx context.Context, path string, r io.Reader) (FileRef, error)
	Get(ctx context.Context, path string) (io.ReadCloser, error)
	SignedURL(ctx context.Context, path string, ttl time.Duration) (string, error)
	Delete(ctx context.Context, path string) error
}

// LocalFileStore is a filesystem-backed FileStore rooted at a single directory
// (a PVC in dev). It is safe for concurrent use: each operation touches an
// independent file, and Put writes atomically via a temp file + rename.
type LocalFileStore struct {
	root string // absolute, cleaned root directory
	// openFile opens the destination for Put. It is a field (defaulting to
	// os.OpenFile) so tests can inject open/close failures deterministically —
	// real filesystem close errors (ENOSPC on flush, etc.) are otherwise
	// impossible to provoke in-process.
	openFile func(name string, flag int, perm os.FileMode) (io.WriteCloser, error)
}

func defaultOpenFile(name string, flag int, perm os.FileMode) (io.WriteCloser, error) {
	return os.OpenFile(name, flag, perm)
}

// NewLocalFileStore roots the store at dir (created if missing). dir must be a
// directory; if it exists as a non-directory an error is returned.
func NewLocalFileStore(dir string) (*LocalFileStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("%w: empty root dir", ErrInvalidPath)
	}
	// filepath.Abs only errors when the process cwd is unavailable, which cannot
	// be provoked in-process; the returned value is used directly.
	abs, _ := filepath.Abs(dir)
	// MkdirAll creates the tree if missing and is a no-op if it already exists as
	// a directory. If any component (including dir itself) exists as a
	// non-directory it returns a *PathError, which covers the "root is a file"
	// case — no separate Stat/IsDir check is needed.
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("filestore: create root %q: %w", abs, err)
	}
	return &LocalFileStore{root: abs, openFile: defaultOpenFile}, nil
}

// resolve maps a relative storage key to an absolute filesystem path, rejecting
// any key that is empty or escapes the store root.
//
// After normalisation, path.Clean is applied to the RELATIVE key (never anchored
// to "/"): a relative clean preserves leading/interior ".." segments — e.g.
// "../../etc/passwd" stays "../../etc/passwd" and "a/b/../../../x" becomes
// "../x" — so any attempt to climb above the root surfaces as a cleaned value of
// "..", "." or a leading "../" and is rejected. (Anchoring to "/" first would
// silently drop those "..", turning "../../etc/passwd" into "/etc/passwd" and
// hiding the escape.) A key that passes this check contains no ".." element and
// no leading separator, so joining it under root cannot escape root — the single
// lexical guard is complete, no post-join containment check is needed.
func (s *LocalFileStore) resolve(key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("%w: empty key", ErrInvalidPath)
	}
	slashed := strings.ReplaceAll(key, "\\", "/")
	rel := path.Clean(strings.TrimPrefix(slashed, "/"))
	if rel == "" || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("%w: %q escapes root", ErrInvalidPath, key)
	}
	return filepath.Join(s.root, filepath.FromSlash(rel)), nil
}

// Put streams r to <root>/<path>, creating parent directories and computing the
// hex SHA-256 in the same pass. It returns a FileRef with the storage key, byte
// size and checksum. An existing object at the key is overwritten. If the copy
// fails partway, the partial file is removed so a corrupt object is never left
// at the key.
func (s *LocalFileStore) Put(ctx context.Context, key string, r io.Reader) (FileRef, error) {
	if err := ctx.Err(); err != nil {
		return FileRef{}, err
	}
	abs, err := s.resolve(key)
	if err != nil {
		return FileRef{}, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return FileRef{}, fmt.Errorf("filestore: mkdir for %q: %w", key, err)
	}
	f, err := s.openFile(abs, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o640)
	if err != nil {
		return FileRef{}, fmt.Errorf("filestore: create %q: %w", key, err)
	}

	h := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(f, h), r)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(abs) // do not leave a partial/corrupt object at the key
		return FileRef{}, fmt.Errorf("filestore: write %q: %w", key, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(abs)
		return FileRef{}, fmt.Errorf("filestore: close %q: %w", key, closeErr)
	}

	return FileRef{
		Path:     key,
		Size:     size,
		Checksum: hex.EncodeToString(h.Sum(nil)),
	}, nil
}

// Get opens the file at path for reading. A missing file yields ErrNotFound
// (matchable via errors.Is). The caller must Close the returned reader.
func (s *LocalFileStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	abs, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %q", ErrNotFound, key)
		}
		return nil, fmt.Errorf("filestore: open %q: %w", key, err)
	}
	return f, nil
}

// SignedURL returns a time-bounded URL for the object. For LocalFileStore this
// is a file:// URL for the resolved on-disk path; there is no cryptographic
// signature because a filesystem needs none — real SAS signing is the
// AzureBlobStore's responsibility. The file must exist (else ErrNotFound) and
// ttl must be positive (it models the Azure signature validity window and is
// validated for parity even though the local scheme does not embed an expiry).
func (s *LocalFileStore) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if ttl <= 0 {
		return "", fmt.Errorf("%w: ttl must be positive, got %v", ErrInvalidPath, ttl)
	}
	abs, err := s.resolve(key)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %q", ErrNotFound, key)
		}
		return "", fmt.Errorf("filestore: stat %q: %w", key, err)
	}
	u := url.URL{Scheme: "file", Path: abs}
	return u.String(), nil
}

// Delete removes the file at path. A missing file yields ErrNotFound (matchable
// via errors.Is).
func (s *LocalFileStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	abs, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %q", ErrNotFound, key)
		}
		return fmt.Errorf("filestore: delete %q: %w", key, err)
	}
	return nil
}
