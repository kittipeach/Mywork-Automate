package httpapi

import (
	"errors"
	"net/http"
	"testing"

	"github.com/mywork/automate/apps/automate-api/internal/audit"
	"github.com/mywork/automate/apps/automate-api/internal/store"
)

// --- pause ---

func TestPause_HappyPath(t *testing.T) {
	fake := seedFake() // flw_payroll is "published"
	sch := &fakeScheduler{}
	aud := &fakeAudit{}
	r := routerWithDeps(fake, &fakeRunner{}, aud, sch)

	w, body := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/pause", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body["status"] != "paused" {
		t.Errorf("status = %v, want paused", body["status"])
	}
	if len(fake.statusSets) != 1 || fake.statusSets[0].status != "paused" {
		t.Errorf("statusSets = %+v, want one paused", fake.statusSets)
	}
	if len(sch.pauses) != 1 || sch.pauses[0] != "flw_payroll" {
		t.Errorf("scheduler pauses = %v, want [flw_payroll]", sch.pauses)
	}
	if got := aud.actions(); len(got) != 1 || got[0] != audit.ActionFlowPause {
		t.Errorf("audit actions = %v, want [flow.pause]", got)
	}
}

func TestPause_WrongState_409(t *testing.T) {
	// flw_leave is "paused" → pause is not a legal transition from paused.
	fake := seedFake()
	sch := &fakeScheduler{}
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, sch)

	w, body := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_leave/pause", nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	assertErrorEnvelope(t, body, "invalid_transition")
	if len(sch.pauses) != 0 || len(fake.statusSets) != 0 {
		t.Errorf("no side effects on invalid transition: pauses=%v sets=%v", sch.pauses, fake.statusSets)
	}
}

func TestPause_FlowNotFound_404(t *testing.T) {
	r := routerWithDeps(seedFake(), &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, _ := doReq(t, r, http.MethodPost, APIBasePath+"/flows/nope/pause", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestPause_SchedulerError_500(t *testing.T) {
	fake := seedFake()
	sch := &fakeScheduler{pauseErr: errors.New("temporal down")}
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, sch)
	w, body := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/pause", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	assertErrorEnvelope(t, body, "internal")
}

func TestPause_SetStatusError_500(t *testing.T) {
	fake := seedFake()
	fake.errWrite = errors.New("db down")
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, _ := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/pause", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestPause_GetFlowError_500(t *testing.T) {
	fake := seedFake()
	fake.errGetFlow = errors.New("db down")
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, _ := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/pause", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

// SetFlowStatus reporting NotFound after GetFlow succeeded (concurrent delete)
// maps to a 404, not a 500.
func TestPause_SetStatusNotFound_404(t *testing.T) {
	fake := seedFake()
	fake.writeNotFound = true
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, _ := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/pause", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// --- resume ---

func TestResume_HappyPath(t *testing.T) {
	fake := seedFake() // flw_leave is "paused"
	sch := &fakeScheduler{}
	aud := &fakeAudit{}
	r := routerWithDeps(fake, &fakeRunner{}, aud, sch)

	w, body := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_leave/resume", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body["status"] != "published" {
		t.Errorf("status = %v, want published", body["status"])
	}
	if len(fake.statusSets) != 1 || fake.statusSets[0].status != "published" {
		t.Errorf("statusSets = %+v, want one published", fake.statusSets)
	}
	if len(sch.resumes) != 1 || sch.resumes[0] != "flw_leave" {
		t.Errorf("scheduler resumes = %v, want [flw_leave]", sch.resumes)
	}
	if got := aud.actions(); len(got) != 1 || got[0] != audit.ActionFlowResume {
		t.Errorf("audit actions = %v, want [flow.resume]", got)
	}
}

func TestResume_WrongState_409(t *testing.T) {
	// flw_payroll is "published" → resume requires "paused".
	fake := seedFake()
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, body := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/resume", nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	assertErrorEnvelope(t, body, "invalid_transition")
}

func TestResume_SchedulerError_500(t *testing.T) {
	fake := seedFake()
	sch := &fakeScheduler{resumeErr: errors.New("temporal down")}
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, sch)
	w, _ := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_leave/resume", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

// --- stop ---

func TestStop_FromPublished(t *testing.T) {
	fake := seedFake() // flw_payroll published
	sch := &fakeScheduler{}
	aud := &fakeAudit{}
	r := routerWithDeps(fake, &fakeRunner{}, aud, sch)

	w, body := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/stop", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body["status"] != "stopped" {
		t.Errorf("status = %v, want stopped", body["status"])
	}
	if len(sch.deletes) != 1 || sch.deletes[0] != "flw_payroll" {
		t.Errorf("scheduler deletes = %v, want [flw_payroll]", sch.deletes)
	}
	if got := aud.actions(); len(got) != 1 || got[0] != audit.ActionFlowStop {
		t.Errorf("audit actions = %v, want [flow.stop]", got)
	}
}

func TestStop_FromPaused(t *testing.T) {
	fake := seedFake() // flw_leave paused
	sch := &fakeScheduler{}
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, sch)
	w, body := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_leave/stop", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body["status"] != "stopped" {
		t.Errorf("status = %v, want stopped", body["status"])
	}
}

