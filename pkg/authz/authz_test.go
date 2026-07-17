package authz

import (
	"testing"
)

// TestCan_Matrix asserts EVERY role×permission cell of the docs/spec/07 §2.1
// RBAC matrix. This is the authoritative encoding of the matrix; if a cell
// changes in the spec, this table must change with it.
func TestCan_Matrix(t *testing.T) {
	// want[role][permission] = expected Can() result. Every combination of the
	// four roles and nine permissions is present exactly once.
	want := map[Role]map[Permission]bool{
		Admin: {
			FlowView: true, RunView: true, FlowCreate: true, FlowPublish: true,
			FlowRun: true, ConnectionManage: true, TemplateManage: true,
			AIUse: true, MaskingBypass: true,
		},
		Designer: {
			FlowView: true, RunView: true, FlowCreate: true, FlowPublish: true,
			FlowRun: true, ConnectionManage: false, TemplateManage: true,
			AIUse: true, MaskingBypass: true,
		},
		Operator: {
			FlowView: true, RunView: true, FlowCreate: false, FlowPublish: false,
			FlowRun: true, ConnectionManage: false, TemplateManage: false,
			AIUse: true, MaskingBypass: true,
		},
		Viewer: {
			FlowView: true, RunView: true, FlowCreate: false, FlowPublish: false,
			FlowRun: false, ConnectionManage: false, TemplateManage: false,
			AIUse: false, MaskingBypass: false,
		},
	}

	roles := []Role{Admin, Designer, Operator, Viewer}
	perms := []Permission{
		FlowView, RunView, FlowCreate, FlowPublish, FlowRun,
		ConnectionManage, TemplateManage, AIUse, MaskingBypass,
	}

	// Guard: ensure the test table covers every cell (4×9 = 36).
	cells := 0
	for _, r := range roles {
		for range perms {
			cells++
		}
		if len(want[r]) != len(perms) {
			t.Fatalf("test table for role %q covers %d perms, want %d", r, len(want[r]), len(perms))
		}
	}
	if cells != len(roles)*len(perms) {
		t.Fatalf("cell count = %d, want %d", cells, len(roles)*len(perms))
	}

	for _, r := range roles {
		for _, p := range perms {
			got := Can(r, p)
			if got != want[r][p] {
				t.Errorf("Can(%q, %q) = %v, want %v", r, p, got, want[r][p])
			}
		}
	}
}

func TestCan_UnknownRole_DeniedForEveryPermission(t *testing.T) {
	perms := []Permission{
		FlowView, RunView, FlowCreate, FlowPublish, FlowRun,
		ConnectionManage, TemplateManage, AIUse, MaskingBypass,
	}
	for _, p := range perms {
		if Can(Role("auditor"), p) {
			t.Errorf("Can(auditor, %q) = true, want false (unknown role denied)", p)
		}
	}
	if Can(Role(""), FlowView) {
		t.Error("Can(empty role, FlowView) = true, want false")
	}
}

func TestCan_UnknownPermission_Denied(t *testing.T) {
	for _, r := range []Role{Admin, Designer, Operator, Viewer} {
		if Can(r, Permission("flow.nuke")) {
			t.Errorf("Can(%q, flow.nuke) = true, want false (unknown permission denied)", r)
		}
	}
}

func TestRoles(t *testing.T) {
	got := Roles()
	want := []Role{Admin, Designer, Operator, Viewer}
	if len(got) != len(want) {
		t.Fatalf("Roles() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Roles()[%d] = %q, want %q (must be highest→lowest privilege)", i, got[i], want[i])
		}
	}
	// The returned slice must be a copy: mutating it must not affect later calls.
	got[0] = Viewer
	if Roles()[0] != Admin {
		t.Error("Roles() returned a shared slice; mutation leaked")
	}
}

func TestPermissions(t *testing.T) {
	tests := []struct {
		role Role
		want []Permission
	}{
		{Admin, []Permission{FlowView, RunView, FlowCreate, FlowPublish, FlowRun, ConnectionManage, TemplateManage, AIUse, MaskingBypass}},
		{Designer, []Permission{FlowView, RunView, FlowCreate, FlowPublish, FlowRun, TemplateManage, AIUse, MaskingBypass}},
		{Operator, []Permission{FlowView, RunView, FlowRun, AIUse, MaskingBypass}},
		{Viewer, []Permission{FlowView, RunView}},
		{Role("auditor"), []Permission{}},
	}
	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			got := Permissions(tt.role)
			if got == nil {
				t.Fatal("Permissions() returned nil, want non-nil slice")
			}
			if len(got) != len(tt.want) {
				t.Fatalf("Permissions(%q) = %v, want %v", tt.role, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("Permissions(%q)[%d] = %q, want %q", tt.role, i, got[i], tt.want[i])
				}
			}
			// Every returned permission must actually be granted by Can().
			for _, p := range got {
				if !Can(tt.role, p) {
					t.Errorf("Permissions(%q) returned %q but Can() denies it", tt.role, p)
				}
			}
		})
	}
}

func TestIsValidRole(t *testing.T) {
	tests := []struct {
		role Role
		want bool
	}{
		{Admin, true}, {Designer, true}, {Operator, true}, {Viewer, true},
		{Role("auditor"), false}, {Role(""), false}, {Role("Admin"), false},
	}
	for _, tt := range tests {
		if got := IsValidRole(tt.role); got != tt.want {
			t.Errorf("IsValidRole(%q) = %v, want %v", tt.role, got, tt.want)
		}
	}
}

func TestHighest(t *testing.T) {
	tests := []struct {
		name       string
		candidates []Role
		want       Role
		wantOK     bool
	}{
		{"single admin", []Role{Admin}, Admin, true},
		{"designer+operator picks designer", []Role{Operator, Designer}, Designer, true},
		{"admin among many wins", []Role{Viewer, Admin, Operator}, Admin, true},
		{"viewer+operator picks operator", []Role{Viewer, Operator}, Operator, true},
		{"order independent", []Role{Designer, Admin, Viewer}, Admin, true},
		{"only viewer", []Role{Viewer}, Viewer, true},
		{"invalid roles ignored, valid kept", []Role{Role("auditor"), Operator}, Operator, true},
		{"all invalid", []Role{Role("auditor"), Role("x")}, "", false},
		{"empty", nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Highest(tt.candidates)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("Highest(%v) = (%q, %v), want (%q, %v)", tt.candidates, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
