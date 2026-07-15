package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/mywork/automate/apps/automate-api/internal/audit"
	"github.com/mywork/automate/apps/automate-api/internal/store"
)

// --- E3-S6: soft delete / restore ---

func TestDeleteFlow_SoftDeletes_204(t *testing.T) {
	fake := seedFake()
	aud := &fakeAudit{}
	r := routerWithDeps(fake, &fakeRunner{}, aud, nil)

	w, _ := doReq(t, r, http.MethodDelete, APIBasePath+"/flows/flw_payroll", nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if len(fake.softDeletes) != 1 || fake.softDeletes[0] != "flw_payroll" {
		t.Fatalf("SoftDeleteFlow calls = %v, want [flw_payroll]", fake.softDeletes)
	}
	if got := aud.actions(); len(got) != 1 || got[0] != audit.ActionFlowDelete {
		t.Fatalf("audit actions = %v, want [flow.delete]", got)
	}
	if aud.records[0].ResourceID != "flw_payroll" {
		t.Errorf("audit resourceId = %q, want flw_payroll", aud.records[0].ResourceID)
	}
}

func TestDeleteFlow_LeavesStatusUnchanged(t *testing.T) {
	fake := seedFake()
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, nil)
	doReq(t, r, http.MethodDelete, APIBasePath+"/flows/flw_payroll", nil)
	// The flow row's status is untouched by a soft delete.
	for _, fl := range fake.flows {
		if fl.ID == "flw_payroll" && fl.Status != "published" {
			t.Errorf("status = %q, want published (soft delete must not change status)", fl.Status)
		}
	}
}

func TestDeleteFlow_NotFound_404(t *testing.T) {
	fake := seedFake()
	aud := &fakeAudit{}
	r := routerWithDeps(fake, &fakeRunner{}, aud, nil)
	w, body := doReq(t, r, http.MethodDelete, APIBasePath+"/flows/nope", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	assertErrorEnvelope(t, body, "not_found")
	if len(aud.records) != 0 {
		t.Errorf("a failed delete must not audit, got %v", aud.records)
	}
}

func TestDeleteFlow_StoreError_500(t *testing.T) {
	fake := seedFake()
	fake.errSoftDelete = errors.New("db down")
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, nil)
	w, body := doReq(t, r, http.MethodDelete, APIBasePath+"/flows/flw_payroll", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	assertErrorEnvelope(t, body, "internal")
}

func TestRestoreFlow_ClearsDeleted_200(t *testing.T) {
	fake := seedFake()
	fake.deleted = map[string]string{"flw_payroll": "2026-07-15T00:00:00.000Z"}
	aud := &fakeAudit{}
	r := routerWithDeps(fake, &fakeRunner{}, aud, nil)

	w, body := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/restore", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body["status"] != "restored" {
		t.Errorf("status body = %v, want restored", body["status"])
	}
	if len(fake.restores) != 1 || fake.restores[0] != "flw_payroll" {
		t.Fatalf("RestoreFlow calls = %v, want [flw_payroll]", fake.restores)
	}
	if got := aud.actions(); len(got) != 1 || got[0] != audit.ActionFlowRestore {
		t.Fatalf("audit actions = %v, want [flow.restore]", got)
	}
}

func TestRestoreFlow_NotFound_404(t *testing.T) {
	fake := seedFake()
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, nil)
	w, body := doReq(t, r, http.MethodPost, APIBasePath+"/flows/nope/restore", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	assertErrorEnvelope(t, body, "not_found")
}

func TestRestoreFlow_StoreError_500(t *testing.T) {
	fake := seedFake()
	fake.errRestore = errors.New("db down")
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, nil)
	w, _ := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/restore", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

// RequireAdmin gates restore: a non-admin with FlowCreate/Publish is still 403.
func TestRestoreFlow_AdminOnly(t *testing.T) {
	cases := []struct {
		role       string
		wantStatus int
	}{
		{"admin", http.StatusOK},
		{"designer", http.StatusForbidden},
		{"operator", http.StatusForbidden},
		{"viewer", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			fake := seedFake()
			r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, nil)
			w, _ := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/restore",
				map[string]string{roleHeader: tc.role})
			if w.Code != tc.wantStatus {
				t.Fatalf("restore as %s = %d, want %d", tc.role, w.Code, tc.wantStatus)
			}
		})
	}
}

// DELETE requires FlowCreate: admin+designer allowed, operator+viewer 403.
func TestDeleteFlow_RBAC(t *testing.T) {
	cases := map[string]bool{"admin": true, "designer": true, "operator": false, "viewer": false}
	for role, allowed := range cases {
		t.Run(role, func(t *testing.T) {
			fake := seedFake()
			r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, nil)
			w, _ := doReq(t, r, http.MethodDelete, APIBasePath+"/flows/flw_payroll",
				map[string]string{roleHeader: role})
			if allowed && w.Code == http.StatusForbidden {
				t.Fatalf("delete as %s = 403, want allowed", role)
			}
			if !allowed && w.Code != http.StatusForbidden {
				t.Fatalf("delete as %s = %d, want 403", role, w.Code)
			}
		})
	}
}

