package httpapi

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/audit"
	"github.com/mywork/automate/apps/automate-api/internal/store"
)

// deleteFlow → DELETE /flows/{id}. Soft delete (E3-S6): it stamps deleted_at on
// the flow without changing its status, so the flow drops out of the default
// /flows list but stays fully restorable. Requires FlowCreate. Returns 204 and
// audits flow.delete. 404 when the flow is unknown.
func (h *handlers) deleteFlow(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.SoftDeleteFlow(c.Request.Context(), id); err != nil {
		h.writeStoreError(c, err)
		return
	}
	h.record(c, audit.Entry{
		Action:       audit.ActionFlowDelete,
		ResourceType: "flow",
		ResourceID:   id,
	})
	c.Status(http.StatusNoContent)
}

// restoreFlow → POST /flows/{id}/restore. Clears deleted_at, returning a
// soft-deleted flow to the default list (E3-S6). Admin-only (gated by
// RequireAdmin on the route — restoring is a privileged recovery action).
// Returns 200 {status:"restored"} and audits flow.restore. 404 when unknown.
func (h *handlers) restoreFlow(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.RestoreFlow(c.Request.Context(), id); err != nil {
		h.writeStoreError(c, err)
		return
	}
	h.record(c, audit.Entry{
		Action:       audit.ActionFlowRestore,
		ResourceType: "flow",
		ResourceID:   id,
	})
	c.JSON(http.StatusOK, gin.H{"status": "restored"})
}

// listGrants → GET /flows/{id}/grants → {grants:[{subjectType,subjectId,access}]}.
// Object-level access grants on a flow (E2-S3). Gated on FlowPublish (admin or a
// flow owner in the object-level model layered on later). The full Grant DTO is
// serialised (it also carries id/flowId) so the UI can address a grant for delete.
func (h *handlers) listGrants(c *gin.Context) {
	grants, err := h.store.ListGrants(c.Request.Context(), c.Param("id"))
	if err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"grants": grants})
}

// grantAccessLevels / grantSubjectTypes are the closed vocabularies validated on
// create (E2-S3). access is viewer|editor|owner; subject_type is role|user.
var grantAccessLevels = map[string]bool{"viewer": true, "editor": true, "owner": true}
var grantSubjectTypes = map[string]bool{"role": true, "user": true}

// addGrant → POST /flows/{id}/grants {subjectType,subjectId,access} → 201 the new
// grant. Validates the closed vocabularies (400 on an unknown subjectType/access
// or an empty subjectId). Gated on FlowPublish. Audits rbac.grant_change.
func (h *handlers) addGrant(c *gin.Context) {
	var in store.GrantInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if !grantSubjectTypes[in.SubjectType] {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", "subjectType must be role or user")
		return
	}
	if in.SubjectID == "" {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", "subjectId is required")
		return
	}
	if !grantAccessLevels[in.Access] {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", "access must be viewer, editor or owner")
		return
	}

	flowID := c.Param("id")
	grant, err := h.store.AddGrant(c.Request.Context(), flowID, in)
	if err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.record(c, audit.Entry{
		Action:       audit.ActionRBACGrantChange,
		ResourceType: "flow",
		ResourceID:   flowID,
		Detail: map[string]any{
			"op":          "add",
			"grantId":     grant.ID,
			"subjectType": grant.SubjectType,
			"subjectId":   grant.SubjectID,
			"access":      grant.Access,
		},
	})
	c.JSON(http.StatusCreated, grant)
}

// deleteGrant → DELETE /flows/{id}/grants/{grantId} → 204. Removes one grant by
// id (scoped to the flow). A non-numeric grantId is a 400; an unknown grant is a
// 404. Gated on FlowPublish. Audits rbac.grant_change.
func (h *handlers) deleteGrant(c *gin.Context) {
	flowID := c.Param("id")
	grantID, err := strconv.ParseInt(c.Param("grantId"), 10, 64)
	if err != nil {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", "grantId must be an integer")
		return
	}
	if err := h.store.DeleteGrant(c.Request.Context(), flowID, grantID); err != nil {
		h.writeStoreError(c, err)
		return
	}
	h.record(c, audit.Entry{
		Action:       audit.ActionRBACGrantChange,
		ResourceType: "flow",
		ResourceID:   flowID,
		Detail:       map[string]any{"op": "delete", "grantId": grantID},
	})
	c.Status(http.StatusNoContent)
}
