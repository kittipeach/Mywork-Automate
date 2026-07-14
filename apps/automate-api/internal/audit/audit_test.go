package audit

import (
	"context"
	"encoding/json"
	"testing"
)

func TestClampLimit(t *testing.T) {
	tests := []struct {
		name      string
		requested int
		want      int
	}{
		{"zero falls back to default", 0, DefaultLimit},
		{"negative falls back to default", -5, DefaultLimit},
		{"one is kept", 1, 1},
		{"below max is kept", 500, 500},
		{"exactly default is kept", DefaultLimit, DefaultLimit},
		{"exactly max is kept", MaxLimit, MaxLimit},
		{"above max is capped", MaxLimit + 1, MaxLimit},
		{"far above max is capped", 100000, MaxLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClampLimit(tt.requested); got != tt.want {
				t.Fatalf("ClampLimit(%d) = %d, want %d", tt.requested, got, tt.want)
			}
		})
	}
}

func TestClampLimitBounds(t *testing.T) {
	if DefaultLimit <= 0 || DefaultLimit > MaxLimit {
		t.Fatalf("DefaultLimit=%d must be in (0, MaxLimit=%d]", DefaultLimit, MaxLimit)
	}
}

func TestNoopRecord_IsNoOp(t *testing.T) {
	s := NewNoop()
	// Recording anything, including a fully-populated entry, must succeed and
	// change nothing observable (List still returns empty).
	if err := s.Record(context.Background(), Entry{Action: ActionAuthLogin, UserID: "u1"}); err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	got, err := s.List(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if got == nil {
		t.Fatal("List must never return a nil slice")
	}
	if len(got) != 0 {
		t.Fatalf("noop List = %d entries, want 0", len(got))
	}
}

func TestNoopList_NeverNil(t *testing.T) {
	s := NewNoop()
	// Any filter, including a fully-specified one, yields a non-nil empty slice.
	got, err := s.List(context.Background(), Filter{
		Action: ActionFlowPublish, From: "2026-01-01T00:00:00.000Z", To: "2026-12-31T00:00:00.000Z", Limit: 50,
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("noop List = %v, want non-nil empty", got)
	}
}

// TestEntryJSON pins the response wire shape: camelCase keys, omitempty on the
// optional fields, and detail serialised as a nested object.
func TestEntryJSON(t *testing.T) {
	e := Entry{
		ID:           7,
		UserID:       "user_1",
		Role:         "admin",
		Action:       ActionFileDownload,
		ResourceType: "file",
		ResourceID:   "file_42",
		Detail:       map[string]any{"bytes": float64(1024)},
		IP:           "10.0.0.1",
		UserAgent:    "curl/8",
		CreatedAt:    "2026-07-14T10:00:00.000Z",
	}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"id", "userId", "role", "action", "resourceType", "resourceId", "detail", "ip", "userAgent", "createdAt"} {
		if _, ok := back[k]; !ok {
			t.Errorf("marshalled entry missing key %q; got %s", k, raw)
		}
	}
	if back["role"] != "admin" || back["action"] != ActionFileDownload {
		t.Errorf("unexpected values: %s", raw)
	}
	detail, ok := back["detail"].(map[string]any)
	if !ok || detail["bytes"] != float64(1024) {
		t.Errorf("detail not serialised as nested object: %s", raw)
	}
}

// TestEntryJSON_OmitEmpty proves optional fields drop out but role/action/id/
// createdAt always render, so consumers can rely on their presence.
func TestEntryJSON_OmitEmpty(t *testing.T) {
	raw, err := json.Marshal(Entry{Role: "viewer", Action: ActionAuthLogout, CreatedAt: "2026-07-14T10:00:00.000Z"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"userId", "resourceType", "resourceId", "detail", "ip", "userAgent"} {
		if _, ok := back[k]; ok {
			t.Errorf("expected %q omitted when empty; got %s", k, raw)
		}
	}
	for _, k := range []string{"id", "role", "action", "createdAt"} {
		if _, ok := back[k]; !ok {
			t.Errorf("expected %q always present; got %s", k, raw)
		}
	}
}

// TestActionConstants guards the audited-event catalogue from docs/spec/07 §6
// against typos/regressions.
func TestActionConstants(t *testing.T) {
	want := map[string]string{
		"ActionAuthLogin":          "auth.login",
		"ActionAuthLogout":         "auth.logout",
		"ActionAuthFailed":         "auth.failed",
		"ActionFlowCreate":         "flow.create",
		"ActionFlowUpdate":         "flow.update",
		"ActionFlowPublish":        "flow.publish",
		"ActionFlowPause":          "flow.pause",
		"ActionFlowResume":         "flow.resume",
		"ActionFlowStop":           "flow.stop",
		"ActionFlowDelete":         "flow.delete",
		"ActionFlowRollback":       "flow.rollback",
		"ActionConnectionCreate":   "connection.create",
		"ActionConnectionUpdate":   "connection.update",
		"ActionConnectionDelete":   "connection.delete",
		"ActionConnectionTest":     "connection.test",
		"ActionExecutionManualRun": "execution.manual_run",
		"ActionExecutionCancel":    "execution.cancel",
		"ActionExecutionRetry":     "execution.retry",
		"ActionFileDownload":       "file.download",
		"ActionMaskingRuleChange":  "masking.rule_change",
		"ActionMaskingBypassView":  "masking.bypass_view",
		"ActionRBACGrantChange":    "rbac.grant_change",
		"ActionAICall":             "ai.call",
	}
	got := map[string]string{
		"ActionAuthLogin":          ActionAuthLogin,
		"ActionAuthLogout":         ActionAuthLogout,
		"ActionAuthFailed":         ActionAuthFailed,
		"ActionFlowCreate":         ActionFlowCreate,
		"ActionFlowUpdate":         ActionFlowUpdate,
		"ActionFlowPublish":        ActionFlowPublish,
		"ActionFlowPause":          ActionFlowPause,
		"ActionFlowResume":         ActionFlowResume,
		"ActionFlowStop":           ActionFlowStop,
		"ActionFlowDelete":         ActionFlowDelete,
		"ActionFlowRollback":       ActionFlowRollback,
		"ActionConnectionCreate":   ActionConnectionCreate,
		"ActionConnectionUpdate":   ActionConnectionUpdate,
		"ActionConnectionDelete":   ActionConnectionDelete,
		"ActionConnectionTest":     ActionConnectionTest,
		"ActionExecutionManualRun": ActionExecutionManualRun,
		"ActionExecutionCancel":    ActionExecutionCancel,
		"ActionExecutionRetry":     ActionExecutionRetry,
		"ActionFileDownload":       ActionFileDownload,
		"ActionMaskingRuleChange":  ActionMaskingRuleChange,
		"ActionMaskingBypassView":  ActionMaskingBypassView,
		"ActionRBACGrantChange":    ActionRBACGrantChange,
		"ActionAICall":             ActionAICall,
	}
	for name, wantVal := range want {
		if got[name] != wantVal {
			t.Errorf("%s = %q, want %q", name, got[name], wantVal)
		}
	}
}

// compile-time assertion that noop satisfies Service.
var _ Service = noop{}
