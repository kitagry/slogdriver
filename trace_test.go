package slogdriver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/kitagry/slogdriver"
	opencensusTrace "go.opencensus.io/trace"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	opentelemetryTrace "go.opentelemetry.io/otel/trace"
)

func TestCloudLoggingHandler_HandleTraceShouldHaveOpencensusTraceKeys(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")

	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{})

	ctx, span := opencensusTrace.StartSpan(context.Background(), "test-span")
	defer span.End()

	logger.InfoContext(ctx, "Hello World")

	var got map[string]any
	err := json.NewDecoder(&buf).Decode(&got)
	if err != nil {
		t.Fatalf("failed to decode json: %+v", err)
	}

	_, ok := got[slogdriver.TraceKey]
	if !ok {
		t.Errorf("log should have key=%s, got %v", slogdriver.TraceKey, got)
	}
}

func TestCloudLoggingHandler_HandleTraceShouldHaveOpentelemetryTraceKeys(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")

	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{})

	ctx := context.Background()

	res, err := resource.New(ctx, resource.WithDetectors(gcp.NewDetector()), resource.WithTelemetrySDK())
	if err != nil {
		t.Fatalf("failed to create resource: %+v", err)
	}

	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res))
	defer tp.Shutdown(ctx)
	otel.SetTracerProvider(tp)

	tracer := otel.Tracer("test-tracer")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	logger.InfoContext(ctx, "Hello World")

	var got map[string]any
	err = json.NewDecoder(&buf).Decode(&got)
	if err != nil {
		t.Fatalf("failed to decode json: %+v", err)
	}

	_, ok := got[slogdriver.TraceKey]
	if !ok {
		t.Errorf("log should have key=%s, got %v", slogdriver.TraceKey, got)
	}
}

func TestCloudLoggingHandler_HandleTraceShouldNotHaveTraceKeys(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")

	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{})

	logger.Info("Hello World")

	var got map[string]any
	err := json.NewDecoder(&buf).Decode(&got)
	if err != nil {
		t.Fatalf("failed to decode json: %+v", err)
	}

	_, ok := got[slogdriver.TraceKey]
	if ok {
		t.Errorf("log should not have key=%s, got %v", slogdriver.TraceKey, got)
	}
}

func newOtelContext(t *testing.T) (context.Context, opentelemetryTrace.SpanContext) {
	t.Helper()
	traceID, err := opentelemetryTrace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := opentelemetryTrace.SpanIDFromHex("0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	spanCtx := opentelemetryTrace.NewSpanContext(opentelemetryTrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: opentelemetryTrace.FlagsSampled,
	})
	return opentelemetryTrace.ContextWithSpanContext(t.Context(), spanCtx), spanCtx
}

func assertTraceKeysOnce(t *testing.T, out []byte, want map[string]any) {
	t.Helper()
	var got map[string]any
	for _, key := range []string{slogdriver.TraceKey, slogdriver.SpanIDKey, slogdriver.TraceSampledKey} {
		got = assertKeyOnce(t, out, key)
	}
	for key, v := range want {
		if got[key] != v {
			t.Errorf("key=%s expected %v, got %v", key, v, got[key])
		}
	}
}

func TestCloudLoggingHandler_HandleTraceShouldPreferManualTraceKeysInRecord(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{ProjectID: "test-project"})
	ctx, _ := newOtelContext(t)

	logger.InfoContext(ctx, "Hello World",
		slog.String(slogdriver.TraceKey, "projects/manual/traces/manual"),
		slog.String(slogdriver.SpanIDKey, "manual-span"),
		slog.Bool(slogdriver.TraceSampledKey, false),
	)

	assertTraceKeysOnce(t, buf.Bytes(), map[string]any{
		slogdriver.TraceKey:        "projects/manual/traces/manual",
		slogdriver.SpanIDKey:       "manual-span",
		slogdriver.TraceSampledKey: false,
	})
}

