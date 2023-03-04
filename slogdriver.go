package slogdriver

import (
	"context"
	"io"
	"os"

	"golang.org/x/exp/slog"
)

const (
	MessageKey        = "message"
	SeverityKey       = "severity"
	HTTPKey           = "httpRequest"
	SourceLocationKey = "logging.googleapis.com/sourceLocation"
	LabelKey          = "logging.googleapis.com/labels"

	TraceKey        = "logging.googleapis.com/trace"
	SpanIDKey       = "logging.googleapis.com/spanId"
	TraceSampledKey = "logging.googleapis.com/trace_sampled"
)

type cloudLoggingHandler struct {
	slog.Handler
	labels []slog.Attr
	opts   HandlerOptions
}

type HandlerOptions struct {
	// ProjectId is Google Cloud Project ID
	// If you want to use trace_id, you should set this or set GOOGLE_CLOUD_PROJECT environment.
	// Cloud Shell and App Engine set this environment variable to the project ID, so use it if present.
	ProjectID string

	// When AddSource is true, the handler adds a ("logging.googleapis.com/sourceLocation", {"file":"path/to/file.go","line":"12"})
	// attribute to the output indicating the source code position of the log statement. AddSource is false by default
	// to skip the cost of computing this information.
	AddSource bool

	// Level reports the minimum record level that will be logged.
	// The handler discards records with lower levels.
	// If Level is nil, the handler assumes LevelInfo.
	// The handler calls Level.Level for each record processed;
	// to adjust the minimum level dynamically, use a LevelVar.
	Level slog.Leveler
}

func New(w io.Writer, opts HandlerOptions) *slog.Logger {
	return slog.New(NewHandler(w, opts))
}

func NewHandler(w io.Writer, opts HandlerOptions) slog.Handler {
	if projectID := os.Getenv("GOOGLE_CLOUD_PROJECT"); opts.ProjectID == "" && projectID != "" {
		opts.ProjectID = projectID
	}

	slogOpts := slog.HandlerOptions{
		// AddSource is handled in Handle method. So, this option is false.
		// see cloudLoggingHandler.makeSourceLocationAttr.
		AddSource: false,
		Level:     opts.Level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.LevelKey:
				val := levelStringToSeverity(a.Value.String())
				return slog.Attr{
					Key:   SeverityKey,
					Value: slog.StringValue(val),
				}
			case slog.MessageKey:
				return slog.Attr{
					Key:   MessageKey,
					Value: a.Value,
				}
			}
			return a
		},
	}
	return &cloudLoggingHandler{
		Handler: slogOpts.NewJSONHandler(w),
		opts:    opts,
	}
}

var _ slog.Handler = (*cloudLoggingHandler)(nil)

func (c *cloudLoggingHandler) Handle(ctx context.Context, r slog.Record) error {
	newRecord := slog.NewRecord(r.Time, r.Level, r.Message, 0)
	attrs := make([]slog.Attr, 0, r.NumAttrs())
	labelMerged := false
	r.Attrs(func(a slog.Attr) {
		if a.Key == LabelKey && a.Value.Kind() == slog.KindGroup {
			// If a is label groups, merge it with c.labels.
			newLabels := make([]slog.Attr, 0, len(a.Value.Group())+len(c.labels))
			newLabels = append(newLabels, a.Value.Group()...)
			newLabels = append(newLabels, c.labels...)
			attr := slog.Group(LabelKey, newLabels...)
			attrs = append(attrs, attr)
			labelMerged = true
			return
		}
		attrs = append(attrs, a)
	})

	newRecord.AddAttrs(attrs...)
	if !labelMerged && len(c.labels) > 0 {
		newRecord.AddAttrs(slog.Group(LabelKey, c.labels...))
	}

	if c.opts.AddSource {
		newRecord.AddAttrs(c.makeSourceLocationAttr(r))
	}

	c.handleTrace(ctx, &newRecord)

	return c.Handler.Handle(ctx, newRecord)
}

func (c *cloudLoggingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var labels []slog.Attr
	i := 0
	for _, a := range attrs {
		if a.Key == LabelKey && a.Value.Kind() == slog.KindGroup {
			labels = a.Value.Group()
			continue
		}
		attrs[i] = a
		i++
	}
	attrs = attrs[:i]
	l := c.Handler.WithAttrs(attrs)
	return c.cloneWithLabels(l, labels)
}

func (c *cloudLoggingHandler) WithGroup(name string) slog.Handler {
	return c.clone(c.Handler.WithGroup(name))
}

func (c *cloudLoggingHandler) clone(handler slog.Handler) *cloudLoggingHandler {
	labels := make([]slog.Attr, len(c.labels))
	copy(labels, c.labels)
	return c.cloneWithLabels(handler, labels)
}

func (c *cloudLoggingHandler) cloneWithLabels(handler slog.Handler, labels []slog.Attr) *cloudLoggingHandler {
	return &cloudLoggingHandler{handler, labels, c.opts}
}