func TestStop_WrongState_409(t *testing.T) {
	// flw_newhire is "draft" → stop not legal from draft.
	fake := seedFake()
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, body := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_newhire/stop", nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	assertErrorEnvelope(t, body, "invalid_transition")
}

func TestStop_SchedulerError_500(t *testing.T) {
	fake := seedFake()
	sch := &fakeScheduler{deleteErr: errors.New("temporal down")}
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, sch)
	w, _ := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/stop", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

// --- publish now requires changeNote + snapshots a version ---

func TestPublish_RequiresChangeNote_400(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	// empty change note
	w, body := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/publish", `{"changeNote":""}`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	assertErrorEnvelope(t, body, "bad_request")
	if len(fake.versionsCreated) != 0 {
		t.Errorf("no version should be created when changeNote missing")
	}
}

func TestPublish_MissingBody_400(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	// no body at all → changeNote empty → 400
	w, _ := doReq(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/publish", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestPublish_BadJSON_400(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/publish", `{bad`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestPublish_SnapshotsVersion(t *testing.T) {
	fake := seedFake() // flw_payroll version 3 → publish bumps to 4
	fake.def = sampleDef()
	aud := &fakeAudit{}
	r := routerWithDeps(fake, &fakeRunner{}, aud, &fakeScheduler{})

	w, body := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/publish",
		`{"changeNote":"tighten mapping"}`, map[string]string{roleHeader: "designer"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%v)", w.Code, body)
	}
	if len(fake.versionsCreated) != 1 {
		t.Fatalf("versionsCreated = %d, want 1", len(fake.versionsCreated))
	}
	vc := fake.versionsCreated[0]
	if vc.flowID != "flw_payroll" {
		t.Errorf("version flowID = %q, want flw_payroll", vc.flowID)
	}
	if vc.versionNo != 4 {
		t.Errorf("versionNo = %d, want 4 (bumped current version)", vc.versionNo)
	}
	if vc.changeNote != "tighten mapping" {
		t.Errorf("changeNote = %q, want tighten mapping", vc.changeNote)
	}
	if vc.publishedBy != "designer" {
		t.Errorf("publishedBy = %q, want designer", vc.publishedBy)
	}
	if got := aud.actions(); len(got) != 1 || got[0] != audit.ActionFlowPublish {
		t.Errorf("audit actions = %v, want [flow.publish]", got)
	}
}

// A CreateVersion error is non-fatal to publish (the store already committed the
// publish); it warns and still 200s.
func TestPublish_CreateVersionError_StillPublishes(t *testing.T) {
	fake := seedFake()
	fake.def = sampleDef()
	fake.errCreateVersion = errors.New("versions table down")
	r := routerWithLogger(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, body := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/publish",
		`{"changeNote":"note"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite version error", w.Code)
	}
	if body["status"] != "published" {
		t.Errorf("status = %v, want published", body["status"])
	}
}

func TestPublish_NotFound_404(t *testing.T) {
	fake := seedFake()
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/nope/publish", `{"changeNote":"x"}`, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestPublish_StoreError_500(t *testing.T) {
	fake := seedFake()
	fake.errWrite = errors.New("db down")
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_payroll/publish", `{"changeNote":"x"}`, nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

// --- versions list ---

func TestListVersions_NewestFirst(t *testing.T) {
	fake := seedFake()
	fake.versions = []storedVersion{
		{flowID: "flw_payroll", v: store.Version{VersionNo: 3, ChangeNote: "v3", PublishedBy: "admin", PublishedAt: "2026-07-12T00:00:00.000Z"}},
		{flowID: "flw_payroll", v: store.Version{VersionNo: 2, ChangeNote: "v2", PublishedBy: "admin", PublishedAt: "2026-07-10T00:00:00.000Z"}},
		{flowID: "flw_other", v: store.Version{VersionNo: 1, ChangeNote: "other"}},
	}
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows/flw_payroll/versions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	vs, ok := body["versions"].([]any)
	if !ok || len(vs) != 2 {
		t.Fatalf("versions = %v, want 2 for flw_payroll", body["versions"])
	}
	first := vs[0].(map[string]any)
	for _, key := range []string{"versionNo", "changeNote", "publishedBy", "publishedAt"} {
		if _, ok := first[key]; !ok {
			t.Errorf("version missing key %q: %v", key, first)
		}
	}
	if first["versionNo"].(float64) != 3 {
		t.Errorf("first versionNo = %v, want 3 (newest first)", first["versionNo"])
	}
}

func TestListVersions_Empty(t *testing.T) {
	r := routerWithDeps(seedFake(), &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows/flw_payroll/versions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	vs, ok := body["versions"].([]any)
	if !ok || len(vs) != 0 {
		t.Fatalf("versions = %v, want empty array", body["versions"])
	}
}

func TestListVersions_StoreError_500(t *testing.T) {
	fake := seedFake()
	fake.errVersions = errors.New("db down")
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows/flw_payroll/versions", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	assertErrorEnvelope(t, body, "internal")
}

// --- rollback ---

func TestRollback_HappyPath(t *testing.T) {
	fake := seedFake() // flw_gov is "stopped", version 4
	fake.versions = []storedVersion{
		{flowID: "flw_gov", v: store.Version{VersionNo: 2}, def: sampleDef()},
	}
	sch := &fakeScheduler{}
	aud := &fakeAudit{}
	r := routerWithDeps(fake, &fakeRunner{}, aud, sch)

	w, body := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_gov/rollback",
		`{"toVersion":2,"changeNote":"revert to v2"}`, map[string]string{roleHeader: "admin"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%v)", w.Code, body)
	}
	if body["status"] != "published" {
		t.Errorf("status = %v, want published", body["status"])
	}
	// The rolled-back definition became a fresh version (flw_gov v4 → 5).
	if len(fake.versionsCreated) != 1 {
		t.Fatalf("versionsCreated = %d, want 1", len(fake.versionsCreated))
	}
	vc := fake.versionsCreated[0]
	if vc.versionNo != 5 {
		t.Errorf("new versionNo = %d, want 5", vc.versionNo)
	}
	if vc.changeNote != "revert to v2" {
		t.Errorf("changeNote = %q, want revert to v2", vc.changeNote)
	}
	// The rollback re-published, so the scheduler was synced (sampleDef has no
	// schedule node → Delete is the idempotent call).
	if len(sch.deletes) != 1 {
		t.Errorf("scheduler deletes = %v, want 1 (schedule sync)", sch.deletes)
	}
	if got := aud.actions(); len(got) != 1 || got[0] != audit.ActionFlowRollback {
		t.Errorf("audit actions = %v, want [flow.rollback]", got)
	}
}

func TestRollback_MissingVersion_404(t *testing.T) {
	fake := seedFake()
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_gov/rollback",
		`{"toVersion":99,"changeNote":"x"}`, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if len(fake.versionsCreated) != 0 {
		t.Errorf("no version should be created when target is missing")
	}
}

func TestRollback_RequiresChangeNote_400(t *testing.T) {
	fake := seedFake()
	fake.versions = []storedVersion{{flowID: "flw_gov", v: store.Version{VersionNo: 2}, def: sampleDef()}}
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_gov/rollback",
		`{"toVersion":2,"changeNote":""}`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestRollback_BadJSON_400(t *testing.T) {
	r := routerWithDeps(seedFake(), &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_gov/rollback", `{bad`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestRollback_GetVersionError_500(t *testing.T) {
	fake := seedFake()
	fake.errGetVersion = errors.New("db down")
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_gov/rollback",
		`{"toVersion":2,"changeNote":"x"}`, nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestRollback_UpdateDefinitionError_500(t *testing.T) {
	fake := seedFake()
	fake.versions = []storedVersion{{flowID: "flw_gov", v: store.Version{VersionNo: 2}, def: sampleDef()}}
	fake.errWrite = errors.New("db down")
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_gov/rollback",
		`{"toVersion":2,"changeNote":"x"}`, nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

// A PublishFlow failure during rollback (after the definition was written back)
// surfaces the publish error — here a 500.
func TestRollback_PublishError_500(t *testing.T) {
	fake := seedFake()
	fake.versions = []storedVersion{{flowID: "flw_gov", v: store.Version{VersionNo: 2}, def: sampleDef()}}
	fake.errPublish = errors.New("publish db down")
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_gov/rollback",
		`{"toVersion":2,"changeNote":"x"}`, nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

// UpdateFlowDefinition reporting NotFound during rollback (flow deleted between
// version lookup and write-back) maps to a 404.
func TestRollback_UpdateDefinitionNotFound_404(t *testing.T) {
	fake := seedFake()
	fake.versions = []storedVersion{{flowID: "flw_gov", v: store.Version{VersionNo: 2}, def: sampleDef()}}
	fake.writeNotFound = true
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_gov/rollback",
		`{"toVersion":2,"changeNote":"x"}`, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// --- RBAC: pause/resume/stop/rollback need FlowPublish; versions need FlowView ---

func TestLifecycle_RBAC_Deny(t *testing.T) {
	// operator and viewer lack FlowPublish → 403 on mutating lifecycle actions.
	mutating := []struct {
		name, method, path, body string
	}{
		{"pause", http.MethodPost, "/flows/flw_payroll/pause", ""},
		{"resume", http.MethodPost, "/flows/flw_leave/resume", ""},
		{"stop", http.MethodPost, "/flows/flw_payroll/stop", ""},
		{"rollback", http.MethodPost, "/flows/flw_gov/rollback", `{"toVersion":2,"changeNote":"x"}`},
	}
	for _, role := range []string{"operator", "viewer"} {
		for _, m := range mutating {
			t.Run(role+"/"+m.name, func(t *testing.T) {
				fake := seedFake()
				fake.versions = []storedVersion{{flowID: "flw_gov", v: store.Version{VersionNo: 2}, def: sampleDef()}}
				r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
				w, body := doReqBody(t, r, m.method, APIBasePath+m.path, m.body,
					map[string]string{roleHeader: role})
				if w.Code != http.StatusForbidden {
					t.Fatalf("role %s %s status = %d, want 403", role, m.name, w.Code)
				}
				assertErrorEnvelope(t, body, "forbidden")
			})
		}
	}
}

func TestVersions_RBAC_ViewerAllowed(t *testing.T) {
	// viewer has FlowView → may read the version list.
	r := routerWithDeps(seedFake(), &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, _ := doReq(t, r, http.MethodGet, APIBasePath+"/flows/flw_payroll/versions",
		map[string]string{roleHeader: "viewer"})
	if w.Code != http.StatusOK {
		t.Fatalf("viewer versions status = %d, want 200", w.Code)
	}
}

func TestVersions_RBAC_UnknownRoleDenied(t *testing.T) {
	// an unknown role lacks FlowView → 403.
	r := routerWithDeps(seedFake(), &fakeRunner{}, &fakeAudit{}, &fakeScheduler{})
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/flows/flw_payroll/versions",
		map[string]string{roleHeader: "auditor"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	assertErrorEnvelope(t, body, "forbidden")
}

// The rollback loads the target definition then re-publishes it; confirm the
// definition threaded to the schedule sync is the pinned one (has a schedule).
func TestRollback_SyncsScheduledDefinition(t *testing.T) {
	fake := seedFake()
	fake.versions = []storedVersion{{flowID: "flw_gov", v: store.Version{VersionNo: 2}, def: scheduleDef()}}
	// GetFlowDefinition (used by the schedule sync) returns the rolled-back def.
	fake.def = scheduleDef()
	sch := &fakeScheduler{}
	r := routerWithDeps(fake, &fakeRunner{}, &fakeAudit{}, sch)
	w, _ := doReqBody(t, r, http.MethodPost, APIBasePath+"/flows/flw_gov/rollback",
		`{"toVersion":2,"changeNote":"restore scheduled"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(sch.syncs) != 1 {
		t.Fatalf("scheduler syncs = %d, want 1", len(sch.syncs))
	}
	if got := fake.versionsCreated; len(got) != 1 || got[0].def.Nodes[0].Type != "trigger.schedule" {
		t.Errorf("snapshotted def should be the rolled-back scheduled def: %+v", got)
	}
}
