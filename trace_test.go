package slogdriver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/kitagry/slogdriver"
	"go.opencensus.io/trace"
)

func TestCloudLoggingHandler_HandleTraceShouldHaveTraceKeys(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")

	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{})

	ctx, span := trace.StartSpan(context.Background(), "test-span")
	defer span.End()

	logger.InfoCtx(ctx, "Hello World")

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
