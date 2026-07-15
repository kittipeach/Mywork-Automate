package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"testing"

	"github.com/mywork/automate/apps/automate-api/internal/store"
	"github.com/mywork/automate/internal/config"
	"github.com/mywork/automate/internal/flowspec"
)

// captureHandler is a slog.Handler that records every emitted record's message,
// level and attributes so a test can assert the run-correlation INFO event.
type captureHandler struct {
	mu      sync.Mutex
	records []capturedRecord
}

type capturedRecord struct {
	level slog.Level
	msg   string
	attrs map[string]any
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	attrs := map[string]any{}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.records = append(h.records, capturedRecord{level: r.Level, msg: r.Message, attrs: attrs})
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

// find returns the first record with the given message, or nil.
func (h *captureHandler) find(msg string) *capturedRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.records {
		if h.records[i].msg == msg {
			return &h.records[i]
		}
	}
	return nil
}

// routerWithCapture builds a router whose logger writes to the capture handler so
// the E11-S3 run-correlation event can be asserted.
func routerWithCapture(st store.Store, run *fakeRunner, cap *captureHandler) http.Handler {
	logger := slog.New(cap)
	return NewRouter(config.Config{Env: config.EnvDev, FileStore: config.FileStoreLocal},
		st, run, nil, nil, AuthConfig{Logger: logger}, nil, nil)
}

// TestRunFlow_EmitsRunCorrelationLog asserts onDone emits a structured INFO event
// carrying run_id/flow_id/status/duration_ms (E11-S3) after recording.
func TestRunFlow_EmitsRunCorrelationLog(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	run := &fakeRunner{result: flowspec.FlowResult{Path: []string{"t1", "q1"}}}
	cap := &captureHandler{}
	r := routerWithCapture(fake, run, cap)

	w, body := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/run", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	execID, _ := body["executionId"].(string)

	rec := cap.find("flow run finished")
	if rec == nil {
		t.Fatalf("no run-correlation log emitted; records = %+v", cap.records)
	}
	if rec.level != slog.LevelInfo {
		t.Errorf("run-correlation level = %v, want INFO", rec.level)
	}
	if rec.attrs["run_id"] != execID {
		t.Errorf("run_id = %v, want the execution id %q", rec.attrs["run_id"], execID)
	}
	if rec.attrs["flow_id"] != "flw_payroll" {
		t.Errorf("flow_id = %v, want flw_payroll", rec.attrs["flow_id"])
	}
	if rec.attrs["status"] != "success" {
		t.Errorf("status = %v, want success", rec.attrs["status"])
	}
	if _, ok := rec.attrs["duration_ms"]; !ok {
		t.Errorf("run-correlation log missing duration_ms: %+v", rec.attrs)
	}
}

// A failed run still emits the correlation event, carrying status=failed.
func TestRunFlow_Failed_EmitsRunCorrelationLog(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	run := &fakeRunner{result: flowspec.FlowResult{Path: []string{"t1"}}, runErr: context.DeadlineExceeded}
	cap := &captureHandler{}
	r := routerWithCapture(fake, run, cap)

	doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/run", nil)

	rec := cap.find("flow run finished")
	if rec == nil {
		t.Fatalf("no run-correlation log emitted on failure; records = %+v", cap.records)
	}
	if rec.attrs["status"] != "failed" {
		t.Errorf("status = %v, want failed", rec.attrs["status"])
	}
}
