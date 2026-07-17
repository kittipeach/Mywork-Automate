// Package auth implements the local (dev) authentication controls for
// MyWork Automate (docs/spec/07 §1.2): argon2id password hashing, short-lived
// HS256 JWTs issued under a dedicated local issuer, and a failed-attempt lockout
// tracker. It is used only when AUTH_LOCAL_ENABLED=true and the runtime guard
// permits it; production uses Entra ID (E2-S1).
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// MinPasswordLen is the local-login password policy floor (docs/spec/07 §1.2:
// "≥ 12 ตัว").
const MinPasswordLen = 12

// argon2id parameters. These follow OWASP's recommended baseline for argon2id
// and are encoded into every hash so verification is self-describing and the
// cost can be raised later without breaking existing hashes.
const (
	argonTime    = 1         // iterations
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// Sentinel errors from the password functions.
var (
	// ErrPasswordTooShort is returned by HashPassword when the password is
	// shorter than MinPasswordLen.
	ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", MinPasswordLen)
	// ErrInvalidHash is returned by VerifyPassword when the stored hash is not a
	// well-formed argon2id encoded hash.
	ErrInvalidHash = errors.New("invalid argon2id hash encoding")
	// ErrIncompatibleVersion is returned when the hash was produced by a newer
	// argon2 version than this build supports.
	ErrIncompatibleVersion = errors.New("incompatible argon2 version")
)

// HashPassword hashes plaintext with argon2id and returns the PHC-style encoded
// string ($argon2id$v=19$m=...,t=...,p=...$salt$hash). It enforces the
// MinPasswordLen policy, returning ErrPasswordTooShort otherwise.
func HashPassword(plaintext string) (string, error) {
	if len(plaintext) < MinPasswordLen {
		return "", ErrPasswordTooShort
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(plaintext), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(hash)), nil
}

// VerifyPassword reports whether plaintext matches the argon2id encodedHash. It
// recomputes the hash with the parameters embedded in encodedHash and compares
// in constant time. It returns (false, ErrInvalidHash) for a malformed hash and
// (false, nil) for a well-formed hash that simply does not match.
func VerifyPassword(plaintext, encodedHash string) (bool, error) {
	params, salt, hash, err := decodeHash(encodedHash)
	if err != nil {
		return false, err
	}
	computed := argon2.IDKey([]byte(plaintext), salt, params.time, params.memory, params.threads, uint32(len(hash))) //nolint:gosec // G115: len(hash) is a fixed 32-byte argon2 digest; cannot overflow uint32
	// subtle.ConstantTimeCompare returns 1 only when both length and bytes match.
	if subtle.ConstantTimeCompare(hash, computed) == 1 {
		return true, nil
	}
	return false, nil
}

type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

// decodeHash parses a PHC-style argon2id encoded hash back into its parameters,
// salt and hash bytes.
func decodeHash(encoded string) (argonParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// "" $ argon2id $ v=19 $ m=..,t=..,p=.. $ salt $ hash  → 6 parts (leading empty).
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return argonParams{}, nil, nil, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return argonParams{}, nil, nil, ErrInvalidHash
	}
	if version != argon2.Version {
		return argonParams{}, nil, nil, ErrIncompatibleVersion
	}

	var p argonParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return argonParams{}, nil, nil, ErrInvalidHash
	}

	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return argonParams{}, nil, nil, ErrInvalidHash
	}
	hash, err := b64.DecodeString(parts[5])
	if err != nil {
		return argonParams{}, nil, nil, ErrInvalidHash
	}
	if len(salt) == 0 || len(hash) == 0 {
		return argonParams{}, nil, nil, ErrInvalidHash
	}
	return p, salt, hash, nil
}
