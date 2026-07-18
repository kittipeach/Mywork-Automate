package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/audit"
	"github.com/mywork/automate/apps/automate-api/internal/notify"
	"github.com/mywork/automate/apps/automate-api/internal/scheduler"
	"github.com/mywork/automate/apps/automate-api/internal/store"
	"github.com/mywork/automate/internal/flowspec"
)

// newExecID returns a fresh execution id (exe_<random hex>). crypto/rand keeps
// ids unguessable; 8 bytes is ample at control-plane volumes.
func newExecID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "exe_" + hex.EncodeToString(b[:])
}

// runFlow → POST /flows/{id}/run. It loads the flow definition (404 if none),
// records a "running" execution, then hands the run to the Runner. The onDone
// callback maps the FlowResult into ExecutionStep rows and calls FinishExecution
// (status "success", or "failed" carrying the error). Returns 202 with the new
// execution id. If no Runner is configured (Temporal unavailable) it 503s.
func (h *handlers) runFlow(c *gin.Context) {
	if h.runner == nil {
		errorEnvelope(c, http.StatusServiceUnavailable, "runner_unavailable",
			"flow runner is not available (temporal unreachable)")
		return
	}

	var body struct {
		Params map[string]any `json:"params"`
	}
	// A missing/empty body is fine (params optional); only reject malformed JSON.
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			errorEnvelope(c, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
	}

	id := c.Param("id")

	def, flow, ok := h.loadRunnable(c, id)
	if !ok {
		return
	}

	execID, ok := h.startRun(c, def, flow, body.Params)
	if !ok {
		return
	}

	h.record(c, audit.Entry{
		Action:       audit.ActionExecutionManualRun,
		ResourceType: "flow",
		ResourceID:   flow.ID,
		Detail:       map[string]any{"executionId": execID},
	})

	c.JSON(http.StatusAccepted, gin.H{"executionId": execID})
}

// loadRunnable loads a flow's runnable definition and summary, writing the
// appropriate error envelope and returning ok=false on any failure (404 when the
// flow is unknown or has no definition, 500 otherwise). It is shared by runFlow
// and retryExecution.
func (h *handlers) loadRunnable(c *gin.Context, id string) (flowspec.FlowDef, store.FlowSummary, bool) {
	ctx := c.Request.Context()

	def, err := h.store.GetFlowDefinition(ctx, id)
	if err != nil {
		h.writeStoreError(c, err)
		return flowspec.FlowDef{}, store.FlowSummary{}, false
	}

	flow, err := h.store.GetFlow(ctx, id)
	if err != nil {
		h.writeStoreError(c, err)
		return flowspec.FlowDef{}, store.FlowSummary{}, false
	}

	// Resolve each db.query node's connectionId to its stored dial fields so the
	// worker dials the right external database (E6-S1). A referenced-but-missing
	// connection is a 400 (the flow can't run as designed).
	def, err = h.resolveConnections(ctx, def)
	if err != nil {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", err.Error())
		return flowspec.FlowDef{}, store.FlowSummary{}, false
	}
	return def, flow, true
}

// resolveConnections injects the stored connection's dial fields (host/port/
// database/username/sslMode + the password's secretRef) into every db.query
// node that names a connectionId. The password itself is never inlined — only
// its secret name — so it is resolved in the worker at query time. Nodes without
// a connectionId are left untouched (they use the worker's default pool).
func (h *handlers) resolveConnections(ctx context.Context, def flowspec.FlowDef) (flowspec.FlowDef, error) {
	for i, n := range def.Nodes {
		if n.Type != "db.query" || len(n.Config) == 0 {
			continue
		}
		var cfg map[string]any
		if err := json.Unmarshal(n.Config, &cfg); err != nil {
			continue // malformed config is caught later by the executor
		}
		connID, _ := cfg["connectionId"].(string)
		if connID == "" {
			continue
		}
		conn, err := h.store.GetConnection(ctx, connID)
		if err != nil {
			if store.IsNotFound(err) {
				return def, fmt.Errorf("db.query node %q references unknown connection %q", n.ID, connID)
			}
			return def, fmt.Errorf("resolve connection %q: %w", connID, err)
		}
		cfg["connHost"] = conn.Host
		cfg["connPort"] = conn.Port
		cfg["connDatabase"] = conn.Database
		cfg["connUsername"] = conn.Username
		cfg["connSslMode"] = conn.SSLMode
		cfg["connSecret"] = conn.SecretRef
		raw, err := json.Marshal(cfg)
		if err != nil {
			return def, fmt.Errorf("re-marshal node %q config: %w", n.ID, err)
		}
		def.Nodes[i].Config = raw
	}
	return def, nil
}

