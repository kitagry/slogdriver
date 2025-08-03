package slogdriver

import (
	"context"
	"io"
	"log/slog"
	"os"
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

var knownKeys = map[string]struct{}{
	MessageKey:        {},
	SeverityKey:       {},
	HTTPKey:           {},
	SourceLocationKey: {},
	LabelKey:          {},
	TraceKey:          {},
	SpanIDKey:         {},
	TraceSampledKey:   {},
}

type cloudLoggingHandler struct {
	slog.Handler
	labels []any
	groups []string
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

	// DefaultLabels is a set of default labels to be added to each log entry.
	DefaultLabels []slog.Attr
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
				if a.Value.String() == "" {
					return slog.Attr{}
				}
				return slog.Attr{
					Key:   MessageKey,
					Value: a.Value,
				}
			}
			return a
		},
	}

	return &cloudLoggingHandler{
		Handler: slog.NewJSONHandler(w, &slogOpts),
		opts:    opts,
	}
}

var _ slog.Handler = (*cloudLoggingHandler)(nil)

func (c *cloudLoggingHandler) Handle(ctx context.Context, r slog.Record) error {
	newRecord := slog.NewRecord(r.Time, r.Level, r.Message, 0)
	attrs := make([]any, 0, r.NumAttrs())
	var labels []any
	knownAttrs := make([]slog.Attr, 0, len(knownKeys))
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == LabelKey && a.Value.Kind() == slog.KindGroup {
			// If a is label groups, merge it with c.labels.
			labels = make([]any, 0, len(a.Value.Group())+len(c.labels)+len(c.opts.DefaultLabels))
			for _, l := range c.opts.DefaultLabels {
				labels = append(labels, l)
			}
			for _, attr := range a.Value.Group() {
				labels = append(labels, attr)
			}
			labels = append(labels, c.labels...)
			return true
		}
		if len(c.groups) > 0 {
			if _, ok := knownKeys[a.Key]; ok {
				knownAttrs = append(knownAttrs, a)
				return true
			}
		}
		attrs = append(attrs, a)
		return true
	})

	for i := len(c.groups) - 1; i >= 0; i-- {
		attrs = []any{slog.Group(c.groups[i], attrs...)}
	}

	newRecord.Add(attrs...)

	if len(labels) == 0 && len(c.opts.DefaultLabels)+len(c.labels) > 0 {
		labels = make([]any, 0, len(c.opts.DefaultLabels)+len(c.labels))
		for _, l := range c.opts.DefaultLabels {
			labels = append(labels, l)
		}
		labels = append(labels, c.labels...)
	}
	if len(labels) > 0 {
		newRecord.AddAttrs(slog.Group(LabelKey, labels...))
	}

	if len(knownAttrs) > 0 {
		newRecord.AddAttrs(knownAttrs...)
	}

	if c.opts.AddSource {
		newRecord.AddAttrs(c.makeSourceLocationAttr(r))
	}

	c.handleTrace(ctx, &newRecord)

	return c.Handler.Handle(ctx, newRecord)
}

func (c *cloudLoggingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var labels []any
	i := 0
	for _, a := range attrs {
		if a.Key == LabelKey && a.Value.Kind() == slog.KindGroup {
			labels = make([]any, len(a.Value.Group()))
			for i, attr := range a.Value.Group() {
				labels[i] = attr
			}
			continue
		}
		attrs[i] = a
		i++
	}
	attrs = attrs[:i]
	l := c.Handler.WithAttrs(attrs)
	h := c.clone(l)
	h.labels = append(h.labels, labels...)
	return h
}

func (c *cloudLoggingHandler) WithGroup(name string) slog.Handler {
	h := c.clone(c.Handler)
	h.groups = append(h.groups, name)
	return h
}

func (c *cloudLoggingHandler) clone(handler slog.Handler) *cloudLoggingHandler {
	labels := make([]any, len(c.labels))
	copy(labels, c.labels)
	groups := make([]string, len(c.groups))
	copy(groups, c.groups)
	return &cloudLoggingHandler{handler, labels, groups, c.opts}
}
