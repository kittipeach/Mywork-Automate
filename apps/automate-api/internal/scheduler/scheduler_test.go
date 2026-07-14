package scheduler

import (
	"context"
	"testing"

	"github.com/mywork/automate/internal/flowspec"
)

// TestNoop verifies the Noop Scheduler satisfies the interface and every method
// is a successful no-op, so the API can run without a Temporal client.
func TestNoop(t *testing.T) {
	var s Scheduler = Noop{}
	ctx := context.Background()

	if err := s.Sync(ctx, "flow-1", Spec{EveryMinutes: 5, Timezone: "UTC", OverlapPolicy: OverlapSkip}, flowspec.FlowInput{}); err != nil {
		t.Errorf("Sync = %v, want nil", err)
	}
	if err := s.Pause(ctx, "flow-1"); err != nil {
		t.Errorf("Pause = %v, want nil", err)
	}
	if err := s.Resume(ctx, "flow-1"); err != nil {
		t.Errorf("Resume = %v, want nil", err)
	}
	if err := s.Delete(ctx, "flow-1"); err != nil {
		t.Errorf("Delete = %v, want nil", err)
	}
}
