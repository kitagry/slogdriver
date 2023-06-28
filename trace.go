package slogdriver

import (
	"context"
	"fmt"
	"log/slog"

	"go.opencensus.io/trace"
)

func (c *cloudLoggingHandler) handleTrace(ctx context.Context, r *slog.Record) {
	if c.opts.ProjectID == "" {
		return
	}

	if ctx == nil {
		return
	}

	span := trace.FromContext(ctx)
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
