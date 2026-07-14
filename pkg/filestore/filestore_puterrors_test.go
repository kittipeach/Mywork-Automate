package filestore

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

// errCloser writes fine but fails on Close, simulating a flush/ENOSPC error.
type errCloser struct{ closeErr error }

func (e errCloser) Write(p []byte) (int, error) { return len(p), nil }
func (e errCloser) Close() error                { return e.closeErr }

func TestPut_ContextCancelled(t *testing.T) {
	s, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Put(ctx, "a.txt", strings.NewReader("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Put with cancelled ctx = %v, want context.Canceled", err)
	}
}

func TestPut_MkdirError_FileInPath(t *testing.T) {
	s, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Create "a" as a file, then try to write "a/b" — MkdirAll(<root>/a) must fail
	// because <root>/a is a regular file, not a directory.
	if _, err := s.Put(context.Background(), "a", strings.NewReader("file")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(context.Background(), "a/b", strings.NewReader("x")); err == nil {
		t.Fatal("Put into a path shadowed by a file should fail, got nil")
	}
}

func TestPut_OpenError(t *testing.T) {
	s, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("boom-open")
	s.openFile = func(string, int, os.FileMode) (io.WriteCloser, error) { return nil, sentinel }
	if _, err := s.Put(context.Background(), "a.txt", strings.NewReader("x")); !errors.Is(err, sentinel) {
		t.Fatalf("Put open error = %v, want wraps boom-open", err)
	}
}

func TestPut_CloseError(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocalFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("boom-close")
	s.openFile = func(string, int, os.FileMode) (io.WriteCloser, error) {
		return errCloser{closeErr: sentinel}, nil
	}
	if _, err := s.Put(context.Background(), "a.txt", strings.NewReader("x")); !errors.Is(err, sentinel) {
		t.Fatalf("Put close error = %v, want wraps boom-close", err)
	}
	// The partial object must have been removed, not left at the key.
	if _, statErr := os.Stat(dir + "/a.txt"); !os.IsNotExist(statErr) {
		t.Fatalf("partial object left after close error: stat err = %v", statErr)
	}
}