// --- E3-S6: list excludes/includes soft-deleted ---

func TestListFlows_ExcludesSoftDeletedByDefault(t *testing.T) {
	fake := seedFake()
	fake.deleted = map[string]string{"flw_payroll": "2026-07-15T00:00:00.000Z"}
	r := newTestRouter(fake)
	_, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows", nil)
	flows := body["flows"].([]any)
	if len(flows) != 4 {
		t.Fatalf("default list len = %d, want 4 (deleted excluded)", len(flows))
	}
	for _, f := range flows {
		if f.(map[string]any)["id"] == "flw_payroll" {
			t.Errorf("soft-deleted flow must not appear in default list")
		}
	}
}

func TestListFlows_IncludeDeleted_AdminSeesDeletedWithMarker(t *testing.T) {
	fake := seedFake()
	fake.deleted = map[string]string{"flw_payroll": "2026-07-15T00:00:00.000Z"}
	r := newTestRouter(fake) // no X-Role → defaults to admin
	_, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows?includeDeleted=true", nil)
	flows := body["flows"].([]any)
	if len(flows) != 5 {
		t.Fatalf("includeDeleted list len = %d, want 5", len(flows))
	}
	var found bool
	for _, f := range flows {
		m := f.(map[string]any)
		if m["id"] == "flw_payroll" {
			found = true
			if m["deletedAt"] != "2026-07-15T00:00:00.000Z" {
				t.Errorf("deletedAt = %v, want the marker", m["deletedAt"])
			}
		}
	}
	if !found {
		t.Fatal("includeDeleted should include the soft-deleted flow")
	}
}

// A live flow omits deletedAt entirely (omitempty), preserving the wire contract.
func TestListFlows_LiveFlowOmitsDeletedAt(t *testing.T) {
	r := newTestRouter(seedFake())
	_, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows", nil)
	first := body["flows"].([]any)[0].(map[string]any)
	if _, present := first["deletedAt"]; present {
		t.Errorf("live flow should omit deletedAt, got %v", first["deletedAt"])
	}
}

// A non-admin passing includeDeleted=true is silently served the live-only list
// (the param is admin-gated in the handler, not a 403).
func TestListFlows_IncludeDeleted_NonAdminIgnored(t *testing.T) {
	fake := seedFake()
	fake.deleted = map[string]string{"flw_payroll": "2026-07-15T00:00:00.000Z"}
	r := newTestRouter(fake)
	_, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows?includeDeleted=true",
		map[string]string{roleHeader: "designer"})
	flows := body["flows"].([]any)
	if len(flows) != 4 {
		t.Fatalf("non-admin includeDeleted len = %d, want 4 (param ignored)", len(flows))
	}
}

// --- E2-S3: flow grants CRUD ---

func TestGrants_CRUD(t *testing.T) {
	fake := seedFake()
	aud := &fakeAudit{}
	r := routerWithDeps(fake, &fakeRunner{}, aud, nil)

	// Initially empty.
	_, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows/flw_payroll/grants", nil)
	if grants := body["grants"].([]any); len(grants) != 0 {
		t.Fatalf("initial grants = %d, want 0", len(grants))
	}

	// Add a grant → 201.
	wc, cb := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/grants",
		`{"subjectType":"role","subjectId":"designer","access":"editor"}`, nil)
	if wc.Code != http.StatusCreated {
		t.Fatalf("add grant = %d, want 201", wc.Code)
	}
	if cb["subjectType"] != "role" || cb["subjectId"] != "designer" || cb["access"] != "editor" {
		t.Errorf("grant body = %v, want the created grant", cb)
	}
	grantID := int64(cb["id"].(float64))

	// List now returns it.
	_, lb := doReq(t, r, http.MethodGet, APIBasePath+"/flows/flw_payroll/grants", nil)
	grants := lb["grants"].([]any)
	if len(grants) != 1 {
		t.Fatalf("grants after add = %d, want 1", len(grants))
	}
	g := grants[0].(map[string]any)
	for _, key := range []string{"id", "flowId", "subjectType", "subjectId", "access"} {
		if _, ok := g[key]; !ok {
			t.Errorf("grant missing key %q: %v", key, g)
		}
	}

	// Delete → 204.
	wd, _ := doReq(t, r, http.MethodDelete,
		APIBasePath+"/flows/flw_payroll/grants/"+i64(grantID), nil)
	if wd.Code != http.StatusNoContent {
		t.Fatalf("delete grant = %d, want 204", wd.Code)
	}

	// List empty again.
	_, fb := doReq(t, r, http.MethodGet, APIBasePath+"/flows/flw_payroll/grants", nil)
	if len(fb["grants"].([]any)) != 0 {
		t.Fatalf("grants after delete = %d, want 0", len(fb["grants"].([]any)))
	}

	// Audit: add + delete both recorded rbac.grant_change.
	want := []string{audit.ActionRBACGrantChange, audit.ActionRBACGrantChange}
	if got := aud.actions(); !equalStrs(got, want) {
		t.Fatalf("audit actions = %v, want %v", got, want)
	}
}