// startRun records a "running" execution, hands the run to the Runner and wires
// the onDone finaliser (mirroring the terminal outcome via FinishExecution). It
// writes the error envelope and returns ok=false on failure. The returned execID
// is the new execution id. Shared by runFlow (manual run) and retryExecution.
func (h *handlers) startRun(c *gin.Context, def flowspec.FlowDef, flow store.FlowSummary, params map[string]any) (string, bool) {
	ctx := c.Request.Context()
	role := string(callerRole(c))

	startedAt := time.Now()
	execID := newExecID()
	exec := store.Execution{
		ID:        execID,
		FlowID:    flow.ID,
		FlowName:  flow.Name,
		Status:    "running",
		Trigger:   "manual",
		StartedAt: startedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		Version:   flow.Version,
	}
	if err := h.store.CreateExecution(ctx, exec); err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return "", false
	}

	in := flowspec.FlowInput{
		Flow:        def,
		ViewerRoles: []string{role},
		Params:      params,
	}

	// onDone records the terminal outcome. It runs in the caller's goroutine for
	// the synchronous fake (tests) and on a background goroutine for the real
	// Temporal runner, so it uses a detached context and never touches gin.
	onDone := func(res flowspec.FlowResult, runErr error) {
		durationMs := time.Since(startedAt).Milliseconds()
		steps := buildSteps(def, res, runErr)
		status := "success"
		if runErr != nil {
			status = "failed"
		}
		ctx := context.Background()
		_ = h.store.FinishExecution(ctx, execID, status, durationMs, steps)

		// E11-S3: emit a run-correlated completion event so ELK can join the
		// control-plane record to the worker's logs. run_id is the execution id
		// (the worker side already stamps WorkflowID via Temporal's logger), and
		// flow_id/status/duration_ms round out the correlation fields.
		h.info("flow run finished",
			"run_id", execID,
			"flow_id", flow.ID,
			"status", status,
			"duration_ms", durationMs)

		// E5-S6: on a failed run, alert the flow's configured recipients. Delivery
		// is best-effort — a notify error must not affect the run's recording, so
		// it is logged at WARN and swallowed. No recipients => no-op notifier.
		if runErr != nil {
			if recipients := failureRecipients(def); len(recipients) > 0 {
				if err := h.notifier.RunFailed(ctx, notify.FailureNotice{
					FlowName:    flow.Name,
					ExecutionID: execID,
					Error:       runErr.Error(),
					Recipients:  recipients,
				}); err != nil {
					h.warn("run-failure notify failed", "executionId", execID, "err", err)
				}
			}
		}
	}

	if err := h.runner.Run(ctx, execID, in, onDone); err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return "", false
	}
	return execID, true
}

// failureRecipients returns the flow's configured run-failure email recipients
// (E5-S6), reading def.Settings.Notification.FailureEmails nil-safely. Any
// missing level of the settings tree yields nil (no recipients → no email).
func failureRecipients(def flowspec.FlowDef) []string {
	if def.Settings == nil || def.Settings.Notification == nil {
		return nil
	}
	return def.Settings.Notification.FailureEmails
}

// writeStoreError maps a store error to the standard envelope: 404 for NotFound,
// 500 otherwise.
func (h *handlers) writeStoreError(c *gin.Context, err error) {
	if store.IsNotFound(err) {
		errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
		return
	}
	errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
}

// sampleLimit caps how many items are copied into a step's I/O snapshot
// (docs/spec/08 E5-S3). The spec sets the hard ceiling at 1,000 items; we keep a
// smaller 50-item head sample per step so run-detail payloads stay light — the
// full item COUNT is preserved separately in OutputCount, so nothing about the
// run's true size is lost.
const sampleLimit = 50