func TestCloudLoggingHandler_HandleTraceShouldPreferManualTraceKeysInWith(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{ProjectID: "test-project"})
	ctx, _ := newOtelContext(t)

	logger.With(
		slog.String(slogdriver.TraceKey, "projects/manual/traces/manual"),
		slog.String(slogdriver.SpanIDKey, "manual-span"),
	).WithGroup("g").With(
		slog.Bool(slogdriver.TraceSampledKey, false),
	).InfoContext(ctx, "Hello World")

	assertTraceKeysOnce(t, buf.Bytes(), map[string]any{
		slogdriver.TraceKey:        "projects/manual/traces/manual",
		slogdriver.SpanIDKey:       "manual-span",
		slogdriver.TraceSampledKey: false,
	})
}

func TestCloudLoggingHandler_HandleTraceShouldNotMixManualAndContextTrace(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{ProjectID: "test-project"})
	ctx, _ := newOtelContext(t)

	logger.InfoContext(ctx, "Hello World", slog.String(slogdriver.TraceKey, "projects/manual/traces/manual"))

	out := buf.Bytes()
	if n := bytes.Count(out, []byte(`"`+slogdriver.TraceKey+`":"projects/manual/traces/manual"`)); n != 1 {
		t.Errorf("manual trace should appear once, got %d: %s", n, out)
	}
	// spanId and trace_sampled from the context belong to another trace.
	for _, key := range []string{slogdriver.SpanIDKey, slogdriver.TraceSampledKey} {
		if bytes.Contains(out, []byte(`"`+key+`":`)) {
			t.Errorf("key=%s should not be added from the context: %s", key, out)
		}
	}
}

func TestCloudLoggingHandler_HandleTraceShouldPreferOpentelemetryOverOpencensus(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{ProjectID: "test-project"})
	ctx, spanCtx := newOtelContext(t)
	ctx, span := opencensusTrace.StartSpan(ctx, "test-span")
	defer span.End()

	logger.InfoContext(ctx, "Hello World")

	assertTraceKeysOnce(t, buf.Bytes(), map[string]any{
		slogdriver.TraceKey:  "projects/test-project/traces/" + spanCtx.TraceID().String(),
		slogdriver.SpanIDKey: spanCtx.SpanID().String(),
	})
}

func TestCloudLoggingHandler_HandleTraceShouldUseTraceIDOnlyWithoutProjectID(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")

	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{})
	ctx, spanCtx := newOtelContext(t)

	logger.InfoContext(ctx, "Hello World")

	assertTraceKeysOnce(t, buf.Bytes(), map[string]any{
		slogdriver.TraceKey:        spanCtx.TraceID().String(),
		slogdriver.SpanIDKey:       spanCtx.SpanID().String(),
		slogdriver.TraceSampledKey: true,
	})
}

func TestCloudLoggingHandler_HandleTraceShouldPreferLogArgumentsOverWith(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{ProjectID: "test-project"})
	ctx, _ := newOtelContext(t)

	logger.With(
		slog.String(slogdriver.TraceKey, "projects/with/traces/with"),
		slog.String(slogdriver.SpanIDKey, "with-span"),
		slog.Bool(slogdriver.TraceSampledKey, true),
	).InfoContext(ctx, "Hello World",
		slog.String(slogdriver.TraceKey, "projects/arg/traces/arg"),
		slog.String(slogdriver.SpanIDKey, "arg-span"),
		slog.Bool(slogdriver.TraceSampledKey, false),
	)

	assertTraceKeysOnce(t, buf.Bytes(), map[string]any{
		slogdriver.TraceKey:        "projects/arg/traces/arg",
		slogdriver.SpanIDKey:       "arg-span",
		slogdriver.TraceSampledKey: false,
	})
}

func TestCloudLoggingHandler_HandleTraceShouldNotMixLogArgumentsAndWithTrace(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{ProjectID: "test-project"})

	logger.With(
		slog.String(slogdriver.TraceKey, "projects/with/traces/with"),
		slog.String(slogdriver.SpanIDKey, "with-span"),
	).Info("Hello World", slog.String(slogdriver.TraceKey, "projects/arg/traces/arg"))

	out := buf.Bytes()
	if n := bytes.Count(out, []byte(`"`+slogdriver.TraceKey+`":"projects/arg/traces/arg"`)); n != 1 {
		t.Errorf("trace in the log arguments should appear once, got %d: %s", n, out)
	}
	// spanId from With belongs to another trace.
	if bytes.Contains(out, []byte(`"`+slogdriver.SpanIDKey+`":`)) {
		t.Errorf("spanId from With should not be written: %s", out)
	}
}
