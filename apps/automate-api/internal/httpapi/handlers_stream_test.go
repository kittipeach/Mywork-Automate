package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mywork/automate/apps/automate-api/internal/store"
)

func TestIsTerminalExecStatus(t *testing.T) {
	terminal := []string{"success", "failed", "cancelled", "skipped"}
	for _, s := range terminal {
		if !isTerminalExecStatus(s) {
			t.Errorf("%q should be terminal", s)
		}
	}
	nonTerminal := []string{"running", "queued", "", "paused"}
	for _, s := range nonTerminal {
		if isTerminalExecStatus(s) {
			t.Errorf("%q should NOT be terminal", s)
		}
	}
}

// scriptedExecReader returns a sequence of executions across successive
// GetExecution reads, then repeats the last one. It implements execReader so
// streamLoop can be exercised without a real store.
type scriptedExecReader struct {
	mu    sync.Mutex
	seq   []store.Execution
	err   error
	reads int
}

func (s *scriptedExecReader) GetExecution(_ context.Context, _ string) (store.Execution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return store.Execution{}, s.err
	}
	i := s.reads
	if i >= len(s.seq) {
		i = len(s.seq) - 1
	}
	s.reads++
	return s.seq[i], nil
}

// TestStreamLoop_EmitsThenStopsOnTerminal drives the loop with running→success
// and asserts the stream carries both frames and closes on the terminal event.
func TestStreamLoop_EmitsThenStopsOnTerminal(t *testing.T) {
	reader := &scriptedExecReader{seq: []store.Execution{
		{ID: "exe_x", Status: "running", FlowID: "flw_1", Steps: []store.ExecutionStep{}},
		{ID: "exe_x", Status: "success", FlowID: "flw_1", Steps: []store.ExecutionStep{}},
	}}
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		// Tiny interval keeps the test fast; generous max so terminal-stop (not the
		// deadline) is what ends the loop.
		streamLoop(context.Background(), w, reader, "exe_x", time.Millisecond, time.Second)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("streamLoop did not stop on terminal state")
	}

	body := w.Body.String()
	// Two data frames: the initial running read, then the terminal success read.
	if strings.Count(body, "data: ") != 2 {
		t.Fatalf("want 2 data frames, body =\n%s", body)
	}
	if !strings.Contains(body, `"status":"running"`) {
		t.Errorf("stream missing the running event:\n%s", body)
	}
	if !strings.Contains(body, `"status":"success"`) {
		t.Errorf("stream missing the terminal success event:\n%s", body)
	}
	// The loop stopped after exactly the terminal read (2 reads: running, success).
	if reader.reads != 2 {
		t.Errorf("GetExecution reads = %d, want 2 (stopped on terminal)", reader.reads)
	}
}

// A client that connects to an already-terminal run gets exactly one event and
// the loop returns immediately (the initial emit is terminal).
func TestStreamLoop_AlreadyTerminal_EmitsOnceAndStops(t *testing.T) {
	reader := &scriptedExecReader{seq: []store.Execution{
		{ID: "exe_x", Status: "failed", Steps: []store.ExecutionStep{}},
	}}
	w := httptest.NewRecorder()
	streamLoop(context.Background(), w, reader, "exe_x", time.Millisecond, time.Second)

	if n := strings.Count(w.Body.String(), "data: "); n != 1 {
		t.Fatalf("want exactly 1 data frame for an already-terminal run, got %d", n)
	}
	if reader.reads != 1 {
		t.Errorf("reads = %d, want 1", reader.reads)
	}
}

// A store read error is emitted as an SSE error event and stops the stream.
func TestStreamLoop_ReadError_EmitsErrorAndStops(t *testing.T) {
	reader := &scriptedExecReader{err: errors.New("execution not found: exe_x")}
	w := httptest.NewRecorder()
	streamLoop(context.Background(), w, reader, "exe_x", time.Millisecond, time.Second)

	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Fatalf("want an SSE error frame, body =\n%s", body)
	}
	if !strings.Contains(body, "execution not found") {
		t.Errorf("error frame should carry the message:\n%s", body)
	}
}

