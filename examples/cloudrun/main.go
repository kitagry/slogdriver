package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"contrib.go.opencensus.io/exporter/stackdriver"
	"github.com/kitagry/slogdriver"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/trace"
)

var logger = slogdriver.New(os.Stdout, slogdriver.HandlerOptions{
	AddSource: true,
	ProjectID: os.Getenv("GOOGLE_CLOUD_PROJECT"),
})

func init() {
	initTrace()
}

func main() {
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
	logger = logger.WithContext(r.Context())
	logger.Info("handle", slogdriver.MakeHTTPAttr(r, r.Response))
	logger.Warn("hoge")
	logger.Error("fuga", fmt.Errorf("error"))
	_, _ = w.Write([]byte("OK"))
}

// Cloud trace setting
func initTrace() {
	exporter, err := stackdriver.NewExporter(stackdriver.Options{
		ProjectID:                os.Getenv("GOOGLE_CLOUD_PROJECT"),
		TraceSpansBufferMaxBytes: 32 * 1024 * 1024,
	})
	if err != nil {
		log.Fatal(err)
	}
	trace.RegisterExporter(exporter)
}

func withTrace(h http.Handler) http.Handler {
	return &ochttp.Handler{Handler: h}
}
