package slogdriver

import (
	"context"
	"encoding/binary"
	"net/http"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/propagation"
	opentelemetryTrace "go.opentelemetry.io/otel/trace"
)

// Middleware puts the trace context of the request headers into the request context,
// so that logs written with the context have trace, spanId and trace_sampled.
// It reads the traceparent header (W3C Trace Context) first and falls back to the X-Cloud-Trace-Context header.
// Invalid headers are ignored.
//
// The trace context is stored as an OpenTelemetry remote span context.
// If the request context already has a span (e.g. by otelhttp), it is kept as is.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if !opentelemetryTrace.SpanContextFromContext(ctx).IsValid() {
			if spanCtx, ok := spanContextFromHeader(r.Header); ok {
				r = r.WithContext(opentelemetryTrace.ContextWithRemoteSpanContext(ctx, spanCtx))
			}
		}
		next.ServeHTTP(w, r)
	})
}

func spanContextFromHeader(h http.Header) (opentelemetryTrace.SpanContext, bool) {
	// propagation.TraceContext validates traceparent as the W3C spec requires and also keeps tracestate.
	ctx := propagation.TraceContext{}.Extract(context.Background(), propagation.HeaderCarrier(h))
	if spanCtx := opentelemetryTrace.SpanContextFromContext(ctx); spanCtx.IsValid() {
		return spanCtx, true
	}
	return parseXCloudTraceContext(h.Get("X-Cloud-Trace-Context"))
}

// parseXCloudTraceContext parses "TRACE_ID/SPAN_ID;o=OPTIONS". SPAN_ID is a decimal number.
// See https://cloud.google.com/trace/docs/trace-context#legacy-http-header
func parseXCloudTraceContext(v string) (opentelemetryTrace.SpanContext, bool) {
	traceIDHex, rest, ok := strings.Cut(v, "/")
	if !ok {
		return opentelemetryTrace.SpanContext{}, false
	}
	traceID, err := opentelemetryTrace.TraceIDFromHex(traceIDHex)
	if err != nil {
		return opentelemetryTrace.SpanContext{}, false
	}
	spanIDDec, options, _ := strings.Cut(rest, ";")
	n, err := strconv.ParseUint(spanIDDec, 10, 64)
	if err != nil {
		return opentelemetryTrace.SpanContext{}, false
	}
	var spanID opentelemetryTrace.SpanID
	binary.BigEndian.PutUint64(spanID[:], n)

	var flags opentelemetryTrace.TraceFlags
	if options == "o=1" {
		flags = opentelemetryTrace.FlagsSampled
	}
	spanCtx := opentelemetryTrace.NewSpanContext(opentelemetryTrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: flags,
		Remote:     true,
	})
	return spanCtx, spanCtx.IsValid()
}
