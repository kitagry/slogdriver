package slogdriver_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/kitagry/slogdriver"
	"golang.org/x/exp/slog"
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

			logger.Log(tt.severity, "msg")

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