// buildSteps maps a FlowResult into execution steps. Each executed node in
// result.Path becomes a "success" step; outputCount comes from the node's
// Meta["rowCount"] if present, else len(Items). On a run error the final step
// (the last node that ran, or a synthetic one when nothing ran) is marked
// "failed" and carries the error message.
//
// It also captures the per-step I/O snapshot (E5-S3): OutputSample is the node's
// emitted items truncated to sampleLimit; InputSample is best-effort the
// predecessor step's (already-truncated) output. The items are ALREADY masked by
// the executors before they reach here, so buildSteps never re-masks them.
func buildSteps(def flowspec.FlowDef, res flowspec.FlowResult, runErr error) []store.ExecutionStep {
	steps := make([]store.ExecutionStep, 0, len(res.Path))
	for i, nodeID := range res.Path {
		out := res.Outputs[nodeID]
		step := store.ExecutionStep{
			NodeID:       nodeID,
			NodeName:     nodeName(def, nodeID),
			NodeType:     nodeType(def, nodeID),
			Status:       "success",
			OutputCount:  outputCount(out),
			OutputSample: truncate(out.Items, sampleLimit),
		}
		// A node run with onError=continue/errorBranch records its failure in
		// Outputs[...].Meta["error"]; surface it as a failed step so the audit
		// trail isn't misleadingly green.
		if out.Meta != nil {
			if e, ok := out.Meta["error"].(string); ok && e != "" {
				step.Status = "failed"
				step.Error = &e
			}
		}
		// Input sample = the previous step's output sample (best-effort chain).
		// The first step has no predecessor, so its input sample stays nil.
		if i > 0 {
			step.InputSample = steps[i-1].OutputSample
		}
		steps = append(steps, step)
	}

	if runErr != nil {
		msg := runErr.Error()
		if len(steps) > 0 {
			last := &steps[len(steps)-1]
			last.Status = "failed"
			last.Error = &msg
		} else {
			// Nothing ran (e.g. trigger resolution failed): synthesise one failed
			// step so the execution detail still explains why it failed.
			steps = append(steps, store.ExecutionStep{
				NodeID:   "flow",
				NodeName: "flow",
				NodeType: "flow",
				Status:   "failed",
				Error:    &msg,
			})
		}
	}
	return steps
}

// truncate returns items capped at n. A nil/short slice is returned unchanged
// (nil stays nil so the DTO omits the sample); a longer slice is cut to its head
// so the sample is representative and cheap. It does not copy — the items are
// already the executor's masked output and are only read after this point.
func truncate(items []map[string]any, n int) []map[string]any {
	if len(items) <= n {
		return items
	}
	return items[:n]
}

// outputCount prefers the node's reported rowCount metadata, falling back to the
// number of items it emitted.
func outputCount(out flowspec.NodeOutput) int64 {
	if out.Meta != nil {
		if rc, ok := out.Meta["rowCount"]; ok {
			if n, ok := toInt64(rc); ok {
				return n
			}
		}
	}
	return int64(len(out.Items))
}

// toInt64 coerces a JSON-decoded number (which may arrive as float64, json
// numbers, or ints depending on the source) into an int64.
func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func nodeName(def flowspec.FlowDef, id string) string {
	for _, n := range def.Nodes {
		if n.ID == id {
			if n.Name != "" {
				return n.Name
			}
			return id
		}
	}
	return id
}

func nodeType(def flowspec.FlowDef, id string) string {
	for _, n := range def.Nodes {
		if n.ID == id {
			return n.Type
		}
	}
	return ""
}

