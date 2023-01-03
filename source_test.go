package slogdriver_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/kitagry/slogdriver"
)

func TestCloudLoggingHandler_HandleSourceLocatin(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{AddSource: true})
	logger.Info("Hello World")

	var got map[string]any
	err := json.NewDecoder(&buf).Decode(&got)
	if err != nil {
		t.Fatalf("failed to decode json: %+v", err)
	}

	sourceLocationAny, ok := got[slogdriver.SourceLocationKey]
	if !ok {
		t.Fatalf("log doesn't have %s key: %+v", slogdriver.SourceLocationKey, got)
	}

	sourceLocation, ok := sourceLocationAny.(map[string]any)
	if !ok {
		t.Fatalf("sourceLocation should map[string]any, but got %T", sourceLocationAny)
	}

	file, ok := sourceLocation["file"]
	if !ok {
		t.Fatalf("sourceLocation should have file attr, but got %v", sourceLocation)
	}

	if _, filename := filepath.Split(file.(string)); filename != "source_test.go" {
		t.Errorf("filepath should be slogdriver_test.go, got %s", filename)
	}
}
