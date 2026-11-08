package slogdriver

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	opencensusTrace "go.opencensus.io/trace"
	opentelemetryTrace "go.opentelemetry.io/otel/trace"
)

// handleTrace adds trace attributes from ctx unless any of them is specified manually.
// Filling only the missing keys would mix a manual trace with a spanId of another trace.
func (c *cloudLoggingHandler) handleTrace(ctx context.Context, r *slog.Record, knownAttrs []slog.Attr) {
	if ctx == nil {
		return
	}

	if c.hasManualTraceKey(knownAttrs) {
		return
	}

	r.AddAttrs(c.traceAttrsFromContext(ctx)...)
}

func (c *cloudLoggingHandler) hasManualTraceKey(knownAttrs []slog.Attr) bool {
	for _, key := range []string{TraceKey, SpanIDKey, TraceSampledKey} {
		if _, ok := c.withKnownKeys[key]; ok {
			return true
		}
		if slices.ContainsFunc(knownAttrs, func(a slog.Attr) bool { return a.Key == key }) {
			return true
		}
	}
	return false
}

// traceAttrsFromContext prefers OpenTelemetry to OpenCensus because OpenCensus is deprecated.
func (c *cloudLoggingHandler) traceAttrsFromContext(ctx context.Context) []slog.Attr {
	if spanCtx := opentelemetryTrace.SpanContextFromContext(ctx); spanCtx.IsValid() {
		return c.makeTraceAttrs(spanCtx.TraceID().String(), spanCtx.SpanID().String(), spanCtx.IsSampled())
	}

	if span := opencensusTrace.FromContext(ctx); span != nil {
		spanCtx := span.SpanContext()
		return c.makeTraceAttrs(spanCtx.TraceID.String(), spanCtx.SpanID.String(), spanCtx.IsSampled())
	}

	return nil
}

func (c *cloudLoggingHandler) makeTraceAttrs(traceID, spanID string, sampled bool) []slog.Attr {
	trace := traceID
	// Cloud Logging also accepts a bare trace ID, so trace is still written without the project ID.
	if c.opts.ProjectID != "" {
		trace = fmt.Sprintf("projects/%s/traces/%s", c.opts.ProjectID, traceID)
	}
	return []slog.Attr{
		slog.String(TraceKey, trace),
		slog.String(SpanIDKey, spanID),
		slog.Bool(TraceSampledKey, sampled),
	}
}