// createFlow → POST /flows {name, folder} → 201 the new draft flow.
func (h *handlers) createFlow(c *gin.Context) {
	var body struct {
		Name   string `json:"name"`
		Folder string `json:"folder"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if body.Name == "" {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", "name is required")
		return
	}
	flow, err := h.store.CreateFlow(c.Request.Context(), body.Name, body.Folder)
	if err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.record(c, audit.Entry{
		Action:       audit.ActionFlowCreate,
		ResourceType: "flow",
		ResourceID:   flow.ID,
		Detail:       map[string]any{"name": flow.Name},
	})
	c.JSON(http.StatusCreated, flow)
}

// updateFlowDraft → PUT /flows/{id}/draft {definition} → 200.
func (h *handlers) updateFlowDraft(c *gin.Context) {
	var body struct {
		Definition flowspec.FlowDef `json:"definition"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	err := h.store.UpdateFlowDefinition(c.Request.Context(), c.Param("id"), body.Definition)
	if err != nil {
		if store.IsNotFound(err) {
			errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
			return
		}
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// publishFlow → POST /flows/{id}/publish {changeNote} → 200 the published flow.
// changeNote is mandatory (E4-S2: publish must carry a change note) — an empty
// note is a 400. After the store publishes (bumping current_version), it snapshots
// the current definition as an immutable version row (versionNo = the flow's new
// current version), then syncs the flow's recurring schedule to the scheduler:
// SpecFromDefinition compiles the definition, then Sync (when the flow has a
// trigger.schedule node) or Delete (idempotent — the flow no longer schedules
// itself). A scheduler/version/definition error does not fail the publish (it
// already committed): it is logged at WARN and the published flow is still returned.
func (h *handlers) publishFlow(c *gin.Context) {
	changeNote, ok := bindChangeNote(c)
	if !ok {
		return
	}
	flow, err := h.publish(c, c.Param("id"), changeNote, audit.ActionFlowPublish)
	if err != nil {
		return // publish already wrote the error envelope
	}
	c.JSON(http.StatusOK, flow)
}

// bindChangeNote decodes the mandatory {changeNote} body and writes a 400 error
// envelope (returning ok=false) when the body is malformed or the note is empty.
func bindChangeNote(c *gin.Context) (string, bool) {
	var body struct {
		ChangeNote string `json:"changeNote"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			errorEnvelope(c, http.StatusBadRequest, "bad_request", err.Error())
			return "", false
		}
	}
	if body.ChangeNote == "" {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", "changeNote is required")
		return "", false
	}
	return body.ChangeNote, true
}

// publish commits the store publish, snapshots the definition as a new version,
// audits the given action and syncs the schedule. It writes the error envelope
// itself on failure (returning a non-nil error) so callers just return; on
// success it returns the published flow summary. action lets rollback record
// flow.rollback while normal publish records flow.publish.
func (h *handlers) publish(c *gin.Context, id, changeNote, action string) (store.FlowSummary, error) {
	ctx := c.Request.Context()

	flow, err := h.store.PublishFlow(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
		} else {
			errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		}
		return store.FlowSummary{}, err
	}

	h.record(c, audit.Entry{
		Action:       action,
		ResourceType: "flow",
		ResourceID:   flow.ID,
		Detail:       map[string]any{"version": flow.Version, "changeNote": changeNote},
	})

	h.snapshotVersion(c, flow, changeNote)
	h.syncSchedule(c, flow.ID)

	return flow, nil
}

// snapshotVersion pins the flow's current definition as an immutable version row
// (versionNo = the flow's post-publish current version, published_by = the caller
// role). Any failure is non-fatal to publish — the store already committed — so it
// is logged at WARN and swallowed (matching the schedule-sync policy).
func (h *handlers) snapshotVersion(c *gin.Context, flow store.FlowSummary, changeNote string) {
	ctx := c.Request.Context()

	def, err := h.store.GetFlowDefinition(ctx, flow.ID)
	if err != nil {
		h.warn("publish: load definition for version snapshot failed", "flowId", flow.ID, "err", err)
		return
	}
	if err := h.store.CreateVersion(ctx, flow.ID, flow.Version, def, changeNote, string(callerRole(c))); err != nil {
		h.warn("publish: snapshot version failed", "flowId", flow.ID, "err", err)
	}
}

// syncSchedule keeps flowID's recurring schedule in step with its published
// definition. It loads the definition, compiles it with SpecFromDefinition, then
// Sync's the schedule if the flow has one or Delete's it (idempotent) if it does
// not. Every failure here is non-fatal to publish — the store already committed —
// so each is logged at WARN and swallowed.
func (h *handlers) syncSchedule(c *gin.Context, flowID string) {
	ctx := c.Request.Context()

	def, err := h.store.GetFlowDefinition(ctx, flowID)
	if err != nil {
		h.warn("publish: load definition for schedule sync failed", "flowId", flowID, "err", err)
		return
	}

	spec, hasSchedule, err := scheduler.SpecFromDefinition(def)
	if err != nil {
		h.warn("publish: compile schedule spec failed", "flowId", flowID, "err", err)
		return
	}

	if !hasSchedule {
		if err := h.sched.Delete(ctx, flowID); err != nil {
			h.warn("publish: delete schedule failed", "flowId", flowID, "err", err)
		}
		return
	}

	in := flowspec.FlowInput{Flow: def, ViewerRoles: []string{string(callerRole(c))}}
	if err := h.sched.Sync(ctx, flowID, spec, in); err != nil {
		h.warn("publish: sync schedule failed", "flowId", flowID, "err", err)
	}
}

// warn logs at WARN when a logger is configured; nil is tolerated (no-op).
func (h *handlers) warn(msg string, kv ...any) {
	if h.log != nil {
		h.log.Warn(msg, kv...)
	}
}

// info logs at INFO when a logger is configured; nil is tolerated (no-op). It
// backs the run-completion correlation event (E11-S3) so ELK gets a structured,
// run-correlated record carrying run_id/flow_id/status/duration_ms.
func (h *handlers) info(msg string, kv ...any) {
	if h.log != nil {
		h.log.Info(msg, kv...)
	}
}

// stashConnPassword moves a raw password (dev convenience) out of the connection
// input and into the secret store, recording only the generated SecretRef. It is
// the enforcement point for "credentials never touch the control-plane DB": the
// returned input carries a SecretRef, never a Password. When no password is
// supplied it is a no-op (the caller provided a SecretRef, or none is needed).
func (h *handlers) stashConnPassword(ctx context.Context, in store.ConnectionInput) (store.ConnectionInput, bool) {
	if in.Password == "" {
		return in, true
	}
	if h.secretsWriter == nil {
		return in, false
	}
	name := in.SecretRef
	if name == "" {
		buf := make([]byte, 16)
		_, _ = rand.Read(buf)
		name = "connpw_" + hex.EncodeToString(buf)
	}
	if err := h.secretsWriter.Write(ctx, name, in.Password); err != nil {
		return in, false
	}
	in.SecretRef = name
	in.Password = ""
	return in, true
}

// createConnection → POST /connections → 201 the new connection.
func (h *handlers) createConnection(c *gin.Context) {
	var in store.ConnectionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if in.Name == "" || in.Type == "" {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", "name and type are required")
		return
	}
	in, ok := h.stashConnPassword(c.Request.Context(), in)
	if !ok {
		errorEnvelope(c, http.StatusBadRequest, "bad_request",
			"cannot save password: no secret store configured — set secretRef to an existing secret instead")
		return
	}
	conn, err := h.store.CreateConnection(c.Request.Context(), in)
	if err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.record(c, audit.Entry{
		Action:       audit.ActionConnectionCreate,
		ResourceType: "connection",
		ResourceID:   conn.ID,
		Detail:       map[string]any{"name": conn.Name, "type": conn.Type},
	})
	c.JSON(http.StatusCreated, conn)
}

// updateConnection → PUT /connections/{id} → 200 the updated connection.
func (h *handlers) updateConnection(c *gin.Context) {
	var in store.ConnectionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	in, ok := h.stashConnPassword(c.Request.Context(), in)
	if !ok {
		errorEnvelope(c, http.StatusBadRequest, "bad_request",
			"cannot save password: no secret store configured — set secretRef to an existing secret instead")
		return
	}
	conn, err := h.store.UpdateConnection(c.Request.Context(), c.Param("id"), in)
	if err != nil {
		if store.IsNotFound(err) {
			errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
			return
		}
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.record(c, audit.Entry{
		Action:       audit.ActionConnectionUpdate,
		ResourceType: "connection",
		ResourceID:   conn.ID,
		Detail:       map[string]any{"name": conn.Name, "type": conn.Type},
	})
	c.JSON(http.StatusOK, conn)
}

// deleteConnection → DELETE /connections/{id} → 204.
func (h *handlers) deleteConnection(c *gin.Context) {
	id := c.Param("id")
	err := h.store.DeleteConnection(c.Request.Context(), id)
	if err != nil {
		if store.IsNotFound(err) {
			errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
			return
		}
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.record(c, audit.Entry{
		Action:       audit.ActionConnectionDelete,
		ResourceType: "connection",
		ResourceID:   id,
	})
	c.Status(http.StatusNoContent)
}

// testConnection → POST /connections/{id}/test. Live reachability probe: resolve
// the connection's password from the secret store, build its DSN and dial it
// (SELECT 1). The result is reported in the body — a failed probe is a 200 with
// status:"error" (the request succeeded; the target is what's unreachable), not
// an HTTP error. The password is never echoed; only the dial outcome is.
func (h *handlers) testConnection(c *gin.Context) {
	ctx := c.Request.Context()
	conn, err := h.store.GetConnection(ctx, c.Param("id"))
	if err != nil {
		if store.IsNotFound(err) {
			errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
			return
		}
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if h.dialer == nil {
		c.JSON(http.StatusOK, gin.H{"status": "unknown", "message": "connection testing is not configured on this server"})
		return
	}
	password := ""
	if conn.SecretRef != "" {
		if h.secretsResolver == nil {
			c.JSON(http.StatusOK, gin.H{"status": "error", "message": "secret store not configured"})
			return
		}
		if password, err = h.secretsResolver.Resolve(ctx, conn.SecretRef); err != nil {
			c.JSON(http.StatusOK, gin.H{"status": "error", "message": "could not resolve the connection secret"})
			return
		}
	}
	dsn := conn.DSN(password)
	if dsn == "" {
		c.JSON(http.StatusOK, gin.H{"status": "error", "message": "connection is missing host / database / username (postgres only)"})
		return
	}
	if err := h.dialer.Ping(ctx, dsn); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": "error", "message": "could not connect: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "connected"})
}
