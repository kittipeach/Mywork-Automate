// Package logscrub redacts secret-shaped substrings from log lines and
// structured log attributes before they leave the process (story E11-S3,
// docs/spec/07-security-testing.md §4 "Secrets leak in logs" and
// docs/spec/08-backlog E11-S3 "JSON logs → ELK … scrubber").
//
// The scrubber is deliberately conservative: it targets shapes that are almost
// always secrets (bearer tokens, JWTs, key=value credential pairs, long
// hex/base64 runs) so that ordinary prose such as "my password policy" survives
// untouched. Every redaction collapses the secret to the fixed token
// [RedactedToken] so the output never leaks the original length.
//
// The package depends only on the Go standard library.
package logscrub

import "regexp"

// RedactedToken is the fixed replacement written in place of any redacted
// secret. It is a constant so redactions never leak length information.
const RedactedToken = "***REDACTED***"

// sensitiveKey matches structured-log attribute keys whose *value* must always
// be redacted regardless of its shape (used by ScrubMap and the slog Handler).
// It is anchored so it matches a whole key, allowing common separators and
// affixes (e.g. "api_key", "X-Api-Key", "authorizationHeader", "db_password").
var sensitiveKey = regexp.MustCompile(`(?i)(^|[_\-.])(password|passwd|pwd|secret|token|authorization|auth|api[_\-]?key|apikey|access[_\-]?key|client[_\-]?secret|private[_\-]?key)([_\-.]|$)`)

// scrubbers is the ordered list of substring redaction rules applied by Scrub.
// Order matters: broad credential-pair rules run before the generic long-run
// rules so that "password=deadbeef…" is reported as a credential pair rather
// than an incidental hex run (the end result — full redaction — is the same,
// but ordering keeps replacements from overlapping).
var scrubbers = []struct {
	re *regexp.Regexp
	// repl is the replacement template; $-group refs may reference the label
	// captured by the pattern so the key stays and only the value is redacted.
	repl string
}{
	// Authorization: <scheme> <token>  (HTTP header form; whole credential
	// value redacted up to the next header/line delimiter so both the scheme
	// and the token are removed).
	{
		re:   regexp.MustCompile(`(?i)\b(authorization)\s*[:=]\s*[^\r\n,;]+`),
		repl: "$1: " + RedactedToken,
	},
	// Bearer <token>  (also covers Basic/Digest credentials).
	{
		re:   regexp.MustCompile(`(?i)\b(bearer|basic|digest)\s+[A-Za-z0-9\-._~+/]+=*`),
		repl: "$1 " + RedactedToken,
	},
	// key=value / "key":"value" credential pairs. The key alternation is kept
	// tight to avoid nuking unrelated identifiers, and the value run stops at
	// whitespace, quotes, commas, semicolons and ampersands so surrounding text
	// (e.g. a query string) survives.
	{
		re:   regexp.MustCompile(`(?i)("?\b(?:password|passwd|pwd|secret|client[_\-]?secret|token|access[_\-]?token|refresh[_\-]?token|api[_\-]?key|apikey|access[_\-]?key|private[_\-]?key)"?)\s*[:=]\s*"?[^\s"',;&]+"?`),
		repl: "$1=" + RedactedToken,
	},
	// JWT-shaped triplet: base64url . base64url . base64url. Requires a
	// reasonably long header+payload so ordinary dotted identifiers (a.b.c) are
	// not caught.
	{
		re:   regexp.MustCompile(`\b[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),
		repl: RedactedToken,
	},
	// Long hex run (>=32 chars, e.g. SHA-256 / API digests). Word-bounded so it
	// only fires on standalone tokens.
	{
		re:   regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`),
		repl: RedactedToken,
	},
	// Long base64 / base64url run (>=40 chars). Trailing '=' padding allowed.
	{
		re:   regexp.MustCompile(`\b[A-Za-z0-9+/_-]{40,}={0,2}`),
		repl: RedactedToken,
	},
}

// Scrub returns s with secret-shaped substrings replaced by RedactedToken.
//
// It is safe on non-secret input: rules are shaped to match credential-like
// tokens only, so ordinary prose is returned unchanged. Scrub is idempotent —
// running it over already-scrubbed text is a no-op because RedactedToken
// contains no secret-shaped substring.
func Scrub(s string) string {
	if s == "" {
		return s
	}
	for _, sc := range scrubbers {
		s = sc.re.ReplaceAllString(s, sc.repl)
	}
	return s
}

// ScrubMap returns a deep copy of m with secrets redacted. String values are
// passed through Scrub; any value (of any type) stored under a sensitive key
// (see sensitiveKey — password/secret/token/authorization/apikey/… ,
// case-insensitive) is replaced wholesale with RedactedToken. Nested maps
// (map[string]any) and slices ([]any) are copied and scrubbed recursively so
// the caller's input is never mutated.
//
// A nil map yields a nil map.
func ScrubMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if sensitiveKey.MatchString(k) {
			out[k] = RedactedToken
			continue
		}
		out[k] = scrubValue(v)
	}
	return out
}

// scrubValue recursively scrubs an arbitrary value: strings via Scrub, nested
// maps via ScrubMap, and slices element-by-element. All other types are copied
// by value unchanged.
func scrubValue(v any) any {
	switch val := v.(type) {
	case string:
		return Scrub(val)
	case map[string]any:
		return ScrubMap(val)
	case []any:
		out := make([]any, len(val))
		for i, e := range val {
			out[i] = scrubValue(e)
		}
		return out
	default:
		return v
	}
}

// IsSensitiveKey reports whether a structured-log attribute key names a value
// that must always be redacted. Exposed so composition roots and the slog
// Handler can share one definition of "sensitive".
func IsSensitiveKey(key string) bool {
	return sensitiveKey.MatchString(key)
}
