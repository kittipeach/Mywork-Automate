package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/mywork/automate/apps/automate-api/internal/store"
	"github.com/mywork/automate/internal/config"
	"github.com/mywork/automate/pkg/secrets"
)

// fakeDialer records the DSN it was asked to dial and returns a canned error.
type fakeDialer struct {
	err     string
	lastDSN string
}

func (d *fakeDialer) Ping(_ context.Context, dsn string) error {
	d.lastDSN = dsn
	if d.err != "" {
		return errors.New(d.err)
	}
	return nil
}

// fakeSecrets is an in-memory secrets.Resolver + secrets.Writer.
type fakeSecrets struct{ m map[string]string }

func newFakeSecrets() *fakeSecrets { return &fakeSecrets{m: map[string]string{}} }
func (f *fakeSecrets) Resolve(_ context.Context, name string) (string, error) {
	v, ok := f.m[name]
	if !ok {
		return "", secrets.ErrNotFound
	}
	return v, nil
}
func (f *fakeSecrets) Write(_ context.Context, name, value string) error {
	f.m[name] = value
	return nil
}

func connRouter(t *testing.T, fake *fakeStore, dialer ConnDialer, sec *fakeSecrets) http.Handler {
	t.Helper()
	var w secrets.Writer
	var r secrets.Resolver
	if sec != nil {
		w, r = sec, sec
	}
	return NewRouter(config.Config{Env: config.EnvDev, FileStore: config.FileStoreLocal}, fake, nil, nil, nil, AuthConfig{}, nil, nil, WithConnDeps(w, r, dialer))
}

func TestTestConnection_LiveProbe(t *testing.T) {
	fake := seedFake()
	fake.conns = []store.Connection{{
		ID: "conn_ext", Name: "Ext", Type: "postgres", Host: "db", Port: 5432,
		Database: "app", Username: "reader", SecretRef: "pw", Status: "untested",
		AllowedRoles: []string{"admin"},
	}}
	sec := newFakeSecrets()
	sec.m["pw"] = "s3cr3t"

	// success
	d := &fakeDialer{}
	w, _ := doReqBody(t, connRouter(t, fake, d, sec), http.MethodPost, "/api/automate/v1/connections/conn_ext/test", ``, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("test = %d, want 200", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["status"] != "ok" {
		t.Fatalf("status = %v, want ok", body["status"])
	}
	if d.lastDSN != "postgres://reader:s3cr3t@db:5432/app?sslmode=disable" {
		t.Fatalf("dialed DSN = %q", d.lastDSN)
	}

	// dial failure → 200 with status error (never leaks the password)
	d2 := &fakeDialer{err: "connection refused"}
	w2, _ := doReqBody(t, connRouter(t, fake, d2, sec), http.MethodPost, "/api/automate/v1/connections/conn_ext/test", ``, nil)
	_ = json.Unmarshal(w2.Body.Bytes(), &body)
	if w2.Code != http.StatusOK || body["status"] != "error" {
		t.Fatalf("failed probe = %d %v, want 200 error", w2.Code, body["status"])
	}
	if msg, _ := body["message"].(string); containsSecret(msg) {
		t.Fatalf("probe message leaked the password: %q", msg)
	}
}

func containsSecret(s string) bool {
	for i := 0; i+6 <= len(s); i++ {
		if s[i:i+6] == "s3cr3t" {
			return true
		}
	}
	return false
}

func TestTestConnection_UnknownConnection(t *testing.T) {
	fake := seedFake()
	fake.conns = nil
	w, _ := doReqBody(t, connRouter(t, fake, &fakeDialer{}, newFakeSecrets()), http.MethodPost, "/api/automate/v1/connections/nope/test", ``, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown conn test = %d, want 404", w.Code)
	}
}

func TestTestConnection_NoDialerConfigured(t *testing.T) {
	fake := seedFake()
	fake.conns = []store.Connection{{ID: "c1", Type: "postgres", Host: "h", Database: "d", Username: "u", AllowedRoles: []string{"admin"}}}
	// dialer nil → "unknown"
	w, _ := doReqBody(t, connRouter(t, fake, nil, nil), http.MethodPost, "/api/automate/v1/connections/c1/test", ``, nil)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != http.StatusOK || body["status"] != "unknown" {
		t.Fatalf("no dialer = %d %v, want 200 unknown", w.Code, body["status"])
	}
}

func TestCreateConnection_SavesPasswordToSecretStore(t *testing.T) {
	fake := seedFake()
	sec := newFakeSecrets()
	body := `{"name":"Ext","type":"postgres","host":"db","port":5432,"database":"app","username":"reader","password":"topsecret"}`
	w, _ := doReqBody(t, connRouter(t, fake, &fakeDialer{}, sec), http.MethodPost, "/api/automate/v1/connections", body, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201: %s", w.Code, w.Body.String())
	}
	var conn store.Connection
	_ = json.Unmarshal(w.Body.Bytes(), &conn)
	if conn.SecretRef == "" {
		t.Fatal("expected a generated secretRef")
	}
	// the password was written to the secret store, never returned on the wire
	if got := sec.m[conn.SecretRef]; got != "topsecret" {
		t.Fatalf("secret store[%s] = %q, want topsecret", conn.SecretRef, got)
	}
	if containsWord(w.Body.String(), "topsecret") {
		t.Fatal("response leaked the raw password")
	}
}

func TestCreateConnection_PasswordWithoutSecretStore(t *testing.T) {
	// No secrets writer configured → saving a raw password is a 400, not a silent
	// plaintext store.
	fake := seedFake()
	body := `{"name":"Ext","type":"postgres","host":"db","database":"app","username":"u","password":"x"}`
	w, _ := doReqBody(t, connRouter(t, fake, &fakeDialer{}, nil), http.MethodPost, "/api/automate/v1/connections", body, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("password without store = %d, want 400", w.Code)
	}
}

func containsWord(s, w string) bool {
	for i := 0; i+len(w) <= len(s); i++ {
		if s[i:i+len(w)] == w {
			return true
		}
	}
	return false
}
