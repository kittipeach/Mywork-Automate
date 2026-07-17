package logscrub

import (
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Scrub — positives (each secret shape must be redacted)
// ---------------------------------------------------------------------------

func TestScrub_Positives(t *testing.T) {
	cases := []struct {
		name string
		in   string
		// mustNotContain is a secret fragment that must be gone after scrubbing.
		mustNotContain string
	}{
		{
			name:           "bearer token",
			in:             "GET /x Authorization header Bearer abcDEF123456ghatoken",
			mustNotContain: "abcDEF123456ghatoken",
		},
		{
			name:           "basic auth credentials",
			in:             "curl -H 'Authorization' Basic dXNlcjpwYXNzd29yZA==",
			mustNotContain: "dXNlcjpwYXNzd29yZA",
		},
		{
			name:           "authorization header colon form",
			in:             "req headers: Authorization: Token deadbeefcafefeed value",
			mustNotContain: "deadbeefcafefeed",
		},
		{
			name:           "password equals",
			in:             "conn string user=admin password=s3cr3tP@ss host=db",
			mustNotContain: "s3cr3tP@ss",
		},
		{
			name:           "password json",
			in:             `{"user":"admin","password":"hunter2secret"}`,
			mustNotContain: "hunter2secret",
		},
		{
			name:           "api_key equals",
			in:             "using api_key=AKIA1234567890value for request",
			mustNotContain: "AKIA1234567890value",
		},
		{
			name:           "apikey colon",
			in:             "apikey: myapikeyvalue123",
			mustNotContain: "myapikeyvalue123",
		},
		{
			name:           "secret pair",
			in:             "client_secret=abcSecretValue987 sent",
			mustNotContain: "abcSecretValue987",
		},
		{
			name:           "token pair",
			in:             "access_token=tok_livexyz123 next",
			mustNotContain: "tok_livexyz123",
		},
		{
			name:           "jwt triplet",
			in:             "auth eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c end",
			mustNotContain: "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
		},
		{
			name:           "long hex sha256",
			in:             "digest e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855 ok",
			mustNotContain: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:           "long base64",
			in:             "blob TWFueSBoYW5kcyBtYWtlIGxpZ2h0IHdvcmsgYW5kIG1vcmU= end",
			mustNotContain: "TWFueSBoYW5kcyBtYWtlIGxpZ2h0IHdvcmsgYW5kIG1vcmU",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Scrub(tc.in)
			if strings.Contains(got, tc.mustNotContain) {
				t.Fatalf("secret leaked: %q still contains %q", got, tc.mustNotContain)
			}
			if !strings.Contains(got, RedactedToken) {
				t.Fatalf("expected %q in output, got %q", RedactedToken, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Scrub — negatives (ordinary text must NOT be over-redacted)
// ---------------------------------------------------------------------------

func TestScrub_Negatives(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"password policy prose", "Please review my password policy before Friday."},
		{"secret word in prose", "The secret to success is consistency."},
		{"token word in prose", "Give me a token of appreciation."},
		{"short hex", "color code ff00aa and short id deadbeef here"},
		{"dotted identifier", "package a.b.c imported from module x.y"},
		{"normal url", "visit https://example.com/docs/getting-started/page"},
		{"plain sentence", "The quick brown fox jumps over the lazy dog."},
		{"short base64ish word", "status is authorized and everything works"},
		{"empty", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Scrub(tc.in)
			if got != tc.in {
				t.Fatalf("over-redacted: input %q became %q", tc.in, got)
			}
		})
	}
}

func TestScrub_Idempotent(t *testing.T) {
	in := "password=s3cr3tP@ss and Bearer abcDEF123456ghatoken"
	once := Scrub(in)
	twice := Scrub(once)
	if once != twice {
		t.Fatalf("Scrub not idempotent: %q != %q", once, twice)
	}
	if strings.Contains(once, "s3cr3tP@ss") || strings.Contains(once, "abcDEF123456ghatoken") {
		t.Fatalf("secret leaked after scrub: %q", once)
	}
}

func TestScrub_KeepsKeyLabel(t *testing.T) {
	// The credential-pair rule must keep the key so logs stay readable.
	got := Scrub("password=hunter2secretvalue")
	if !strings.HasPrefix(got, "password=") {
		t.Fatalf("expected key label preserved, got %q", got)
	}
	if strings.Contains(got, "hunter2secretvalue") {
		t.Fatalf("secret leaked: %q", got)
	}
}

// ---------------------------------------------------------------------------
// ScrubMap
// ---------------------------------------------------------------------------

func TestScrubMap_Nil(t *testing.T) {
	if got := ScrubMap(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestScrubMap_SensitiveKeys(t *testing.T) {
	in := map[string]any{
		"Password":      "anything at all",
		"api_key":       12345,        // non-string value under sensitive key
		"Authorization": "Bearer xyz", // still redacted wholesale
		"user":          "alice",
	}
	got := ScrubMap(in)

	for _, k := range []string{"Password", "api_key", "Authorization"} {
		if got[k] != RedactedToken {
			t.Errorf("key %q: expected %q, got %v", k, RedactedToken, got[k])
		}
	}
	if got["user"] != "alice" {
		t.Errorf("non-sensitive key altered: %v", got["user"])
	}
}

func TestScrubMap_ScrubsStringValues(t *testing.T) {
	in := map[string]any{
		"note": "connect with password=topSecretValue1 now",
	}
	got := ScrubMap(in)
	s, _ := got["note"].(string)
	if strings.Contains(s, "topSecretValue1") {
		t.Fatalf("secret leaked in string value: %q", s)
	}
}

func TestScrubMap_Recursive(t *testing.T) {
	in := map[string]any{
		"outer": map[string]any{
			"token": "shouldRedact",
			"list": []any{
				"password=deepSecretValue1",
				map[string]any{"secret": "nestedSecretVal"},
				42,
			},
		},
		"count": 7,
	}
	got := ScrubMap(in)

	outer := got["outer"].(map[string]any)
	if outer["token"] != RedactedToken {
		t.Errorf("nested sensitive key not redacted: %v", outer["token"])
	}
	list := outer["list"].([]any)
	if s := list[0].(string); strings.Contains(s, "deepSecretValue1") {
		t.Errorf("secret leaked in nested list string: %q", s)
	}
	inner := list[1].(map[string]any)
	if inner["secret"] != RedactedToken {
		t.Errorf("deep sensitive key not redacted: %v", inner["secret"])
	}
	if list[2] != 42 {
		t.Errorf("scalar in list altered: %v", list[2])
	}
	if got["count"] != 7 {
		t.Errorf("scalar altered: %v", got["count"])
	}
}

func TestScrubMap_DoesNotMutateInput(t *testing.T) {
	inner := map[string]any{"password": "origSecretValue"}
	in := map[string]any{
		"child": inner,
		"note":  "password=anotherSecretVal",
	}
	_ = ScrubMap(in)

	if inner["password"] != "origSecretValue" {
		t.Fatalf("input mutated: nested map changed to %v", inner["password"])
	}
	if in["note"] != "password=anotherSecretVal" {
		t.Fatalf("input mutated: note changed to %v", in["note"])
	}
}

func TestIsSensitiveKey(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"password", true},
		{"Password", true},
		{"db_password", true},
		{"api_key", true},
		{"apiKey", true},
		{"X-Api-Key", true},
		{"authorization", true},
		{"client_secret", true},
		{"access_key", true},
		{"private_key", true},
		{"token", true},
		{"username", false},
		{"note", false},
		{"count", false},
		{"description", false},
	}
	for _, tc := range cases {
		if got := IsSensitiveKey(tc.key); got != tc.want {
			t.Errorf("IsSensitiveKey(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestScrubMap_DeepCopyType(t *testing.T) {
	// Ensure the returned map is a distinct object (deep copy) even when empty.
	in := map[string]any{}
	got := ScrubMap(in)
	if reflect.ValueOf(got).Pointer() == reflect.ValueOf(in).Pointer() {
		t.Fatal("expected a distinct map instance")
	}
}
