package slogdriver

import (
	"context"
	"fmt"
	"log/slog"

	opencensusTrace "go.opencensus.io/trace"
	opentelemetryTrace "go.opentelemetry.io/otel/trace"
)

func (c *cloudLoggingHandler) handleTrace(ctx context.Context, r *slog.Record) {
	if c.opts.ProjectID == "" {
		return
	}

	if ctx == nil {
		return
	}

	c.handleOpencensusTrace(ctx, r)
	c.handleOpentelemetryTrace(ctx, r)
}

func (c *cloudLoggingHandler) handleOpencensusTrace(ctx context.Context, r *slog.Record) {
	span := opencensusTrace.FromContext(ctx)
	if span == nil {
		return
	}

	spanCtx := span.SpanContext()
	r.AddAttrs(
		slog.String(TraceKey, fmt.Sprintf("projects/%s/traces/%s", c.opts.ProjectID, spanCtx.TraceID.String())),
		slog.String(SpanIDKey, spanCtx.SpanID.String()),
		slog.Bool(TraceSampledKey, spanCtx.IsSampled()),
	)
}

func (c *cloudLoggingHandler) handleOpentelemetryTrace(ctx context.Context, r *slog.Record) {
	spanCtx := opentelemetryTrace.SpanContextFromContext(ctx)

	if !spanCtx.HasTraceID() || !spanCtx.HasSpanID() {
		return
	}
	r.AddAttrs(
		slog.String(TraceKey, fmt.Sprintf("projects/%s/traces/%s", c.opts.ProjectID, spanCtx.TraceID().String())),
		slog.String(SpanIDKey, spanCtx.SpanID().String()),
		slog.Bool(TraceSampledKey, spanCtx.IsSampled()),
	)
}
