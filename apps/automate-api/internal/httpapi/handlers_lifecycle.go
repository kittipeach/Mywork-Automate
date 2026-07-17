package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/audit"
	"github.com/mywork/automate/apps/automate-api/internal/lifecycle"
	"github.com/mywork/automate/apps/automate-api/internal/store"
)

// pauseFlow → POST /flows/{id}/pause → 200 {status:"paused"}. Legal only from
// "published" (lifecycle.CanTransition); otherwise 409 invalid_transition. It
// sets the status then pauses the Temporal schedule (in-flight runs finish).
func (h *handlers) pauseFlow(c *gin.Context) {
	h.transition(c, lifecycle.StatusPaused, audit.ActionFlowPause, func(id string) error {
		return h.sched.Pause(c.Request.Context(), id)
	})
}

// resumeFlow → POST /flows/{id}/resume → 200 {status:"published"}. Legal only
// from "paused"; otherwise 409. It restores the status then resumes the schedule.
func (h *handlers) resumeFlow(c *gin.Context) {
	h.transition(c, lifecycle.StatusPublished, audit.ActionFlowResume, func(id string) error {
		return h.sched.Resume(c.Request.Context(), id)
	})
}

// stopFlow → POST /flows/{id}/stop → 200 {status:"stopped"}. Legal from
// "published" or "paused"; otherwise 409. It sets the status then deletes the
// schedule entirely.
func (h *handlers) stopFlow(c *gin.Context) {
	h.transition(c, lifecycle.StatusStopped, audit.ActionFlowStop, func(id string) error {
		return h.sched.Delete(c.Request.Context(), id)
	})
}

// transition is the shared pause/resume/stop path. It loads the flow (404 when
// unknown), validates from→to against the lifecycle state machine (409 when
// illegal), persists the new status, runs the scheduler op (500 on failure — the
// schedule and the status must not diverge for a lifecycle command, unlike the
// best-effort sync on publish), audits and returns {status: to}.
func (h *handlers) transition(c *gin.Context, to, action string, schedOp func(id string) error) {
	id := c.Param("id")
	ctx := c.Request.Context()

	flow, err := h.store.GetFlow(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
			return
		}
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	if !lifecycle.CanTransition(flow.Status, to) {
		errorEnvelope(c, http.StatusConflict, "invalid_transition",
			"cannot transition flow from "+flow.Status+" to "+to)
		return
	}

	if err := h.store.SetFlowStatus(ctx, id, to); err != nil {
		if store.IsNotFound(err) {
			errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
			return
		}
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	if err := schedOp(id); err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	h.record(c, audit.Entry{
		Action:       action,
		ResourceType: "flow",
		ResourceID:   id,
		Detail:       map[string]any{"status": to},
	})

	c.JSON(http.StatusOK, gin.H{"status": to})
}

// listVersions → GET /flows/{id}/versions → {versions:[...]} newest first.
func (h *handlers) listVersions(c *gin.Context) {
	versions, err := h.store.ListVersions(c.Request.Context(), c.Param("id"))
	if err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

// rollbackFlow → POST /flows/{id}/rollback {toVersion, changeNote} → 200. It
// loads the pinned definition of toVersion (404 when that version is unknown),
// writes it back as the current draft, then publishes — creating a NEW version
// (rollback never mutates history), syncing the schedule and auditing
// flow.rollback. changeNote is mandatory (400 when empty).
func (h *handlers) rollbackFlow(c *gin.Context) {
	var body struct {
		ToVersion  int    `json:"toVersion"`
		ChangeNote string `json:"changeNote"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if body.ChangeNote == "" {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", "changeNote is required")
		return
	}

	id := c.Param("id")
	ctx := c.Request.Context()

	def, err := h.store.GetVersionDefinition(ctx, id, body.ToVersion)
	if err != nil {
		if store.IsNotFound(err) {
			errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
			return
		}
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	if err := h.store.UpdateFlowDefinition(ctx, id, def); err != nil {
		if store.IsNotFound(err) {
			errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
			return
		}
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	flow, err := h.publish(c, id, body.ChangeNote, audit.ActionFlowRollback)
	if err != nil {
		return // publish already wrote the error envelope
	}
	c.JSON(http.StatusOK, flow)
}
