// Package lifecycle is the pure flow-status state machine (docs/spec/08 E4-S1).
// It has no dependencies and no side effects: given a flow's current status and a
// requested next status, CanTransition reports whether the hop is legal. The
// httpapi handlers consult it before touching the store/scheduler and answer 409
// invalid_transition when it returns false.
package lifecycle

// The four flow statuses. A flow is authored as draft, goes live on publish, may
// be temporarily paused (schedule suspended) and is ultimately stopped (schedule
// deleted). A stopped flow is terminal for its schedule but may be re-published.
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusPaused    = "paused"
	StatusStopped   = "stopped"
)

// transitions is the allowed adjacency: transitions[from][to] == true iff the
// hop is legal. Absent keys (unknown statuses, self-transitions) are illegal.
var transitions = map[string]map[string]bool{
	StatusDraft: {
		StatusPublished: true,
	},
	StatusPublished: {
		StatusPaused:  true,
		StatusStopped: true,
	},
	StatusPaused: {
		StatusPublished: true, // resume
		StatusStopped:   true,
	},
	StatusStopped: {
		StatusPublished: true, // re-publish a stopped flow
	},
}

// CanTransition reports whether a flow may move from status `from` to status
// `to`. It is total: any pair involving an unknown status, or a self-transition,
// returns false.
func CanTransition(from, to string) bool {
	return transitions[from][to]
}
