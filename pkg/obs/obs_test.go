package obs

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// newTestTracer installs an in-memory-exporter-backed provider and returns a
// tracer plus the exporter so tests can inspect finished spans. The provider is
// flushed and shut down on cleanup.
func newTestTracer(t *testing.T) (trace.Tracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return tp.Tracer(tracerName), exp
}

// attrsOf flattens a finished span's attributes into a map for assertions.
func attrsOf(kvs []attribute.KeyValue) map[string]string {
	m := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		m[string(kv.Key)] = kv.Value.String()
	}
	return m
}

func TestStartRunSpan(t *testing.T) {
	tracer, exp := newTestTracer(t)

	ctx, span := StartRunSpan(context.Background(), tracer, "run-42")
	if span == nil {
		t.Fatal("expected non-nil run span")
	}
	if !span.SpanContext().IsValid() {
		t.Fatal("expected a valid (recording) span context")
	}
	if ctx == nil {
		t.Fatal("expected a non-nil returned context")
	}
	span.End()

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	got := spans[0]
	if got.Name != SpanRun {
		t.Errorf("span name = %q, want %q", got.Name, SpanRun)
	}
	if a := attrsOf(got.Attributes); a[AttrRunID] != "run-42" {
		t.Errorf("%s = %q, want %q", AttrRunID, a[AttrRunID], "run-42")
	}
}

func TestStartNodeSpan(t *testing.T) {
	tracer, exp := newTestTracer(t)

	ctx, node := StartNodeSpan(context.Background(), tracer, "node-7", "db.query")
	if node == nil {
		t.Fatal("expected non-nil node span")
	}
	if ctx == nil {
		t.Fatal("expected a non-nil returned context")
	}
	node.End()

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	got := spans[0]
	if got.Name != SpanNode {
		t.Errorf("span name = %q, want %q", got.Name, SpanNode)
	}
	a := attrsOf(got.Attributes)
	if a[AttrNodeID] != "node-7" {
		t.Errorf("%s = %q, want %q", AttrNodeID, a[AttrNodeID], "node-7")
	}
	if a[AttrNodeType] != "db.query" {
		t.Errorf("%s = %q, want %q", AttrNodeType, a[AttrNodeType], "db.query")
	}
}

func TestNodeSpanNestsUnderRun(t *testing.T) {
	tracer, exp := newTestTracer(t)

	ctx, run := StartRunSpan(context.Background(), tracer, "run-1")
	_, node := StartNodeSpan(ctx, tracer, "n1", "http")
	node.End()
	run.End()

	spans := exp.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Locate each finished span by name.
	byName := map[string]tracetest.SpanStub{}
	for _, s := range spans {
		byName[s.Name] = s
	}
	runStub, nodeStub := byName[SpanRun], byName[SpanNode]

	if runStub.SpanContext.TraceID() != nodeStub.SpanContext.TraceID() {
		t.Fatal("node span is not in the same trace as the run (1 run != 1 trace)")
	}
	if nodeStub.Parent.SpanID() != runStub.SpanContext.SpanID() {
		t.Fatal("node span is not a child of the run span")
	}
}

func TestTracer_ReturnsFromGlobalProvider(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		_ = tp.Shutdown(context.Background())
	})

	_, span := StartRunSpan(context.Background(), Tracer(), "run-global")
	span.End()

	if n := len(exp.GetSpans()); n != 1 {
		t.Fatalf("expected Tracer() to use the global provider (1 span), got %d", n)
	}
}

func TestInit_SmokeAndShutdown(t *testing.T) {
	prev := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	shutdown, err := Init(context.Background(), "automate-test")
	if err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown function")
	}

	// The global provider should now produce working spans.
	_, span := StartRunSpan(context.Background(), Tracer(), "run-init")
	span.End()

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}
}

func TestInit_ExporterError(t *testing.T) {
	prev := otel.GetTracerProvider()
	orig := newExporter
	t.Cleanup(func() {
		newExporter = orig
		otel.SetTracerProvider(prev)
	})

	wantErr := errors.New("exporter boom")
	newExporter = func() (sdktrace.SpanExporter, error) {
		return nil, wantErr
	}

	shutdown, err := Init(context.Background(), "automate-test")
	if err == nil {
		t.Fatal("expected error when exporter build fails")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped %v, got %v", wantErr, err)
	}
	if shutdown != nil {
		t.Fatal("expected nil shutdown on error")
	}
}

func TestInit_ResourceError(t *testing.T) {
	prev := otel.GetTracerProvider()
	orig := newResource
	t.Cleanup(func() {
		newResource = orig
		otel.SetTracerProvider(prev)
	})

	wantErr := errors.New("boom")
	newResource = func(context.Context, string) (*resource.Resource, error) {
		return nil, wantErr
	}

	shutdown, err := Init(context.Background(), "automate-test")
	if err == nil {
		t.Fatal("expected error when resource build fails")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped %v, got %v", wantErr, err)
	}
	if shutdown != nil {
		t.Fatal("expected nil shutdown on error")
	}
}

func TestNewResource_HasServiceName(t *testing.T) {
	res, err := newResource(context.Background(), "svc-name-here")
	if err != nil {
		t.Fatalf("newResource error: %v", err)
	}
	found := ""
	for _, kv := range res.Attributes() {
		if string(kv.Key) == "service.name" {
			found = kv.Value.String()
		}
	}
	if found != "svc-name-here" {
		t.Fatalf("service.name = %q, want %q", found, "svc-name-here")
	}
}
