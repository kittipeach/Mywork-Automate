package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/store"
)

// streamPollInterval is how often streamExecution re-reads the execution and
// emits its current state. It is a package var (not a const) so tests can shrink
// it to keep the terminal-detection test fast.
var streamPollInterval = 1 * time.Second

// streamMaxDuration caps how long a single SSE connection stays open before the
// handler returns even if the run has not reached a terminal state. It bounds
// server resources against a stuck run or an abandoned tab.
var streamMaxDuration = 2 * time.Minute

// isTerminalExecStatus reports whether an execution status is final — the stream
// emits that state once more and then closes. skipped joins the run-detail
// terminal set (a run whose only branch was skipped ends "skipped") alongside the
// cancel/finish set already used by execution control.
func isTerminalExecStatus(status string) bool {
	switch status {
	case "success", "failed", "cancelled", "skipped":
		return true
	default:
		return false
	}
}

// getExecution is the store read the stream loop polls; a seam so streamLoop can
// be unit-tested with a fake that returns running→success across reads.
type execReader interface {
	GetExecution(ctx context.Context, id string) (store.Execution, error)
}

// streamExecution → GET /executions/{id}/stream. Server-Sent Events: every
// streamPollInterval it re-reads the execution and emits `data: {json}\n\n`,
// stopping when the run reaches a terminal state (success/failed/cancelled/
// skipped), the client disconnects, or streamMaxDuration elapses. Requires
// RunView. A 404 (unknown execution) is emitted as an SSE error event and the
// stream closes. The heavy lifting lives in streamLoop so it is unit-testable
// with an httptest recorder and a fake store.
func (h *handlers) streamExecution(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering

	streamLoop(c.Request.Context(), c.Writer, h.store, c.Param("id"),
		streamPollInterval, streamMaxDuration)
}

// flusher is the subset of http.Flusher the loop needs; httptest.ResponseRecorder
// satisfies it, so the loop is testable without a real connection.
type flusher interface{ Flush() }

// streamLoop drives the SSE emission. It writes the execution's current JSON on
// each tick (starting immediately), flushing after each event so the client sees
// live updates, and returns when: the run is terminal (after emitting its final
// state), ctx is cancelled (client disconnect), or maxDuration elapses. A store
// read error is emitted as an `event: error` frame and stops the stream (a
// not-found execution ends the stream cleanly rather than hanging).
//
// It is deliberately independent of gin so a test can pass an
// httptest.ResponseRecorder and a fake store.
func streamLoop(ctx context.Context, w io.Writer, store execReader, id string, interval, maxDuration time.Duration) {
	deadline := time.NewTimer(maxDuration)
	defer deadline.Stop()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// emit reads the execution and writes one SSE frame. It returns done=true when
	// the stream should stop (terminal state or a read error).
	emit := func() (done bool) {
		exec, err := store.GetExecution(ctx, id)
		if err != nil {
			writeSSEError(w, err.Error())
			return true
		}
		writeSSEData(w, exec)
		return isTerminalExecStatus(exec.Status)
	}

	// Emit the initial state immediately so a client that connects to an
	// already-terminal run still receives (and only receives) that final event.
	if emit() {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			return
		case <-ticker.C:
			if emit() {
				return
			}
		}
	}
}

// writeSSEData marshals v and writes it as a `data:` SSE frame, flushing if the
// writer supports it. A marshal error is swallowed (nothing sensible to emit for
// a malformed record) — the next tick retries.
func writeSSEData(w io.Writer, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n\n"))
	flush(w)
}

// writeSSEError writes an `event: error` frame carrying msg as its data.
func writeSSEError(w io.Writer, msg string) {
	b, _ := json.Marshal(gin.H{"error": msg})
	_, _ = w.Write([]byte("event: error\ndata: "))
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n\n"))
	flush(w)
}

// flush pushes buffered bytes to the client when the writer is a Flusher (gin's
// ResponseWriter and httptest.ResponseRecorder both are), so SSE frames are seen
// live rather than at connection close.
func flush(w io.Writer) {
	if f, ok := w.(flusher); ok {
		f.Flush()
	}
}
