package scheduler

import (
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/client"
)

// Temporal ScheduleOverlapPolicy enum values (go.temporal.io/api/enums/v1).
// We use plain untyped integer constants rather than importing the enum package
// directly: the api module is only a transitive dependency of the SDK and has
// no require line in this module's go.mod, so naming it would break the build.
// These are assigned to the SDK's typed Overlap field, where Go converts the
// untyped constant implicitly. Values are pinned by the wire protocol and are
// covered by mapOverlap's unit tests.
const (
	overlapSkipEnum     = 1 // SCHEDULE_OVERLAP_POLICY_SKIP
	overlapBufferOneNum = 2 // SCHEDULE_OVERLAP_POLICY_BUFFER_ONE
	overlapAllowAllNum  = 6 // SCHEDULE_OVERLAP_POLICY_ALLOW_ALL
)

// scheduleID derives the Temporal schedule id for a flow. One schedule per
// flow, so the id is stable across create/update/pause/resume/delete.
func scheduleID(flowID string) string {
	return "flow-" + flowID
}

// mapOverlap translates a canonical overlap policy (OverlapSkip/Queue/Parallel)
// into the Temporal overlap enum value: skip -> SKIP, queue -> BUFFER_ONE,
// parallel -> ALLOW_ALL. An unrecognised value is a programming error (Spec is
// validated by SpecFromDefinition) and is surfaced rather than silently
// defaulted. The result is an int so this helper stays free of the api module
// and is directly unit-testable.
func mapOverlap(policy string) (int, error) {
	switch policy {
	case OverlapSkip:
		return overlapSkipEnum, nil
	case OverlapQueue:
		return overlapBufferOneNum, nil
	case OverlapParallel:
		return overlapAllowAllNum, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrOverlapPolicy, policy)
	}
}

// setScheduleOverlap writes the Temporal overlap enum value for policy into dst.
// dst is generic over any int32-based type so we can set the SDK's
// ScheduleOverlapPolicy field via untyped constants without importing the api
// enum module (which is not a direct require in go.mod). An unknown policy —
// already rejected by mapOverlap before this is called — leaves dst unchanged.
func setScheduleOverlap[T ~int32](dst *T, policy string) {
	switch policy {
	case OverlapSkip:
		*dst = overlapSkipEnum
	case OverlapQueue:
		*dst = overlapBufferOneNum
	case OverlapParallel:
		*dst = overlapAllowAllNum
	}
}

// buildScheduleSpec maps a Spec into a Temporal ScheduleSpec. Cron mode emits a
// single CronExpression; simple mode emits a single Interval of EveryMinutes.
// The timezone is carried on TimeZoneName so calendar/cron matching is
// evaluated in the flow's zone.
func buildScheduleSpec(spec Spec) client.ScheduleSpec {
	out := client.ScheduleSpec{TimeZoneName: spec.Timezone}
	if spec.Cron != "" {
		out.CronExpressions = []string{spec.Cron}
	} else {
		out.Intervals = []client.ScheduleIntervalSpec{
			{Every: time.Duration(spec.EveryMinutes) * time.Minute},
		}
	}
	return out
}

// isAlreadyExists reports whether err indicates the schedule already exists
// (Create on an existing id). The SDK surfaces a gRPC AlreadyExists
// serviceerror, but that type lives in the api module we deliberately do not
// import; a message match is sufficient here since this branch only drives the
// create-vs-update fork in the integration-tested adapter.
func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "already running")
}
