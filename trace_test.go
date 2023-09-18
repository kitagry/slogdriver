package slogdriver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/kitagry/slogdriver"
	opencensusTrace "go.opencensus.io/trace"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
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
