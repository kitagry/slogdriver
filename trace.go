package slogdriver

import (
	"fmt"

	"go.opencensus.io/trace"
	"golang.org/x/exp/slog"
)

func (c *cloudLoggingHandler) handleTrace(r *slog.Record) {
	if c.opts.ProjectID == "" {
		return
	}

	if r.Context == nil {
		return
	}

	span := trace.FromContext(r.Context)
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
