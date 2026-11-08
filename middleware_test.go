package slogdriver_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kitagry/slogdriver"
	opencensusTrace "go.opencensus.io/trace"
	opentelemetryTrace "go.opentelemetry.io/otel/trace"
)

func mustRemoteSpanContext(t *testing.T, traceID, spanID string, sampled bool) opentelemetryTrace.SpanContext {
	t.Helper()
	tid, err := opentelemetryTrace.TraceIDFromHex(traceID)
	if err != nil {
		t.Fatal(err)
	}
	sid, err := opentelemetryTrace.SpanIDFromHex(spanID)
	if err != nil {
		t.Fatal(err)
	}
	var flags opentelemetryTrace.TraceFlags
	if sampled {
		flags = opentelemetryTrace.FlagsSampled
	}
	return opentelemetryTrace.NewSpanContext(opentelemetryTrace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: flags,
		Remote:     true,
	})
}

// serveWithMiddleware returns the span context that the next handler of the middleware sees.
func serveWithMiddleware(t *testing.T, req *http.Request) opentelemetryTrace.SpanContext {
	t.Helper()
	var got opentelemetryTrace.SpanContext
	h := slogdriver.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = opentelemetryTrace.SpanContextFromContext(r.Context())
	}))
	h.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestMiddleware_ParseTraceHeader(t *testing.T) {
	const (
		traceID = "0af7651916cd43dd8448eb211c80319c"
		spanID  = "b7ad6b7169203331"
	)

	tests := map[string]struct {
		traceparent        string
		xCloudTraceContext string
		// want is nil when no span context should be set.
		want func(t *testing.T) opentelemetryTrace.SpanContext
	}{
		"traceparent sampled": {
			traceparent: "00-" + traceID + "-" + spanID + "-01",
			want: func(t *testing.T) opentelemetryTrace.SpanContext {
				return mustRemoteSpanContext(t, traceID, spanID, true)
			},
		},
		"traceparent not sampled": {
			traceparent: "00-" + traceID + "-" + spanID + "-00",
			want: func(t *testing.T) opentelemetryTrace.SpanContext {
				return mustRemoteSpanContext(t, traceID, spanID, false)
			},
		},
		"traceparent with a future version and extra fields": {
			traceparent: "01-" + traceID + "-" + spanID + "-01-extra",
			want: func(t *testing.T) opentelemetryTrace.SpanContext {
				return mustRemoteSpanContext(t, traceID, spanID, true)
			},
		},
		"traceparent version ff is invalid": {
			traceparent: "ff-" + traceID + "-" + spanID + "-01",
		},
		"traceparent version 00 with extra fields is invalid": {
			traceparent: "00-" + traceID + "-" + spanID + "-01-extra",
		},
		"traceparent with short trace id is invalid": {
			traceparent: "00-" + traceID[1:] + "-" + spanID + "-01",
		},
		"traceparent with short span id is invalid": {
			traceparent: "00-" + traceID + "-" + spanID[1:] + "-01",
		},
		"traceparent with all zero trace id is invalid": {
			traceparent: "00-00000000000000000000000000000000-" + spanID + "-01",
		},
		"traceparent with all zero span id is invalid": {
			traceparent: "00-" + traceID + "-0000000000000000-01",
		},
		"traceparent with uppercase hex is invalid": {
			traceparent: "00-0AF7651916CD43DD8448EB211C80319C-" + spanID + "-01",
		},
		"traceparent with invalid flags is invalid": {
			traceparent: "00-" + traceID + "-" + spanID + "-zz",
		},
		"traceparent without flags is invalid": {
			traceparent: "00-" + traceID + "-" + spanID,
		},
		"x-cloud-trace-context sampled": {
			xCloudTraceContext: traceID + "/1;o=1",
			want: func(t *testing.T) opentelemetryTrace.SpanContext {
				return mustRemoteSpanContext(t, traceID, "0000000000000001", true)
			},
		},
		"x-cloud-trace-context with decimal span id": {
			xCloudTraceContext: traceID + "/12345678901234567890;o=0",
			want: func(t *testing.T) opentelemetryTrace.SpanContext {
				return mustRemoteSpanContext(t, traceID, "ab54a98ceb1f0ad2", false)
			},
		},
		"x-cloud-trace-context without options is not sampled": {
			xCloudTraceContext: traceID + "/1",
			want: func(t *testing.T) opentelemetryTrace.SpanContext {
				return mustRemoteSpanContext(t, traceID, "0000000000000001", false)
			},
		},
		"x-cloud-trace-context without span id is invalid": {
			xCloudTraceContext: traceID,
		},
		"x-cloud-trace-context with zero span id is invalid": {
			xCloudTraceContext: traceID + "/0;o=1",
		},
		"x-cloud-trace-context with overflowing span id is invalid": {
			xCloudTraceContext: traceID + "/18446744073709551616;o=1",
		},
		"x-cloud-trace-context with hex span id is invalid": {
			xCloudTraceContext: traceID + "/" + spanID + ";o=1",
		},
		"x-cloud-trace-context with short trace id is invalid": {
			xCloudTraceContext: traceID[1:] + "/1;o=1",
		},
		"x-cloud-trace-context with all zero trace id is invalid": {
			xCloudTraceContext: "00000000000000000000000000000000/1;o=1",
		},
		"traceparent is preferred to x-cloud-trace-context": {
			traceparent:        "00-" + traceID + "-" + spanID + "-01",
			xCloudTraceContext: "105445aa7843bc8bf206b12000100000/1;o=0",
			want: func(t *testing.T) opentelemetryTrace.SpanContext {
				return mustRemoteSpanContext(t, traceID, spanID, true)
			},
		},
		"x-cloud-trace-context is used when traceparent is invalid": {
			traceparent:        "00-invalid",
			xCloudTraceContext: traceID + "/1;o=1",
			want: func(t *testing.T) opentelemetryTrace.SpanContext {
				return mustRemoteSpanContext(t, traceID, "0000000000000001", true)
			},
		},
		"no header": {},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.traceparent != "" {
				req.Header.Set("traceparent", tt.traceparent)
			}
			if tt.xCloudTraceContext != "" {
				req.Header.Set("X-Cloud-Trace-Context", tt.xCloudTraceContext)
			}

			got := serveWithMiddleware(t, req)

			if tt.want == nil {
				if got.IsValid() {
					t.Errorf("span context should not be set, got %+v", got)
				}
				return
			}
			if want := tt.want(t); !got.Equal(want) {
				t.Errorf("span context expected %+v, got %+v", want, got)
			}
		})
	}
}

