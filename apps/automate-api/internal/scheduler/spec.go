// Package scheduler compiles a flow's trigger.schedule node into a schedule
// spec and syncs it to a scheduling backend (Temporal) on lifecycle events.
//
// The package is split into pure logic — SpecFromDefinition and the mapping
// helpers, which are 100%/≥95% unit-tested — and a thin Temporal adapter
// (temporalScheduler) that the API composition root wires in. Handlers depend
// only on the Scheduler interface so they never import Temporal directly.
package scheduler

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mywork/automate/internal/flowspec"
)

// scheduleNodeType is the flow node type that carries a recurring schedule
// (docs/spec/05 §1.1). A flow has at most one in MVP.
const scheduleNodeType = "trigger.schedule"

// Default values applied when the node config omits them (docs/spec/05 §1.1).
const (
	// DefaultTimezone is the IANA zone used when config.timezone is empty.
	DefaultTimezone = "Asia/Bangkok"
	// DefaultOverlapPolicy is used when config.overlapPolicy is empty.
	DefaultOverlapPolicy = OverlapSkip
)

// Overlap policy values as they appear in node config and in Spec.OverlapPolicy
// (docs/spec/05 §1.1, FR-TRIG-006). These are the canonical, backend-agnostic
// strings; the Temporal adapter maps them to enum values.
const (
	OverlapSkip     = "skip"
	OverlapQueue    = "queue"
	OverlapParallel = "parallel"
)

// Schedule modes (docs/spec/05 §1.1).
const (
	modeSimple = "simple"
	modeCron   = "cron"
)

// Spec is the backend-agnostic description of when a flow should run and how
// concurrent runs are handled. Exactly one of Cron / EveryMinutes is set:
// cron mode populates Cron, simple mode populates EveryMinutes.
type Spec struct {
	// Cron is a cron expression (cron mode); empty in simple mode.
	Cron string
	// EveryMinutes is the interval in minutes (simple mode); 0 in cron mode.
	EveryMinutes int
	// Timezone is the IANA zone name (defaulted to DefaultTimezone).
	Timezone string
	// OverlapPolicy is one of OverlapSkip / OverlapQueue / OverlapParallel.
	OverlapPolicy string
}

// scheduleConfig is the JSON shape of a trigger.schedule node's config
// (docs/spec/05 §1.1). Fields are optional so we can validate them explicitly
// and produce actionable errors rather than relying on zero values.
type scheduleConfig struct {
	Mode          string `json:"mode"`
	EveryMinutes  *int   `json:"everyMinutes"`
	Cron          string `json:"cron"`
	Timezone      string `json:"timezone"`
	OverlapPolicy string `json:"overlapPolicy"`
}

// Errors returned by SpecFromDefinition. Callers can match with errors.Is to
// distinguish validation failures from a well-formed config.
var (
	ErrInvalidConfig    = errors.New("scheduler: invalid trigger.schedule config")
	ErrUnknownMode      = errors.New("scheduler: unknown schedule mode")
	ErrEveryMinutes     = errors.New("scheduler: simple mode requires everyMinutes >= 1")
	ErrEmptyCron        = errors.New("scheduler: cron mode requires a non-empty cron expression")
	ErrOverlapPolicy    = errors.New("scheduler: unknown overlapPolicy")
	ErrMultipleSchedule = errors.New("scheduler: flow has more than one trigger.schedule node")
)

// SpecFromDefinition finds the single trigger.schedule node in def, parses its
// config, applies defaults (timezone Asia/Bangkok, overlapPolicy skip) and
// validates it. It returns (spec, true, nil) when a valid schedule node is
// present, (zero, false, nil) when the flow has no schedule node (a manual or
// otherwise-triggered flow), and (zero, false, err) on a malformed config.
func SpecFromDefinition(def flowspec.FlowDef) (Spec, bool, error) {
	node, found, err := findScheduleNode(def)
	if err != nil {
		return Spec{}, false, err
	}
	if !found {
		return Spec{}, false, nil
	}

	cfg, err := parseConfig(node.Config)
	if err != nil {
		return Spec{}, false, err
	}

	spec, err := specFromConfig(cfg)
	if err != nil {
		return Spec{}, false, err
	}
	return spec, true, nil
}

// findScheduleNode returns the lone trigger.schedule node, if any. More than
// one is a definition error (MVP allows a single trigger per flow).
func findScheduleNode(def flowspec.FlowDef) (flowspec.NodeDef, bool, error) {
	var found flowspec.NodeDef
	var count int
	for _, n := range def.Nodes {
		if n.Type == scheduleNodeType {
			found = n
			count++
		}
	}
	if count > 1 {
		return flowspec.NodeDef{}, false, ErrMultipleSchedule
	}
	return found, count == 1, nil
}

// parseConfig decodes the node's raw config into scheduleConfig. A nil/empty
// config decodes to the zero value, which specFromConfig then rejects with a
// mode error (rather than silently succeeding).
func parseConfig(raw json.RawMessage) (scheduleConfig, error) {
	var cfg scheduleConfig
	if len(raw) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return scheduleConfig{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	return cfg, nil
}

// specFromConfig applies defaults and validates a decoded config into a Spec.
func specFromConfig(cfg scheduleConfig) (Spec, error) {
	overlap, err := normalizeOverlap(cfg.OverlapPolicy)
	if err != nil {
		return Spec{}, err
	}

	spec := Spec{
		Timezone:      normalizeTimezone(cfg.Timezone),
		OverlapPolicy: overlap,
	}

	switch cfg.Mode {
	case modeSimple:
		if cfg.EveryMinutes == nil || *cfg.EveryMinutes < 1 {
			return Spec{}, ErrEveryMinutes
		}
		spec.EveryMinutes = *cfg.EveryMinutes
	case modeCron:
		if cfg.Cron == "" {
			return Spec{}, ErrEmptyCron
		}
		spec.Cron = cfg.Cron
	default:
		return Spec{}, fmt.Errorf("%w: %q", ErrUnknownMode, cfg.Mode)
	}

	return spec, nil
}

// normalizeTimezone returns the configured zone or the Asia/Bangkok default.
func normalizeTimezone(tz string) string {
	if tz == "" {
		return DefaultTimezone
	}
	return tz
}

// normalizeOverlap defaults an empty policy to skip and rejects unknown values.
func normalizeOverlap(policy string) (string, error) {
	switch policy {
	case "":
		return DefaultOverlapPolicy, nil
	case OverlapSkip, OverlapQueue, OverlapParallel:
		return policy, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrOverlapPolicy, policy)
	}
}
