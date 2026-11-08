# slogdriver

[slog](https://github.com/golang/exp/tree/master/slog) based logging library optimized for [Cloud Logging](https://cloud.google.com/logging). This packaged inspired by [Zapdriver](https://github.com/blendle/zapdriver) and [zerodriver](https://github.com/hirosassa/zerodriver).

## What is this package?

This package provides simple structured logger optimized for [Cloud Logging](https://cloud.google.com/logging) based on [slog](https://github.com/golang/exp/tree/master/slog).

## Requirements

Go 1.26 or above

## Usage

Initialize a logger.

```go
logger := slogdriver.New(os.Stdout, slogdriver.HandlerOptions{})
```

This `logger` is *slog.Logger. Then, write by using slog API.

```go
logger.Info("Hello World!", slog.String("key", "value"))
```

### GCP specific fields

If your log follows [LogEntry](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry) format, you can query logs or create metrics alert easier and efficiently on GCP Cloud Logging console.

#### Severity

You can use slog.Level(Debug, Info, Warn, Error). And this library prepare [all severities for Cloud Logging](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity).

* DEFAULT
* DEBUG
* INFO
* NOTICE
* WARNING
* ERROR
* CRITICAL
* ALERT
* EMERGENCY

```go
logger := slogdriver.New(os.Stdout, slogdriver.HandlerOptions{
  Level: slogdriver.LevelDefault,
})
logger.Log(context.Background(), slogdriver.LevelEmergency, "emergency msg")
```

Other levels are mapped to the nearest severity at or below them (e.g. `slog.LevelInfo+1` is `INFO`, and levels below `LevelDefault` are `DEFAULT`).

#### HTTP request

To log HTTP related metrics and information, you can create slog.Attr with the following function.

```go
var req *http.Request
var res *http.Response
logger.Info("Http Request finished", slogdriver.MakeHTTPAttr(req, res))
```

The following fields needs to be set manually:

- ServerIP
- Latency
- CacheLookup
- CacheHit
- CacheValidatedWithOriginServer
- CacheFillBytes

Using these feature, you can log HTTP related information as follows,

```go
p := slogdriver.MakeHTTPPayload(req, res)
p.Latency = time.Since(start)
logger.Info("http finished", slogdriver.MakeHTTPAttrFromHTTPPayload(p))
// Or, you can create attr manually
logger.Info("http finished", slog.Any(slogdriver.HTTPKey, p))
```

#### Trace context

##### Without OpenTelemetry

`slogdriver.Middleware` reads the `traceparent` or `X-Cloud-Trace-Context` header added by Cloud Run, GKE or Cloud Load Balancing, and puts the trace context into the request context.
Logs written with the request context have trace, spanId and trace_sampled, so they are grouped with the request log in Cloud Logging.

```go
logger := slogdriver.New(os.Stdout, slogdriver.HandlerOptions{ProjectID: "YOUR_PROJECT_ID"})

mux := http.NewServeMux()
mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	logger.InfoContext(r.Context(), "Hello World")
	// got:
	// {"severity":"INFO","message":"Hello World","logging.googleapis.com/trace":"projects/YOUR_PROJECT_ID/traces/0af7651916cd43dd8448eb211c80319c","logging.googleapis.com/spanId":"b7ad6b7169203331","logging.googleapis.com/trace_sampled":true}
})
http.ListenAndServe(":8080", slogdriver.Middleware(mux))
```

- When both headers exist, `traceparent` is used. Invalid headers are ignored. `tracestate` is kept together with `traceparent`.
- The span ID of `X-Cloud-Trace-Context` is a decimal number, so it is converted to 16 hex digits.
- The trace context is stored as an OpenTelemetry remote span context (`trace.ContextWithRemoteSpanContext`), so `trace.SpanContextFromContext(r.Context())` also returns it.
- If the request context already has a valid OpenTelemetry span or an OpenCensus span, the middleware does nothing. So, when you use it with `otelhttp`, the span created by `otelhttp` is used. If you put `slogdriver.Middleware` outside `otelhttp`, the span from the header becomes the parent of the `otelhttp` span (unless the propagator of `otelhttp` extracts another parent).

If neither `ProjectID` nor `GOOGLE_CLOUD_PROJECT` is set, trace is written as the trace ID only (e.g. `"logging.googleapis.com/trace":"0af7651916cd43dd8448eb211c80319c"`), which Cloud Logging also accepts. You can still filter logs by the trace ID, but set `ProjectID` (or `GOOGLE_CLOUD_PROJECT`) to group them with the request log and link them to Cloud Trace, because the request log uses `projects/PROJECT_ID/traces/TRACE_ID`.

##### With OpenTelemetry

```go
import "go.opentelemetry.io/otel"

// Set projectId or, you can set environment GOOGLE_CLOUD_PROJECT
logger := slogdriver.New(os.Stdout, slogdriver.HandlerOptions{ProjectID: "YOUR_PROJECT_ID"})

ctx, span := otel.Tracer(traceName).Start(context.Background(), "span")
defer span.End()

logger.InfoContext(ctx, "Hello World")
// got:
// {"severity":"INFO","message":"Hello World","logging.googleapis.com/trace":"projects/YOUR_PROJECT_ID/traces/00000000000000000000000000000000","logging.googleapis.com/spanId":"0000000000000000","logging.googleapis.com/trace_sampled":true}
```

If you use [go.opentelemetry.io/otel](https://pkg.go.dev/go.opentelemetry.io/otel), it is able to set traceId with request context like the below:

```go
import (
	gcppropagator "github.com/GoogleCloudPlatform/opentelemetry-operations-go/propagator"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
)

otel.SetTextMapPropagator(
    propagation.NewCompositeTextMapPropagator(
        gcppropagator.CloudTraceOneWayPropagator{},
        propagation.TraceContext{},
    ),
)
handler := NewServeMux()
handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	logger.InfoContext(r.Context(), "Hello World") // This log should include trace information.
})
handler := otelhttp.NewHandler(handler, traceName)
```

You can see [example for Cloud Run](./examples/cloudrun).

If you set any of `slogdriver.TraceKey`, `slogdriver.SpanIDKey` or `slogdriver.TraceSampledKey` manually (in log arguments or `With`), trace information from the context is not added, so that a manual trace is not mixed with a span of another trace.
For the same reason, if the log arguments have any of these keys, all of them set by `With` are ignored.
When both OpenTelemetry and OpenCensus spans are in the context, the OpenTelemetry span is used.

#### Labels

You can add any "labels" to your log as following:

```go
logger.Info("", slog.Group(slogdriver.LabelKey, slog.String("label", "hoge")))
```

You can set common label like the follow:

```go
logger = logger.With(slog.Group(slogdriver.LabelKey, slog.String("commonLabel", "hoge")))
logger.Info("Hello World", slog.Group(slogdriver.LabelKey, slog.String("label1", "fuga")))
// got:
// {"severity":"INFO","message":"Hello World","logging.googleapis.com/labels":{"commonLabel":"hoge","label1":"fuga"}}
logger.Warn("Hello World", slog.Group(slogdriver.LabelKey, slog.String("label2", "fuga")))
// got:
// {"severity":"WARNING","message":"Hello World","logging.googleapis.com/labels":{"commonLabel":"hoge","label2":"fuga"}}
```

The value of `slogdriver.LabelKey` must be a group, a map with string keys, or a `slog.LogValuer` resolved to one of them, because Cloud Logging labels are a map. Otherwise, it is written as an ordinary attribute with the key `!BADKEY`, as `log/slog` does for malformed attributes.

```go
logger.Info("Hello World", slog.String(slogdriver.LabelKey, "hoge"))
// got:
// {"severity":"INFO","message":"Hello World","!BADKEY":"hoge"}
```

#### InsertID and Operation

`slogdriver.InsertIDKey` and `slogdriver.OperationKey` are always written at the top level, even under `WithGroup`.

```go
logger.Info("Hello World",
	slog.String(slogdriver.InsertIDKey, "insert-id"),
	slog.Group(slogdriver.OperationKey, slog.String("id", "operation-id"), slog.String("producer", "my-job")),
)
// got:
// {"severity":"INFO","message":"Hello World","logging.googleapis.com/insertId":"insert-id","logging.googleapis.com/operation":{"id":"operation-id","producer":"my-job"}}
```

#### Special keys passed via `With`

When the same special key (`httpRequest`, `logging.googleapis.com/sourceLocation`, `logging.googleapis.com/insertId`, `logging.googleapis.com/operation`, `logging.googleapis.com/trace`, `logging.googleapis.com/spanId` or `logging.googleapis.com/trace_sampled`) is passed both via `With` and as a log argument, it is written once with the value of the log argument. When it is passed in several `With` calls, the last one is used.

```go
logger.With(slog.String(slogdriver.InsertIDKey, "with")).Info("Hello World", slog.String(slogdriver.InsertIDKey, "arg"))
// got:
// {"severity":"INFO","message":"Hello World","logging.googleapis.com/insertId":"arg"}
```

#### Source Location

```go
logger := slogdriver.New(os.Stdout, slogdriver.HandlerOptions{AddSource: true})
logger.Info("Hello World")
// {"severity":"INFO","message":"Hello World","logging.googleapis.com/sourceLocation":{"file":"/path/to/source.go","line":"12","function":"yourFunction"}}
```

#### Error Reporting

Set `ErrorReporting` to report logs at `slog.LevelError` or above to [Cloud Error Reporting](https://cloud.google.com/error-reporting/docs/formatting-error-messages). It is disabled by default, and logs below `slog.LevelError` are not changed.

```go
logger := slogdriver.New(os.Stdout, slogdriver.HandlerOptions{
	ErrorReporting: &slogdriver.ErrorReportingOptions{Service: "my-service", Version: "v1.0.0"},
})
logger.Error("failed to save user", slog.Any("error", err))
// got:
// {"severity":"ERROR","message":"failed to save user","error":"connection refused","@type":"type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent","serviceContext":{"service":"my-service","version":"v1.0.0"},"context":{"reportLocation":{"filePath":"/path/to/main.go","lineNumber":12,"functionName":"main.saveUser"}},"stack_trace":"failed to save user: connection refused\n\ngoroutine 1 [running]:\nmain.saveUser(...)\n\t/path/to/main.go:12 +0x1d\nmain.main(...)\n\t/path/to/main.go:20 +0x25\n"}
```

- `Service` and `Version` default to the `K_SERVICE` and `K_REVISION` environment variables set by Cloud Run. If `Service` is still empty, the executable name is used. If `Version` is still empty, it is omitted.
- `context.reportLocation` is the location of the log call.
- `stack_trace` is the call stack from the log call, in the format of `runtime/debug.Stack`. Its first line is the message, followed by the first `error` value in the log arguments (`"message: error"`). Error Reporting uses this line as the error message.
- When the record is handled in another goroutine than the log call (e.g. an asynchronous handler wraps slogdriver), `stack_trace` is omitted and only `context.reportLocation` is written.
- Do not use `@type`, `serviceContext`, `context` or `stack_trace` as top-level keys of your own attributes for error logs, because they would be written twice.

## TODO

- [x] severity
- [x] message
- [x] httpRequest
- [x] time, timestamp
- [x] insertId
- [x] labels
- [x] operation
- [x] sourceLocation
- [x] spanId
- [x] trace
- [x] traceSampled

## docs

https://cloud.google.com/logging/docs/structured-logging#special-payload-fields
