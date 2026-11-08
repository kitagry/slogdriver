package slogdriver_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestMakeHTTPPayload_RemoteIP(t *testing.T) {
	tests := map[string]struct {
		xff        string
		remoteAddr string
		want       string
	}{
		"single X-Forwarded-For": {
			xff:        "1.1.1.1",
			remoteAddr: "10.0.0.1:1234",
			want:       "1.1.1.1",
		},
		"multiple X-Forwarded-For uses the client IP": {
			xff:        " 1.1.1.1 , 2.2.2.2, 3.3.3.3",
			remoteAddr: "10.0.0.1:1234",
			want:       "1.1.1.1",
		},
		"empty first X-Forwarded-For falls back to RemoteAddr": {
			xff:        " , 2.2.2.2",
			remoteAddr: "10.0.0.1:1234",
			want:       "10.0.0.1",
		},
		"no X-Forwarded-For uses RemoteAddr": {
			remoteAddr: "10.0.0.1:1234",
			want:       "10.0.0.1",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}

			got := slogdriver.MakeHTTPPayload(req, nil).RemoteIP

			if got != tt.want {
				t.Errorf("RemoteIP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMakeHTTPPayload_UnknownContentLength(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://example.com", strings.NewReader("body"))
	req.ContentLength = -1
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		ContentLength: -1,
		Body:          io.NopCloser(nil),
	}

	got := slogdriver.MakeHTTPPayload(req, resp)

	if got.RequestSize != "" {
		t.Errorf("RequestSize = %q, want empty", got.RequestSize)
	}
	if got.ResponseSize != "" {
		t.Errorf("ResponseSize = %q, want empty", got.ResponseSize)
	}
}

func TestHTTPPayload_MarshalJSONOmitsZeroValues(t *testing.T) {
	p := slogdriver.HTTPPayload{
		RequestMethod: http.MethodGet,
		RequestURL:    "/test",
	}

	got, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}

	want := `{"requestMethod":"GET","requestUrl":"/test"}`
	if string(got) != want {
		t.Errorf("json.Marshal(HTTPPayload) = %s, want %s", got, want)
	}
}
