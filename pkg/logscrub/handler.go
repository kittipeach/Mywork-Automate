package logscrub

import (
	"context"
	"log/slog"
)

// Handler is a slog.Handler middleware that scrubs secret-shaped data from log
// records before delegating to an inner handler. Composition roots wrap their
// real handler with this one, e.g.:
//
//	base := slog.NewJSONHandler(os.Stdout, nil)
//	logger := slog.New(logscrub.NewHandler(base))
//
// It scrubs the record message (via Scrub), every string attribute value (via
// Scrub), and the value of any attribute whose key is sensitive (via
// IsSensitiveKey → RedactedToken). Group and nested-attribute structure is
// preserved. Attributes added through WithAttrs are scrubbed once, at
// attachment time.
type Handler struct {
	inner slog.Handler
}

// NewHandler wraps inner so that records are scrubbed before it sees them.
// It panics if inner is nil, since a nil inner handler can never be useful.
func NewHandler(inner slog.Handler) *Handler {
	if inner == nil {
		panic("logscrub: inner handler must not be nil")
	}
	return &Handler{inner: inner}
}

// Enabled delegates to the inner handler.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle scrubs the record's message and attributes, then delegates.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	scrubbed := slog.NewRecord(r.Time, r.Level, Scrub(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		scrubbed.AddAttrs(scrubAttr(a))
		return true
	})
	return h.inner.Handle(ctx, scrubbed)
}

// WithAttrs scrubs the supplied attributes once and attaches them to the inner
// handler, returning a wrapped handler so later records are still scrubbed.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	scrubbed := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		scrubbed[i] = scrubAttr(a)
	}
	return &Handler{inner: h.inner.WithAttrs(scrubbed)}
}

// WithGroup delegates group creation to the inner handler.
func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{inner: h.inner.WithGroup(name)}
}

// scrubAttr returns a copy of a with its value scrubbed. If the attribute key
// is sensitive the whole value is redacted; otherwise string values are passed
// through Scrub and group values are scrubbed member-by-member. Non-string
// scalars are returned unchanged.
func scrubAttr(a slog.Attr) slog.Attr {
	if IsSensitiveKey(a.Key) {
		return slog.String(a.Key, RedactedToken)
	}
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.String(a.Key, Scrub(v.String()))
	case slog.KindGroup:
		members := v.Group()
		scrubbed := make([]slog.Attr, len(members))
		for i, m := range members {
			scrubbed[i] = scrubAttr(m)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(scrubbed...)}
	default:
		return slog.Attr{Key: a.Key, Value: v}
	}
}
