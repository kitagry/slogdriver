package slogdriver_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/kitagry/slogdriver"
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
	logger = logger.With(slog.Group(slogdriver.LabelKey, slog.String("commonLabel", "fuga")))
	logger.Info("Hello World", slog.Group(slogdriver.LabelKey, slog.String("specifiedLabel", "piyo")))

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

	if labels["commonLabel"] != "fuga" {
		t.Errorf("unexpected commonLabel: %s", labels["commonLabel"])
	}

	if labels["specifiedLabel"] != "piyo" {
		t.Errorf("unexpected specifiedLabel: %s", labels["specifiedLabel"])
	}
}