func TestMiddleware_ShouldKeepExistingSpanInContext(t *testing.T) {
	existing := mustRemoteSpanContext(t, "105445aa7843bc8bf206b12000100000", "0000000000000001", false)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	req = req.WithContext(opentelemetryTrace.ContextWithSpanContext(req.Context(), existing))

	got := serveWithMiddleware(t, req)

	if !got.Equal(existing) {
		t.Errorf("existing span context should be kept, expected %+v, got %+v", existing, got)
	}
}

func TestMiddleware_ShouldKeepExistingOpencensusSpanInContext(t *testing.T) {
	ctx, span := opencensusTrace.StartSpan(t.Context(), "test-span")
	defer span.End()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	req = req.WithContext(ctx)

	got := serveWithMiddleware(t, req)

	if got.IsValid() {
		t.Errorf("span context should not be set when an OpenCensus span exists, got %+v", got)
	}
}

func TestMiddleware_ShouldKeepTracestate(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	req.Header.Set("tracestate", "congo=t61rcWkgMzE")

	got := serveWithMiddleware(t, req)

	if ts := got.TraceState().String(); ts != "congo=t61rcWkgMzE" {
		t.Errorf("tracestate expected %q, got %q", "congo=t61rcWkgMzE", ts)
	}
}

func TestMiddleware_LogShouldHaveTraceFromHeader(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{ProjectID: "test-project"})
	h := slogdriver.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.InfoContext(r.Context(), "Hello World")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Cloud-Trace-Context", "0af7651916cd43dd8448eb211c80319c/12345678901234567890;o=1")

	h.ServeHTTP(httptest.NewRecorder(), req)

	assertTraceKeysOnce(t, buf.Bytes(), map[string]any{
		slogdriver.TraceKey:        "projects/test-project/traces/0af7651916cd43dd8448eb211c80319c",
		slogdriver.SpanIDKey:       "ab54a98ceb1f0ad2",
		slogdriver.TraceSampledKey: true,
	})
}
