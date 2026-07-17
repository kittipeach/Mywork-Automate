// Package obs wires MyWork Automate's OpenTelemetry tracing (story E11-S1,
// docs/spec/08-backlog "OpenTelemetry → App Insights"): one workflow run is one
// trace and each node execution is a span. It exposes a thin composition-root
// entry point (Init) plus span helpers the worker uses to bracket runs and
// nodes.
//
// Exporter selection is intentionally simple: spans are written to stdout via
// the stdouttrace exporter by default, which a sidecar/collector ships onward
// to Application Insights. If OTEL_EXPORTER_OTLP_ENDPOINT is set the caller's
// environment is expected to carry an OTLP collector; the default stdout path
// keeps this package dependency-light and always testable.
package obs

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	// tracerName is the instrumentation-scope name used for every tracer this
	// package hands out, so spans are attributable to automate's own code.
	tracerName = "github.com/mywork/automate/pkg/obs"

	// SpanRun is the span name for a whole workflow run (the trace root).
	SpanRun = "flow.run"
	// SpanNode is the span name for a single node execution.
	SpanNode = "node.execute"

	// AttrRunID carries the automate run id on the run span; it doubles as the
	// correlation key shared with the ELK/structured-logging pipeline (E11-S3).
	AttrRunID = "automate.run_id"
	// AttrNodeID carries the node identifier on a node span.
	AttrNodeID = "automate.node_id"
	// AttrNodeType carries the node's executor type (e.g. "db.query").
	AttrNodeType = "automate.node_type"
)

// Init builds an OTel TracerProvider with a stdout span exporter and a resource
// describing this service, installs it as the global provider, and returns a
// shutdown function that flushes and stops the provider.
//
// Composition roots call this once at startup and defer the returned shutdown:
//
//	shutdown, err := obs.Init(ctx, "automate-worker")
//	if err != nil { ... }
//	defer shutdown(context.Background())
func Init(ctx context.Context, serviceName string) (shutdown func(context.Context) error, err error) {
	exp, err := newExporter()
	if err != nil {
		return nil, fmt.Errorf("obs: create stdout exporter: %w", err)
	}

	res, err := newResource(ctx, serviceName)
	if err != nil {
		// Roll back the exporter we already created so Init never leaks it.
		_ = exp.Shutdown(ctx)
		return nil, fmt.Errorf("obs: build resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

// newExporter builds the span exporter used by Init. It is a package-level var
// so tests can exercise Init's exporter-error branch deterministically; in
// production it always returns the stdout exporter (which never errors with no
// options — the error branch exists only for interface completeness).
var newExporter = func() (sdktrace.SpanExporter, error) {
	return stdouttrace.New()
}

// newResource builds the OTel resource carrying service.name (plus SDK/host
// defaults merged in). It is a package-level var so tests can exercise the
// merge/error paths deterministically.
var newResource = func(ctx context.Context, serviceName string) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
}

// Tracer returns a named tracer from the globally installed TracerProvider. If
// Init has not run, this yields the OTel no-op tracer, so calling the span
// helpers is always safe.
func Tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

// StartRunSpan starts the root span for a workflow run named SpanRun, tagged
// with the run id (AttrRunID). The returned context carries the span and must
// be passed to StartNodeSpan so node spans nest under the run. The caller owns
// ending the span.
func StartRunSpan(ctx context.Context, tracer trace.Tracer, runID string) (context.Context, trace.Span) {
	return tracer.Start(ctx, SpanRun,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String(AttrRunID, runID),
		),
	)
}

// StartNodeSpan starts a child span named SpanNode for one node execution,
// tagged with the node id and type. ctx should descend from StartRunSpan so the
// node span nests inside the run's trace. The caller owns ending the span.
func StartNodeSpan(ctx context.Context, tracer trace.Tracer, nodeID, nodeType string) (context.Context, trace.Span) {
	return tracer.Start(ctx, SpanNode,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String(AttrNodeID, nodeID),
			attribute.String(AttrNodeType, nodeType),
		),
	)
}
