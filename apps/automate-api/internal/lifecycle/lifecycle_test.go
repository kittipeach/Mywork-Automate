package lifecycle

import "testing"

// TestCanTransition_FullMatrix exercises every (from × to) pair over the known
// statuses plus an unknown status, so the whole transition table is covered.
//
// Allowed transitions (docs/spec/08 E4-S1):
//   - draft     → published
//   - published → paused | stopped
//   - paused    → published (resume) | stopped
//   - stopped   → published (re-publish a stopped flow)
//
// Everything else — including self-transitions and any hop involving an unknown
// status — is rejected (the handler answers 409 invalid_transition).
func TestCanTransition_FullMatrix(t *testing.T) {
	states := []string{StatusDraft, StatusPublished, StatusPaused, StatusStopped, "bogus"}

	allowed := map[string]map[string]bool{
		StatusDraft:     {StatusPublished: true},
		StatusPublished: {StatusPaused: true, StatusStopped: true},
		StatusPaused:    {StatusPublished: true, StatusStopped: true},
		StatusStopped:   {StatusPublished: true},
	}

	for _, from := range states {
		for _, to := range states {
			want := allowed[from][to]
			if got := CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%q, %q) = %v, want %v", from, to, got, want)
			}
		}
	}
}
