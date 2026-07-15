package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/flowvalidate"
)

// validateFlow → POST /flows/{id}/validate (docs/spec/08 E3-S4, FR-CANVAS-007,008).
// It loads the flow's definition (404 when the flow is unknown or has no
// definition), runs the pure graph validator and returns
//
//	{"valid": bool, "issues": [ {nodeId, severity, message}, ... ]}
//
// where valid is true iff no issue is error-severity (warnings leave a flow
// valid). issues always serialises as an array, never null. Gated on FlowView
// (anyone who can view/design a flow can validate it).
func (h *handlers) validateFlow(c *gin.Context) {
	def, err := h.store.GetFlowDefinition(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeStoreError(c, err)
		return
	}

	// Validate always returns a non-nil slice (empty for a clean flow), so issues
	// serialises as [] not null without any extra guard here.
	issues := flowvalidate.Validate(def)
	c.JSON(http.StatusOK, gin.H{
		"valid":  !flowvalidate.HasErrors(issues),
		"issues": issues,
	})
}
