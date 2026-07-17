package scheduler

import (
	"errors"
	"testing"
	"time"

	"go.temporal.io/sdk/client"
)

func TestScheduleID(t *testing.T) {
	if got := scheduleID("abc"); got != "flow-abc" {
		t.Fatalf("scheduleID = %q, want flow-abc", got)
	}
	if got := scheduleID(""); got != "flow-" {
		t.Fatalf("scheduleID(empty) = %q, want flow-", got)
	}
}

func TestMapOverlap(t *testing.T) {
	tests := []struct {
		policy  string
		want    int
		wantErr bool
	}{
		{OverlapSkip, overlapSkipEnum, false},
		{OverlapQueue, overlapBufferOneNum, false},
		{OverlapParallel, overlapAllowAllNum, false},
		{"unknown", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.policy, func(t *testing.T) {
			got, err := mapOverlap(tt.policy)
			if tt.wantErr {
				if !errors.Is(err, ErrOverlapPolicy) {
					t.Fatalf("err = %v, want ErrOverlapPolicy", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("mapOverlap(%q) = %d, want %d", tt.policy, got, tt.want)
			}
		})
	}
}

// overlapEnum mirrors the SDK's ScheduleOverlapPolicy (an int32-based enum) so
// setScheduleOverlap's generic constraint can be exercised without importing
// the api module.
type overlapEnum int32

func TestSetScheduleOverlap(t *testing.T) {
	tests := []struct {
		policy string
		want   overlapEnum
	}{
		{OverlapSkip, overlapSkipEnum},
		{OverlapQueue, overlapBufferOneNum},
		{OverlapParallel, overlapAllowAllNum},
	}
	for _, tt := range tests {
		t.Run(tt.policy, func(t *testing.T) {
			var dst overlapEnum
			setScheduleOverlap(&dst, tt.policy)
			if dst != tt.want {
				t.Fatalf("setScheduleOverlap(%q) = %d, want %d", tt.policy, dst, tt.want)
			}
		})
	}
}

func TestSetScheduleOverlap_UnknownLeavesUnchanged(t *testing.T) {
	// mapOverlap rejects unknown policies before setScheduleOverlap runs, so an
	// unknown value here must be a no-op (dst stays at its prior value).
	dst := overlapEnum(99)
	setScheduleOverlap(&dst, "bogus")
	if dst != 99 {
		t.Fatalf("dst = %d, want unchanged 99", dst)
	}
}

func TestBuildScheduleSpec_Simple(t *testing.T) {
	spec := Spec{EveryMinutes: 15, Timezone: "Asia/Bangkok", OverlapPolicy: OverlapSkip}

	got := buildScheduleSpec(spec)
	if got.TimeZoneName != "Asia/Bangkok" {
		t.Errorf("TimeZoneName = %q, want Asia/Bangkok", got.TimeZoneName)
	}
	if len(got.CronExpressions) != 0 {
		t.Errorf("CronExpressions = %v, want none", got.CronExpressions)
	}
	if len(got.Intervals) != 1 {
		t.Fatalf("Intervals len = %d, want 1", len(got.Intervals))
	}
	if want := 15 * time.Minute; got.Intervals[0].Every != want {
		t.Errorf("Every = %v, want %v", got.Intervals[0].Every, want)
	}
}

func TestBuildScheduleSpec_Cron(t *testing.T) {
	spec := Spec{Cron: "0 8 * * MON", Timezone: "UTC", OverlapPolicy: OverlapQueue}

	got := buildScheduleSpec(spec)
	if got.TimeZoneName != "UTC" {
		t.Errorf("TimeZoneName = %q, want UTC", got.TimeZoneName)
	}
	if len(got.Intervals) != 0 {
		t.Errorf("Intervals = %v, want none", got.Intervals)
	}
	wantCron := []string{"0 8 * * MON"}
	if len(got.CronExpressions) != 1 || got.CronExpressions[0] != wantCron[0] {
		t.Fatalf("CronExpressions = %v, want %v", got.CronExpressions, wantCron)
	}
}

// TestBuildScheduleSpec_AssignableToOptions proves the built spec drops straight
// into the SDK's ScheduleOptions, catching any drift in the SDK's spec type.
func TestBuildScheduleSpec_AssignableToOptions(t *testing.T) {
	opts := client.ScheduleOptions{Spec: buildScheduleSpec(Spec{EveryMinutes: 1, Timezone: "UTC"})}
	if len(opts.Spec.Intervals) != 1 {
		t.Fatalf("Intervals len = %d, want 1", len(opts.Spec.Intervals))
	}
}

func TestIsAlreadyExists(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"already_exists", errors.New("schedule already exists"), true},
		{"already_running", errors.New("workflow already running"), true},
		{"mixed_case", errors.New("Schedule ALREADY EXISTS for id"), true},
		{"unrelated", errors.New("connection refused"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAlreadyExists(tt.err); got != tt.want {
				t.Fatalf("isAlreadyExists(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
