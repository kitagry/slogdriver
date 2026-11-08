package slogdriver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/kitagry/slogdriver"
)

func TestSeverity(t *testing.T) {
	tests := map[string]struct {
		severity       slog.Level
		expectSeverity string
	}{
		"DEFAULT": {
			severity:       slogdriver.LevelDefault,
			expectSeverity: "DEFAULT",
		},
		"DEBUG": {
			severity:       slogdriver.LevelDebug,
			expectSeverity: "DEBUG",
		},
		"INFO": {
			severity:       slogdriver.LevelInfo,
			expectSeverity: "INFO",
		},
		"NOTICE": {
			severity:       slogdriver.LevelNotice,
			expectSeverity: "NOTICE",
		},
		"WARNING": {
			severity:       slogdriver.LevelWarning,
			expectSeverity: "WARNING",
		},
		"ERROR": {
			severity:       slogdriver.LevelError,
			expectSeverity: "ERROR",
		},
		"CRITICAL": {
			severity:       slogdriver.LevelCritical,
			expectSeverity: "CRITICAL",
		},
		"ALERT": {
			severity:       slogdriver.LevelAlert,
			expectSeverity: "ALERT",
		},
		"EMERGENCY": {
			severity:       slogdriver.LevelEmergency,
			expectSeverity: "EMERGENCY",
		},
	}

	for n, tt := range tests {
		t.Run(n, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
				Level: slogdriver.LevelDefault,
			})

			logger.Log(context.Background(), tt.severity, "msg")

			var got map[string]any
			err := json.NewDecoder(&buf).Decode(&got)
			if err != nil {
				t.Fatalf("failed to decode json: %+v", err)
			}

			if tt.expectSeverity != got[slogdriver.SeverityKey] {
				t.Errorf("Severity expected %s, got %s", tt.expectSeverity, got[slogdriver.SeverityKey])
			}
		})
	}
}

func TestSeverity_IntermediateLevelShouldUseNearestLowerSeverity(t *testing.T) {
	tests := map[string]struct {
		level          slog.Level
		expectSeverity string
	}{
		"below DEFAULT": {level: slogdriver.LevelDefault - 1, expectSeverity: "DEFAULT"},
		"DEBUG-1":       {level: slogdriver.LevelDebug - 1, expectSeverity: "DEFAULT"},
		"INFO+1":        {level: slogdriver.LevelInfo + 1, expectSeverity: "INFO"},
		"NOTICE+1":      {level: slogdriver.LevelNotice + 1, expectSeverity: "NOTICE"},
		"ERROR+1":       {level: slogdriver.LevelError + 1, expectSeverity: "ERROR"},
		"EMERGENCY+1":   {level: slogdriver.LevelEmergency + 1, expectSeverity: "EMERGENCY"},
	}

	for n, tt := range tests {
		t.Run(n, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
				Level: slogdriver.LevelDefault - 1,
			})

			logger.Log(t.Context(), tt.level, "msg")

			var got map[string]any
			if err := json.NewDecoder(&buf).Decode(&got); err != nil {
				t.Fatalf("failed to decode json: %+v", err)
			}

			if tt.expectSeverity != got[slogdriver.SeverityKey] {
				t.Errorf("Severity expected %s, got %s", tt.expectSeverity, got[slogdriver.SeverityKey])
			}
		})
	}
}

func TestSeverity_UserLevelAttrShouldNotOverrideSeverity(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{})

	logger.Error("msg", slog.Int("level", 3), slog.Group("g", slog.String("level", "high")))

	out := buf.Bytes()
	if n := bytes.Count(out, []byte(`"`+slogdriver.SeverityKey+`":`)); n != 1 {
		t.Errorf("severity should appear once, got %d: %s", n, out)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got[slogdriver.SeverityKey] != "ERROR" {
		t.Errorf("Severity expected ERROR, got %v", got[slogdriver.SeverityKey])
	}
	if g, _ := got["g"].(map[string]any); g["level"] != "high" {
		t.Errorf("nested level attr should be kept, got %v", got["g"])
	}
}