func TestAddGrant_Validation_400(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"bad json", `{bad`},
		{"unknown subjectType", `{"subjectType":"group","subjectId":"x","access":"viewer"}`},
		{"empty subjectId", `{"subjectType":"user","subjectId":"","access":"viewer"}`},
		{"unknown access", `{"subjectType":"user","subjectId":"u1","access":"superuser"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			aud := &fakeAudit{}
			r := routerWithDeps(seedFake(), &fakeRunner{}, aud, nil)
			w, body := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/grants", tc.body, nil)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", w.Code)
			}
			assertErrorEnvelope(t, body, "bad_request")
			if len(aud.records) != 0 {
				t.Errorf("invalid grant must not audit, got %v", aud.records)
			}
		})
	}
}

func TestAddGrant_StoreError_500(t *testing.T) {
	fake := seedFake()
	fake.errAddGrant = errors.New("db down")
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, nil)
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/grants",
		`{"subjectType":"role","subjectId":"designer","access":"owner"}`, nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestListGrants_StoreError_500(t *testing.T) {
	fake := seedFake()
	fake.errListGrants = errors.New("db down")
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, nil)
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows/flw_payroll/grants", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	assertErrorEnvelope(t, body, "internal")
}

func TestDeleteGrant_BadID_400(t *testing.T) {
	r := routerWithDeps(seedFake(), &fakeRunner{}, &fakeAudit{}, nil)
	w, body := doReq(t, r, http.MethodDelete, APIBasePath+"/flows/flw_payroll/grants/notanumber", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	assertErrorEnvelope(t, body, "bad_request")
}

func TestDeleteGrant_NotFound_404(t *testing.T) {
	fake := seedFake()
	aud := &fakeAudit{}
	r := routerWithDeps(fake, &fakeRunner{}, aud, nil)
	w, _ := doReq(t, r, http.MethodDelete, APIBasePath+"/flows/flw_payroll/grants/999", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if len(aud.records) != 0 {
		t.Errorf("a failed delete must not audit, got %v", aud.records)
	}
}

func TestDeleteGrant_StoreError_500(t *testing.T) {
	fake := seedFake()
	fake.grants = []store.Grant{{ID: 5, FlowID: "flw_payroll", SubjectType: "user", SubjectID: "u1", Access: "viewer"}}
	fake.errDeleteGrant = errors.New("db down")
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, nil)
	w, _ := doReq(t, r, http.MethodDelete, APIBasePath+"/flows/flw_payroll/grants/5", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

// Grants routes require FlowPublish: admin+designer allowed, operator+viewer 403.
func TestGrants_RBAC_403(t *testing.T) {
	routes := []struct {
		method, path, body string
	}{
		{http.MethodGet, APIBasePath + "/flows/flw_payroll/grants", ""},
		{http.MethodPost, APIBasePath + "/flows/flw_payroll/grants", `{"subjectType":"role","subjectId":"designer","access":"viewer"}`},
		{http.MethodDelete, APIBasePath + "/flows/flw_payroll/grants/1", ""},
	}
	allowed := map[string]bool{"admin": true, "designer": true, "operator": false, "viewer": false}
	for _, rt := range routes {
		for role, ok := range allowed {
			t.Run(rt.method+"/"+role, func(t *testing.T) {
				fake := seedFake()
				fake.grants = []store.Grant{{ID: 1, FlowID: "flw_payroll", SubjectType: "user", SubjectID: "u1", Access: "viewer"}}
				r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, nil)
				w, _ := doReqBody(t, r, rt.method, rt.path, rt.body, map[string]string{roleHeader: role})
				if ok && w.Code == http.StatusForbidden {
					t.Fatalf("%s as %s = 403, want allowed", rt.method, role)
				}
				if !ok && w.Code != http.StatusForbidden {
					t.Fatalf("%s as %s = %d, want 403", rt.method, role, w.Code)
				}
			})
		}
	}
}

// i64 renders an int64 as a base-10 string for path construction.
func i64(n int64) string {
	return strconv.FormatInt(n, 10)
}
