package scheduler

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/mywork/automate/internal/flowspec"
)

// scheduleNode builds a flow with a single trigger.schedule node carrying the
// given raw config, plus an unrelated node to prove selection is by type.
func scheduleNode(t *testing.T, cfg string) flowspec.FlowDef {
	t.Helper()
	return flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "n0", Type: "db.query", Name: "q", Config: json.RawMessage(`{}`)},
			{ID: "n1", Type: scheduleNodeType, Name: "sched", Config: json.RawMessage(cfg)},
		},
	}
}

func TestSpecFromDefinition_Simple(t *testing.T) {
	def := scheduleNode(t, `{"mode":"simple","everyMinutes":15}`)

	spec, has, err := SpecFromDefinition(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Fatal("hasSchedule = false, want true")
	}
	want := Spec{EveryMinutes: 15, Timezone: DefaultTimezone, OverlapPolicy: OverlapSkip}
	if spec != want {
		t.Fatalf("spec = %+v, want %+v", spec, want)
	}
}

func TestSpecFromDefinition_Cron(t *testing.T) {
	def := scheduleNode(t, `{"mode":"cron","cron":"0 8 * * MON","timezone":"UTC","overlapPolicy":"queue"}`)

	spec, has, err := SpecFromDefinition(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Fatal("hasSchedule = false, want true")
	}
	want := Spec{Cron: "0 8 * * MON", Timezone: "UTC", OverlapPolicy: OverlapQueue}
	if spec != want {
		t.Fatalf("spec = %+v, want %+v", spec, want)
	}
}

func TestSpecFromDefinition_NoScheduleNode(t *testing.T) {
	def := flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "n0", Type: "trigger.manual", Name: "m", Config: json.RawMessage(`{}`)},
		},
	}

	spec, has, err := SpecFromDefinition(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Fatal("hasSchedule = true, want false")
	}
	if spec != (Spec{}) {
		t.Fatalf("spec = %+v, want zero", spec)
	}
}

func TestSpecFromDefinition_Defaults(t *testing.T) {
	// No timezone/overlapPolicy => Asia/Bangkok + skip.
	def := scheduleNode(t, `{"mode":"simple","everyMinutes":1}`)

	spec, has, err := SpecFromDefinition(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Fatal("hasSchedule = false, want true")
	}
	if spec.Timezone != "Asia/Bangkok" {
		t.Errorf("timezone = %q, want Asia/Bangkok", spec.Timezone)
	}
	if spec.OverlapPolicy != OverlapSkip {
		t.Errorf("overlapPolicy = %q, want skip", spec.OverlapPolicy)
	}
}

func TestSpecFromDefinition_Errors(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr error
	}{
		{"every_minutes_zero", `{"mode":"simple","everyMinutes":0}`, ErrEveryMinutes},
		{"every_minutes_negative", `{"mode":"simple","everyMinutes":-5}`, ErrEveryMinutes},
		{"every_minutes_missing", `{"mode":"simple"}`, ErrEveryMinutes},
		{"empty_cron", `{"mode":"cron","cron":""}`, ErrEmptyCron},
		{"cron_missing", `{"mode":"cron"}`, ErrEmptyCron},
		{"unknown_mode", `{"mode":"weekly","everyMinutes":5}`, ErrUnknownMode},
		{"empty_mode", `{"everyMinutes":5}`, ErrUnknownMode},
		{"bad_overlap", `{"mode":"simple","everyMinutes":5,"overlapPolicy":"nope"}`, ErrOverlapPolicy},
		{"malformed_json", `{"mode":"simple",`, ErrInvalidConfig},
		{"wrong_type", `{"mode":"simple","everyMinutes":"ten"}`, ErrInvalidConfig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := scheduleNode(t, tt.config)
			spec, has, err := SpecFromDefinition(def)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if has {
				t.Errorf("hasSchedule = true, want false on error")
			}
			if spec != (Spec{}) {
				t.Errorf("spec = %+v, want zero on error", spec)
			}
		})
	}
}

func TestSpecFromDefinition_EmptyConfig(t *testing.T) {
	// A schedule node with a nil config decodes to an empty struct, which fails
	// mode validation rather than silently succeeding.
	def := flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "n1", Type: scheduleNodeType, Name: "sched", Config: nil},
		},
	}
	_, has, err := SpecFromDefinition(def)
	if !errors.Is(err, ErrUnknownMode) {
		t.Fatalf("err = %v, want ErrUnknownMode", err)
	}
	if has {
		t.Error("hasSchedule = true, want false on error")
	}
}

func TestSpecFromDefinition_MultipleScheduleNodes(t *testing.T) {
	def := flowspec.FlowDef{
		Nodes: []flowspec.NodeDef{
			{ID: "a", Type: scheduleNodeType, Name: "one", Config: json.RawMessage(`{"mode":"simple","everyMinutes":5}`)},
			{ID: "b", Type: scheduleNodeType, Name: "two", Config: json.RawMessage(`{"mode":"simple","everyMinutes":5}`)},
		},
	}
	_, has, err := SpecFromDefinition(def)
	if !errors.Is(err, ErrMultipleSchedule) {
		t.Fatalf("err = %v, want ErrMultipleSchedule", err)
	}
	if has {
		t.Error("hasSchedule = true, want false on error")
	}
}

func TestSpecFromDefinition_ParallelOverlap(t *testing.T) {
	def := scheduleNode(t, `{"mode":"simple","everyMinutes":30,"overlapPolicy":"parallel"}`)
	spec, _, err := SpecFromDefinition(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.OverlapPolicy != OverlapParallel {
		t.Fatalf("overlapPolicy = %q, want parallel", spec.OverlapPolicy)
	}
}
