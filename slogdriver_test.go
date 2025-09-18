package slogdriver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"os"
	"testing"
	"testing/slogtest"

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

func TestHandler(t *testing.T) {
	var buf bytes.Buffer
	slogtest.Run(
		t,
		func(t *testing.T) slog.Handler {
			buf.Reset()
			return slogdriver.NewHandler(&buf, slogdriver.HandlerOptions{})
		},
		func(t *testing.T) map[string]any {
			line := buf.Bytes()
			if len(line) == 0 {
				return nil
			}

			var m map[string]any
			if err := json.Unmarshal(line, &m); err != nil {
				t.Fatal(err)
			}

			for k := range m {
				switch k {
				case slogdriver.SeverityKey:
					m[slog.LevelKey] = m[k]
					delete(m, k)
				case slogdriver.MessageKey:
					m[slog.MessageKey] = m[k]
					delete(m, k)
				}
			}

			return m
		},
	)
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

	logger = logger.
		With(slog.Bool("commonKey1", true)).
		WithGroup("group1").With(slog.Int("commonKey2", 2)).
		WithGroup("group2").With(slog.String("commonKey3", "piyo"))
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
				Key1       string `json:"key1"`
				Key2       int    `json:"key2"`
				CommonKey3 string `json:"commonKey3"`
			} `json:"group2"`
			CommonKey2 int `json:"commonKey2"`
		} `json:"group1"`
		CommonKey1 bool `json:"commonKey1"`

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

	if !result.CommonKey1 {
		t.Errorf("unexpected commonKey1: %v", result.CommonKey1)
	}

	if result.Group1.CommonKey2 != 2 {
		t.Errorf("unexpected group1.commonKey2: %d", result.Group1.CommonKey2)
	}

	if result.Group1.Group2.CommonKey3 != "piyo" {
		t.Errorf("unexpected group1.group2.commonKey3: %s", result.Group1.Group2.CommonKey3)
	}

	if result.HttpRequest == nil {
		t.Error("http request key not found")
	}

	if result.Source == nil {
		t.Error("source location key not found")
	}

	expectedLabels := map[string]any{"label": "hoge"}
	if !maps.Equal(result.Labels, expectedLabels) {
		t.Errorf("labels should be %v got %v", expectedLabels, result.Labels)
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
