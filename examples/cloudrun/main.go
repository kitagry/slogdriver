package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	gcppropagator "github.com/GoogleCloudPlatform/opentelemetry-operations-go/propagator"
	"github.com/kitagry/slogdriver"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const traceName = "slogdriver-sample"

var logger = slogdriver.New(os.Stdout, slogdriver.HandlerOptions{
	AddSource: true,
	ProjectID: os.Getenv("GOOGLE_CLOUD_PROJECT"),
})

func main() {
	ctx := context.Background()
	exporter, err := texporter.New(texporter.WithProjectID(os.Getenv("GOOGLE_CLOUD_PROJECT")))
	if err != nil {
		log.Fatal(err)
	}

	res, err := resource.New(ctx, resource.WithDetectors(gcp.NewDetector()), resource.WithTelemetrySDK())
	if err != nil {
		logger.Log(ctx, slogdriver.LevelCritical, err.Error())
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	defer tp.Shutdown(ctx)

	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			gcppropagator.CloudTraceOneWayPropagator{},
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)
	logger.Info("starting server...")

	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		logger.Warn(fmt.Sprintf("defaulting to port %s", port))
	}

	logger.Info(fmt.Sprintf("listening on port %s", port))
	if err := http.ListenAndServe(":"+port, withTrace(withRequestLog(mux))); err != nil {
		logger.Error("server stopped", slog.Any("error", err))
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer(traceName).Start(r.Context(), "log handle")
	logger.InfoContext(ctx, "log handle")
	time.Sleep(100 * time.Millisecond)
	span.End()

	ctx, span = otel.Tracer(traceName).Start(r.Context(), "hoge")
	logger.WarnContext(ctx, "hoge")
	time.Sleep(100 * time.Millisecond)
	span.End()

	ctx, span = otel.Tracer(traceName).Start(r.Context(), "fuga")
	logger.ErrorContext(ctx, "fuga", "error", fmt.Errorf("error msg"))
	time.Sleep(100 * time.Millisecond)
	span.End()

	_, _ = w.Write([]byte("OK"))
}

func withTrace(h http.Handler) http.Handler {
	return otelhttp.NewHandler(h, traceName)
}

// withRequestLog logs the request with httpRequest after h has written the response.
// It must run inside withTrace so that the log has the trace of the request.
func withRequestLog(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w}
		h.ServeHTTP(rec, r)

		// r.Response is always nil on the server side, so the response fields come from rec.
		p := slogdriver.MakeHTTPPayload(r, nil)
		p.Status = rec.statusCode()
		p.ResponseSize = strconv.FormatInt(rec.size, 10)
		p.Latency = slogdriver.MakeLatency(time.Since(start), false) // true on GKE
		logger.InfoContext(r.Context(), "request finished", slogdriver.MakeHTTPAttrFromHTTPPayload(p))
	})
}

// responseRecorder records the status code and the number of bytes written to the response.
type responseRecorder struct {
	http.ResponseWriter
	status int
	size   int64
}

func (rec *responseRecorder) WriteHeader(code int) {
	// 1xx responses are followed by the final response, so they are not recorded.
	if rec.status == 0 && code >= 200 {
		rec.status = code
	}
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *responseRecorder) Write(b []byte) (int, error) {
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	n, err := rec.ResponseWriter.Write(b)
	rec.size += int64(n)
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying ResponseWriter (e.g. for Flush).
func (rec *responseRecorder) Unwrap() http.ResponseWriter {
	return rec.ResponseWriter
}

func (rec *responseRecorder) statusCode() int {
	// net/http sends 200 when the handler writes nothing.
	if rec.status == 0 {
		return http.StatusOK
	}
	return rec.status
}
