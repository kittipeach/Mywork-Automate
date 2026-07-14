// Package authz is the pure RBAC permission model for MyWork Automate
// (docs/spec/07 §2.1). It defines the four baseline system roles and the
// permissions each grants, exposing a single decision point Can(role, perm).
//
// The model is deny-by-default: an unknown role, an unknown permission, or any
// role×permission cell not explicitly granted resolves to false. This package
// has no I/O and no dependencies so it can be tested to 100% and reused by the
// API middleware and (later) the worker.
package authz

// Role is a baseline system role. Custom roles arrive in Phase 2; for MVP the
// four below map 1:1 to the columns of the docs/spec/07 §2.1 matrix.
type Role string

// The four baseline system roles (docs/spec/07 §2.1). Admin is the most
// privileged; Viewer the least.
const (
	Admin    Role = "admin"
	Designer Role = "designer"
	Operator Role = "operator"
	Viewer   Role = "viewer"
)

// Permission is a discrete capability guarded by RBAC. Each corresponds to a
// row of the docs/spec/07 §2.1 matrix.
type Permission string

// Permissions mapped from the docs/spec/07 §2.1 matrix rows.
const (
	// FlowView — see flows in the catalog (list/get). Every role has it; the
	// object-level flow_grants filter (which flows a viewer actually sees) is a
	// separate concern layered on top.
	FlowView Permission = "flow.view"
	// RunView — see run history and step logs (matrix: "ดู run history + log").
	RunView Permission = "run.view"
	// FlowCreate — create/edit a flow draft (matrix: "สร้าง/แก้ flow (draft)").
	FlowCreate Permission = "flow.create"
	// FlowPublish — publish or roll back a flow (matrix: "Publish / Rollback").
	FlowPublish Permission = "flow.publish"
	// FlowRun — manual run plus pause/resume/stop/cancel of a run
	// (matrix rows: "Pause/Resume/Stop/Cancel run" and "Manual run").
	FlowRun Permission = "flow.run"
	// ConnectionManage — manage users/roles/masking/connections. This is the
	// admin-only management row of the matrix.
	ConnectionManage Permission = "connection.manage"
	// TemplateManage — create/edit templates (matrix: "สร้าง/แก้ template").
	TemplateManage Permission = "template.manage"
	// AIUse — use AI Assist / AI nodes (matrix: "ใช้ AI Assist").
	AIUse Permission = "ai.use"
	// MaskingBypass — eligibility to view unmasked sensitive data. Viewer is
	// denied always (matrix: "❌ เสมอ"); the other roles are eligible subject to
	// the masking exempt rules enforced separately in the masking engine (E2-S4).
	MaskingBypass Permission = "masking.bypass"
)

// rolePermissions is the RBAC matrix: role → set of granted permissions
// (docs/spec/07 §2.1). Anything absent is denied. Keeping it a table (rather
// than branching logic) makes the matrix auditable at a glance and lets tests
// assert every cell.
var rolePermissions = map[Role]map[Permission]bool{
	Admin: {
		FlowView:         true,
		RunView:          true,
		FlowCreate:       true,
		FlowPublish:      true,
		FlowRun:          true,
		ConnectionManage: true,
		TemplateManage:   true,
		AIUse:            true,
		MaskingBypass:    true,
	},
	Designer: {
		FlowView:       true,
		RunView:        true,
		FlowCreate:     true,
		FlowPublish:    true,
		FlowRun:        true,
		TemplateManage: true,
		AIUse:          true,
		MaskingBypass:  true,
	},
	Operator: {
		FlowView:      true,
		RunView:       true,
		FlowRun:       true,
		AIUse:         true,
		MaskingBypass: true,
	},
	Viewer: {
		FlowView: true,
		RunView:  true,
	},
}

// allRoles lists the roles in descending privilege order. Used by Roles() and
// by callers that must pick the highest-privilege role from a claim set.
var allRoles = []Role{Admin, Designer, Operator, Viewer}

// allPermissions is the canonical ordered permission list, used to produce a
// stable order from Permissions().
var allPermissions = []Permission{
	FlowView,
	RunView,
	FlowCreate,
	FlowPublish,
	FlowRun,
	ConnectionManage,
	TemplateManage,
	AIUse,
	MaskingBypass,
}

// Can reports whether role is granted permission p. Deny-by-default: an unknown
// role or permission, or any ungranted cell, returns false.
func Can(role Role, p Permission) bool {
	perms, ok := rolePermissions[role]
	if !ok {
		return false
	}
	return perms[p]
}

// Roles returns the baseline system roles in descending privilege order. The
// returned slice is a copy; callers may mutate it freely.
func Roles() []Role {
	out := make([]Role, len(allRoles))
	copy(out, allRoles)
	return out
}

// Permissions returns the permissions granted to role, in canonical order. An
// unknown role yields an empty (non-nil) slice.
func Permissions(role Role) []Permission {
	out := make([]Permission, 0, len(allPermissions))
	perms, ok := rolePermissions[role]
	if !ok {
		return out
	}
	for _, p := range allPermissions {
		if perms[p] {
			out = append(out, p)
		}
	}
	return out
}

// IsValidRole reports whether role is one of the baseline system roles.
func IsValidRole(role Role) bool {
	_, ok := rolePermissions[role]
	return ok
}

// Highest returns the most privileged valid role among the candidates (privilege
// order: admin > designer > operator > viewer). Invalid/unknown roles are
// ignored. The second return is false when none of the candidates is valid.
func Highest(candidates []Role) (Role, bool) {
	for _, r := range allRoles { // allRoles is already highest→lowest
		for _, c := range candidates {
			if c == r {
				return r, true
			}
		}
	}
	return "", false
}
