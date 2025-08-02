package slogdriver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/kitagry/slogdriver"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func ExampleNew() {
	logger := slogdriver.New(
		os.Stdout,
		slogdriver.HandlerOptions{
			AddSource:     true,
			DefaultLabels: []slog.Attr{slog.String("defaultLabel", "hoge")},
		},
	)
	logger = logger.With(slog.Group(slogdriver.LabelKey, slog.String("commonLabel", "fuga")))
	logger.Info("Hello World", slog.Group(slogdriver.LabelKey, slog.String("specifiedLabel", "piyo")))
}

func TestLabels(t *testing.T) {
	var buf bytes.Buffer

	logger := slogdriver.New(
		&buf,
		slogdriver.HandlerOptions{
			AddSource:     true,
			DefaultLabels: []slog.Attr{slog.String("defaultLabel", "hoge")},
		},
	)
	logger = logger.With(slog.Group(slogdriver.LabelKey, slog.String("commonLabel1", "fuga")))
	logger = logger.With(slog.Group(slogdriver.LabelKey, slog.String("commonLabel2", "piyo")))
	logger.Info("Hello World", slog.Group(slogdriver.LabelKey, slog.String("specifiedLabel", "hogera")))

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	if err != nil {
		t.Fatal(err)
	}

	labels, ok := result[slogdriver.LabelKey].(map[string]any)
	if !ok {
		t.Fatalf("unexpected type: %T", result[slogdriver.LabelKey])
	}

	if labels["defaultLabel"] != "hoge" {
		t.Errorf("unexpected defaultLabel: %s", labels["defaultLabel"])
	}

	if labels["commonLabel1"] != "fuga" {
		t.Errorf("unexpected commonLabel1: %s", labels["commonLabel"])
	}

	if labels["commonLabel2"] != "piyo" {
		t.Errorf("unexpected commonLabel2: %s", labels["commonLabel2"])
	}

	if labels["specifiedLabel"] != "hogera" {
		t.Errorf("unexpected specifiedLabel: %s", labels["specifiedLabel"])
	}
}

func TestShouldSetDefaultLabelsWithoutSpecifiedLabel(t *testing.T) {
	var buf bytes.Buffer

	logger := slogdriver.New(
		&buf,
		slogdriver.HandlerOptions{
			AddSource:     true,
			DefaultLabels: []slog.Attr{slog.String("defaultLabel", "hoge")},
		},
	)
	logger.Info("Hello World")

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	if err != nil {
		t.Fatal(err)
	}

	labels, ok := result[slogdriver.LabelKey].(map[string]any)
	if !ok {
		t.Fatalf("unexpected type: %T", result[slogdriver.LabelKey])
	}

	if labels["defaultLabel"] != "hoge" {
		t.Errorf("unexpected defaultLabel: %s", labels["defaultLabel"])
	}
}

func TestGroup(t *testing.T) {
	var buf bytes.Buffer

	logger := slogdriver.New(
		&buf,
		slogdriver.HandlerOptions{
			ProjectID: "test-project",
			AddSource: true,
		},
	)

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

	logger = logger.WithGroup("group1").WithGroup("group2")
	logger.InfoContext(
		ctx,
		"Hello World",
		slogdriver.MakeHTTPAttrFromHTTPPayload(slogdriver.HTTPPayload{}),
		slog.Group(slogdriver.LabelKey, slog.String("label", "hoge")),
		slog.String("key1", "fuga"),
		slog.Int("key2", 1),
	)

	type entry struct {
		Group1 struct {
			Group2 struct {
				Key1 string `json:"key1"`
				Key2 int    `json:"key2"`
			} `json:"group2"`
		} `json:"group1"`

		HttpRequest  *slogdriver.HTTPPayload            `json:"httpRequest"`
		Source       *slogdriver.LogEntrySourceLocation `json:"logging.googleapis.com/sourceLocation"`
		Labels       map[string]any                     `json:"logging.googleapis.com/labels"`
		Trace        *string                            `json:"logging.googleapis.com/trace"`
		SpanID       *string                            `json:"logging.googleapis.com/spanId"`
		TraceSampled *bool                              `json:"logging.googleapis.com/trace_sampled"`
	}

	var result entry
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}

	if result.Group1.Group2.Key1 != "fuga" {
		t.Errorf("unexpected group1.group2.key1: %s", result.Group1.Group2.Key1)
	}

	if result.Group1.Group2.Key2 != 1 {
		t.Errorf("unexpected group1.group2.key2: %d", result.Group1.Group2.Key2)
	}

	if result.HttpRequest == nil {
		t.Error("http request key not found")
	}

	if result.Source == nil {
		t.Error("source location key not found")
	}

	if result.Labels == nil {
		t.Error("labels key not found")
	}

	if result.Trace == nil {
		t.Error("trace key not found")
	}
	if result.SpanID == nil {
		t.Errorf("span id key not found")
	}
	if result.TraceSampled == nil {
		t.Errorf("trace sampled key not found")
	}
}