// The loop returns when the client disconnects (ctx cancelled) even if the run
// never reaches a terminal state.
func TestStreamLoop_ClientDisconnect_Stops(t *testing.T) {
	reader := &scriptedExecReader{seq: []store.Execution{
		{ID: "exe_x", Status: "running", Steps: []store.ExecutionStep{}},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		streamLoop(ctx, w, reader, "exe_x", 50*time.Millisecond, time.Minute)
		close(done)
	}()

	// Let the initial (running) frame emit, then disconnect.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("streamLoop did not stop on client disconnect")
	}
}

// The loop returns when the max duration elapses on a still-running execution.
func TestStreamLoop_MaxDuration_Stops(t *testing.T) {
	reader := &scriptedExecReader{seq: []store.Execution{
		{ID: "exe_x", Status: "running", Steps: []store.ExecutionStep{}},
	}}
	w := httptest.NewRecorder()

	start := time.Now()
	// interval longer than maxDuration → the deadline is what ends the loop.
	streamLoop(context.Background(), w, reader, "exe_x", time.Second, 30*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("loop should have stopped at ~maxDuration, took %v", elapsed)
	}
}

// End-to-end through the router: the SSE handler sets the event-stream content
// type and streams the terminal frame for a running→success execution.
func TestStreamExecution_Handler_SetsHeadersAndStreams(t *testing.T) {
	// Shrink the poll interval for the duration of the test so the handler's
	// terminal read happens quickly.
	old := streamPollInterval
	streamPollInterval = time.Millisecond
	defer func() { streamPollInterval = old }()

	fake := seedFake()
	// exe_1003 is seeded "running"; flip it to success on the next read by using a
	// store wrapper that returns running once then success.
	r := newTestRouter(&flipToSuccessStore{fakeStore: fake, id: "exe_1003"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, APIBasePath+"/executions/exe_1003/stream", nil)
	r.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	if !strings.Contains(w.Body.String(), `"status":"success"`) {
		t.Fatalf("stream should end with the terminal success event:\n%s", w.Body.String())
	}
}

// flipToSuccessStore wraps a fakeStore and makes GetExecution(id) return
// "running" on the first call and "success" thereafter, so the handler's loop
// terminates deterministically.
type flipToSuccessStore struct {
	*fakeStore
	id    string
	calls int
}

func (s *flipToSuccessStore) GetExecution(ctx context.Context, id string) (store.Execution, error) {
	exec, err := s.fakeStore.GetExecution(ctx, id)
	if err != nil || id != s.id {
		return exec, err
	}
	s.calls++
	if s.calls == 1 {
		exec.Status = "running"
	} else {
		exec.Status = "success"
	}
	return exec, nil
}

// writeSSEData swallows a marshal failure (nothing sensible to emit) — an
// unmarshalable value (a channel) produces no frame.
func TestWriteSSEData_MarshalError_NoFrame(t *testing.T) {
	w := httptest.NewRecorder()
	writeSSEData(w, make(chan int))
	if w.Body.Len() != 0 {
		t.Fatalf("marshal error should emit nothing, got %q", w.Body.String())
	}
}

// The stream route is gated on RunView — every baseline role has it; an unknown
// role is denied.
func TestStreamExecution_RBAC(t *testing.T) {
	old := streamPollInterval
	streamPollInterval = time.Millisecond
	defer func() { streamPollInterval = old }()

	cases := map[string]int{
		"admin":    http.StatusOK,
		"viewer":   http.StatusOK,
		"operator": http.StatusOK,
		"auditor":  http.StatusForbidden,
	}
	for role, want := range cases {
		t.Run(role, func(t *testing.T) {
			fake := seedFake()
			// Seed a terminal execution so the stream returns promptly for allowed roles.
			fake.execs = append(fake.execs, store.Execution{ID: "exe_done", Status: "success", Steps: []store.ExecutionStep{}})
			r := newTestRouter(fake)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, APIBasePath+"/executions/exe_done/stream", nil)
			req.Header.Set(roleHeader, role)
			r.ServeHTTP(w, req)
			if w.Code != want {
				t.Fatalf("stream as %s = %d, want %d", role, w.Code, want)
			}
		})
	}
}
