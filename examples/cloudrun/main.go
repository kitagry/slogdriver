package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
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
	if err := http.ListenAndServe(":"+port, withTrace(mux)); err != nil {
		logger.Error("fail", err)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer(traceName).Start(r.Context(), "log handle")
	logger.InfoContext(ctx, "log handle", slogdriver.MakeHTTPAttr(r, r.Response))
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
