package store

import (
	"errors"
	"testing"
)

func TestNotFound(t *testing.T) {
	err := NewNotFound("flow xyz not found")
	if err.Error() != "flow xyz not found" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if !IsNotFound(err) {
		t.Error("IsNotFound should be true for a NewNotFound error")
	}
	if IsNotFound(errors.New("other")) {
		t.Error("IsNotFound should be false for a plain error")
	}
	if IsNotFound(nil) {
		t.Error("IsNotFound(nil) should be false")
	}
}
