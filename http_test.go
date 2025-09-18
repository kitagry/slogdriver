package slogdriver_test

import (
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/kitagry/slogdriver"
)

func TestMakeHTTPAttr(t *testing.T) {
	req, err := http.NewRequest("GET", "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp := &http.Response{
		StatusCode:    http.StatusOK,
		ContentLength: 100,
		Body:          io.NopCloser(nil),
	}

	got := slogdriver.MakeHTTPAttr(req, resp)
	expected := slog.Any(slogdriver.HTTPKey, slogdriver.HTTPPayload{
		RequestMethod: "GET",
		RequestURL:    "https://example.com",
		Status:        http.StatusOK,
		ResponseSize:  "100",
		Protocol:      "HTTP/1.1",
	})

	if !got.Equal(expected) {
		t.Errorf("MakeHTTPAttr expected %+v, got %+v", expected, got)
	}
}

func TestMakeLatencyForGKE(t *testing.T) {
	tests := []struct {
		d             time.Duration
		expectLatency string
	}{
		{
			d:             time.Second,
			expectLatency: "1s",
		},
		{
			d:             time.Millisecond,
			expectLatency: "0.001s",
		},
		{
			d:             time.Nanosecond,
			expectLatency: "0.000000001s",
		},
		{
			d:             time.Minute,
			expectLatency: "60s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.d.String(), func(t *testing.T) {
			latency := slogdriver.MakeLatency(tt.d, true)

			got, ok := latency.(string)
			if !ok {
				t.Fatalf("latency should be string, got %T", latency)
			}

			if tt.expectLatency != got {
				t.Errorf("expected %s, got %s", tt.expectLatency, got)
			}
		})
	}
}
