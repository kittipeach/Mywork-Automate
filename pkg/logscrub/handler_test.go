package logscrub

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// capture is the shared sink so WithAttrs-derived handlers report back to the
// same place the test holds a reference to.
type capture struct {
	msg     string
	records []map[string]any
}

// captureHandler is a minimal inner slog.Handler that records the last record
// it received, so tests can assert exactly what the wrapper delegated.
type captureHandler struct {
	sink  *capture
	attrs []slog.Attr
}

func newCapture() (*capture, *captureHandler) {
	s := &capture{}
	return s, &captureHandler{sink: s}
}

func (c *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (c *captureHandler) Handle(_ context.Context, r slog.Record) error {
	rec := map[string]any{}
	for _, a := range c.attrs {
		rec[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		rec[a.Key] = a.Value.Any()
		return true
	})
	c.sink.msg = r.Message
	c.sink.records = append(c.sink.records, rec)
	return nil
}

func (c *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &captureHandler{
		sink:  c.sink,
		attrs: append(append([]slog.Attr{}, c.attrs...), attrs...),
	}
}

func (c *captureHandler) WithGroup(string) slog.Handler { return c }

func TestNewHandler_NilPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on nil inner handler")
		}
	}()
	NewHandler(nil)
}

func TestHandler_ScrubsMessage(t *testing.T) {
	sink, inner := newCapture()
	logger := slog.New(NewHandler(inner))

	logger.Info("connecting with password=topSecretValue1 done")

	if strings.Contains(sink.msg, "topSecretValue1") {
		t.Fatalf("message not scrubbed: %q", sink.msg)
	}
	if !strings.Contains(sink.msg, RedactedToken) {
		t.Fatalf("expected redaction token in message: %q", sink.msg)
	}
}

func TestHandler_ScrubsStringAttrValue(t *testing.T) {
	sink, inner := newCapture()
	logger := slog.New(NewHandler(inner))

	logger.Info("hello", slog.String("detail", "Bearer abcDEF123456ghatoken"))

	last := sink.records[len(sink.records)-1]
	got, _ := last["detail"].(string)
	if strings.Contains(got, "abcDEF123456ghatoken") {
		t.Fatalf("attr value not scrubbed: %q", got)
	}
}

func TestHandler_RedactsSensitiveKey(t *testing.T) {
	sink, inner := newCapture()
	logger := slog.New(NewHandler(inner))

	logger.Info("login", slog.String("password", "whatever value"), slog.String("user", "alice"))

	last := sink.records[len(sink.records)-1]
	if last["password"] != RedactedToken {
		t.Fatalf("sensitive key not redacted: %v", last["password"])
	}
	if last["user"] != "alice" {
		t.Fatalf("non-sensitive attr altered: %v", last["user"])
	}
}

func TestHandler_NonStringScalarUnchanged(t *testing.T) {
	sink, inner := newCapture()
	logger := slog.New(NewHandler(inner))

	logger.Info("counted", slog.Int("count", 42))

	last := sink.records[len(sink.records)-1]
	if last["count"] != int64(42) {
		t.Fatalf("scalar attr altered: %v (%T)", last["count"], last["count"])
	}
}

func TestHandler_WithAttrsScrubsOnce(t *testing.T) {
	sink, inner := newCapture()
	logger := slog.New(NewHandler(inner)).With(
		slog.String("token", "leakedTokenValue"),
		slog.String("free", "password=preScrubbedSecret1"),
	)

	logger.Info("event")

	last := sink.records[len(sink.records)-1]
	if last["token"] != RedactedToken {
		t.Fatalf("WithAttrs sensitive key not redacted: %v", last["token"])
	}
	if s := last["free"].(string); strings.Contains(s, "preScrubbedSecret1") {
		t.Fatalf("WithAttrs string value not scrubbed: %q", s)
	}
}

func TestHandler_WithGroupScrubsGroupedAttr(t *testing.T) {
	sink, inner := newCapture()
	logger := slog.New(NewHandler(inner))

	logger.Info("grouped", slog.Group("ctx",
		slog.String("secret", "leakGroupSecret"),
		slog.String("ok", "value"),
	))

	last := sink.records[len(sink.records)-1]
	grp, ok := last["ctx"].([]slog.Attr)
	if !ok {
		t.Fatalf("expected grouped attr, got %T", last["ctx"])
	}
	for _, a := range grp {
		if a.Key == "secret" && a.Value.Any() != RedactedToken {
			t.Fatalf("grouped sensitive key not redacted: %v", a.Value.Any())
		}
	}
}

func TestHandler_EnabledDelegates(t *testing.T) {
	_, inner := newCapture()
	h := NewHandler(inner)
	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("expected Enabled to delegate true")
	}
}

func TestHandler_WithGroupReturnsWrapped(t *testing.T) {
	_, inner := newCapture()
	h := NewHandler(inner)
	if _, ok := h.WithGroup("g").(*Handler); !ok {
		t.Fatal("expected WithGroup to return *Handler")
	}
}

// TestHandler_EndToEndJSON proves the wrapper composes with the real
// slog.JSONHandler used in production and produces scrubbed JSON output.
func TestHandler_EndToEndJSON(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{})
	logger := slog.New(NewHandler(base)).With(slog.String("run_id", "run-123"))

	logger.Info("processing", slog.String("secret", "mustNotAppear"),
		slog.String("blob", "Bearer abcDEF123456ghatoken"))

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON produced: %v\n%s", err, buf.String())
	}
	if out["run_id"] != "run-123" {
		t.Errorf("run_id correlation lost: %v", out["run_id"])
	}
	if out["secret"] != RedactedToken {
		t.Errorf("sensitive key not redacted in JSON: %v", out["secret"])
	}
	if s, _ := out["blob"].(string); strings.Contains(s, "abcDEF123456ghatoken") {
		t.Errorf("secret leaked in JSON: %q", s)
	}
}
