# slogdriver

[slog](https://github.com/golang/exp/tree/master/slog) based logging library optimized for [Cloud Logging](https://cloud.google.com/logging). This packaged inspired by [Zapdriver](https://github.com/blendle/zapdriver) and [zerodriver](https://github.com/hirosassa/zerodriver).

## What is this package?

This package provides simple structured logger optimized for [Cloud Logging](https://cloud.google.com/logging) based on [slog](https://github.com/golang/exp/tree/master/slog).

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
p := slogdriver.NewHTTPPayload(req, res)
p.Latency = time.Since(start)
logger.Info("http finished", slogdriver.MakeHTTPAttrFromHTTPPayload(p))
// Or, you can create attr manually
logger.Info("http finished", slog.Any(slogdriver.HTTPKey, p))
```

#### Trace context

```go
import "go.opencensus.io/trace"

// Set projectId or, you can set environment GOOGLE_CLOUD_PROJECT
logger := slogdriver.New(os.Stdout, slogdriver.HandlerOptions{ProjectID: "YOUR_PROJECT_ID"})

ctx, span := trace.StartSpan(context.Background(), "span")
defer span.End()

logger = logger.WithContext(ctx)
logger.Info("Hello World")
// got:
// {"severity":"INFO","message":"Hello World","logging.googleapis.com/trace":"projects/YOUR_PROJECT_ID/traces/00000000000000000000000000000000","logging.googleapis.com/spanId":"0000000000000000","logging.googleapis.com/trace_sampled":true}
```

If you use [go.opencensus.io/trace](https://pkg.go.dev/go.opencensus.io/trace), it is able to set traceId with request context like the below:

```go
import (
	"contrib.go.opencensus.io/exporter/stackdriver/propagation"
	"go.opencensus.io/plugin/ochttp"
)

handler := &ochttp.Handler{Propagation: &propagation.HTTPFormat{}}
handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	logger = logger.WithContext(r.Context())
	logger.Info("Hello World") // This log should include trace information.
})
```

You can see [example for Cloud Run](./examples/cloudrun).

#### Labels

You can add any "labels" to your log as following:

```go
logger.Info("", slog.Group(slogdriver.LabelKey, slog.String("label", "hoge")))
```

You can set common label like the follow:

```go
logger = logger.With(slog.Group(slogdriver.LabelKey, slog.String("commonLabel", "hoge")))
logger.Info("Hello World", slogdriver.LabelKey, slog.String("label1", "fuga"))
// got:
// {"severity":"INFO","message":"Hello World","logging.googleapis.com/labels":{"commonLabel":"hoge","label1":"fuga"}}
logger.Warn("Hello World", slogdriver.LabelKey, slog.String("label2", "fuga"))
// got:
// {"severity":"WARNING","message":"Hello World","logging.googleapis.com/labels":{"commonLabel":"hoge","label2":"fuga"}}
```

## TODO

- [x] severity
- [x] message
- [x] httpRequest
- [x] time, timestamp
- [ ] insertId
- [x] labels
- [ ] operation
- [x] sourceLocation
- [x] spanId
- [x] trace
- [x] traceSampled

## docs

https://cloud.google.com/logging/docs/structured-logging?hl=ja#special-payload-fields
