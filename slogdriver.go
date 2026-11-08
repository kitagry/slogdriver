package slogdriver

import (
	"context"
	"io"
	"log/slog"
	"maps"
	"os"
	"runtime"
	"slices"
)

const (
	MessageKey        = "message"
	SeverityKey       = "severity"
	HTTPKey           = "httpRequest"
	SourceLocationKey = "logging.googleapis.com/sourceLocation"
	LabelKey          = "logging.googleapis.com/labels"

	InsertIDKey  = "logging.googleapis.com/insertId"
	OperationKey = "logging.googleapis.com/operation"

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
	InsertIDKey:       {},
	OperationKey:      {},
	TraceKey:          {},
	SpanIDKey:         {},
	TraceSampledKey:   {},
}

type cloudLoggingHandler struct {
	slog.Handler
	labels []slog.Attr
	groups []group
	// withKnownKeys holds known keys passed to WithAttrs, which are already written by Handler.
	withKnownKeys map[string]struct{}
	opts          HandlerOptions
	// serviceContext is nil when Error Reporting is disabled.
	serviceContext *serviceContext
}

type group struct {
	name  string
	attrs []any
}

type HandlerOptions struct {
	// ProjectId is Google Cloud Project ID
	// trace is written as "projects/PROJECT_ID/traces/TRACE_ID" when this or GOOGLE_CLOUD_PROJECT environment is set,
	// otherwise it is written as the trace ID only.
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

	// ErrorReporting makes logs at slog.LevelError or above reported to Cloud Error Reporting.
	// If nil, logs are written as before.
	ErrorReporting *ErrorReportingOptions
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
		// see makeSourceLocationAttr.
		AddSource: false,
		Level:     opts.Level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Built-in level/message attrs are always top-level; nested ones are user attrs.
			if len(groups) > 0 {
				return a
			}
			switch a.Key {
			case slog.LevelKey:
				// User attrs may also use the "level" key, but only the built-in one holds a slog.Level.
				level, ok := a.Value.Any().(slog.Level)
				if !ok {
					return a
				}
				return slog.String(SeverityKey, levelToSeverity(level))
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

	h := &cloudLoggingHandler{
		Handler: slog.NewJSONHandler(w, &slogOpts),
		opts:    opts,
	}
	if opts.ErrorReporting != nil {
		sc := newServiceContext(opts.ErrorReporting)
		h.serviceContext = &sc
	}
	return h
}

var _ slog.Handler = (*cloudLoggingHandler)(nil)

func (c *cloudLoggingHandler) Handle(ctx context.Context, r slog.Record) error {
	newRecord := slog.NewRecord(r.Time, r.Level, r.Message, 0)
	labels := make([]slog.Attr, 0, len(c.opts.DefaultLabels)+len(c.labels))
	labels = append(labels, c.opts.DefaultLabels...)
	labels = append(labels, c.labels...)
	knownAttrs := make([]slog.Attr, 0, len(knownKeys))
	normalAttrs := make([]any, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == LabelKey && a.Value.Kind() == slog.KindGroup {
			// If a is label groups, merge it with c.labels.
			labels = append(labels, a.Value.Group()...)
			return true
		}

		if _, ok := knownKeys[a.Key]; ok {
			knownAttrs = append(knownAttrs, a)
			return true
		}

		normalAttrs = append(normalAttrs, a)
		return true
	})

	groupedAttr := slices.Clone(normalAttrs)
	for _, group := range slices.Backward(c.groups) {
		groupedAttr = []any{slog.Group(group.name, slices.Concat(group.attrs, groupedAttr)...)}
	}
	newRecord.Add(groupedAttr...)

	if len(labels) > 0 {
		newRecord.AddAttrs(slog.Group(LabelKey, toAnySlice(labels)...))
	}

	if len(knownAttrs) > 0 {
		newRecord.AddAttrs(knownAttrs...)
	}

	reportError := c.serviceContext != nil && r.Level >= slog.LevelError
	var frame runtime.Frame
	if c.opts.AddSource || reportError {
		frame = recordFrame(r)
	}

	if c.opts.AddSource {
		newRecord.AddAttrs(makeSourceLocationAttr(frame))
	}

	c.handleTrace(ctx, &newRecord, knownAttrs)

	if reportError {
		newRecord.AddAttrs(c.errorReportingAttrs(r, frame)...)
	}

	return c.Handler.Handle(ctx, newRecord)
}

func (c *cloudLoggingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var labels []slog.Attr
	var newKnownKeys []string
	i := 0
	groupAttrs := make([]any, 0, len(attrs))
	for _, a := range attrs {
		if a.Key == LabelKey && a.Value.Kind() == slog.KindGroup {
			labels = append(labels, a.Value.Group()...)
			continue
		}

		_, known := knownKeys[a.Key]
		if len(c.groups) > 0 && !known {
			groupAttrs = append(groupAttrs, a)
			continue
		}
		if known {
			newKnownKeys = append(newKnownKeys, a.Key)
		}

		attrs[i] = a
		i++
	}
	attrs = attrs[:i]
	l := c.Handler.WithAttrs(attrs)
	h := c.clone(l)
	if len(newKnownKeys) > 0 {
		// clone shares withKnownKeys, so copy it before mutating.
		h.withKnownKeys = maps.Clone(c.withKnownKeys)
		if h.withKnownKeys == nil {
			h.withKnownKeys = make(map[string]struct{}, len(newKnownKeys))
		}
		for _, k := range newKnownKeys {
			h.withKnownKeys[k] = struct{}{}
		}
	}
	h.labels = append(h.labels, labels...)

	if len(groupAttrs) > 0 {
		h.groups[len(h.groups)-1].attrs = append(h.groups[len(h.groups)-1].attrs, groupAttrs...)
	}

	return h
}

func (c *cloudLoggingHandler) WithGroup(name string) slog.Handler {
	h := c.clone(c.Handler)
	h.groups = append(h.groups, group{name: name})
	return h
}

func (c *cloudLoggingHandler) clone(handler slog.Handler) *cloudLoggingHandler {
	return &cloudLoggingHandler{
		Handler:        handler,
		labels:         slices.Clone(c.labels),
		groups:         slices.Clone(c.groups),
		withKnownKeys:  c.withKnownKeys, // read-only; WithAttrs copies it before adding keys
		opts:           c.opts,
		serviceContext: c.serviceContext,
	}
}

func toAnySlice[T any](tl []T) []any {
	result := make([]any, len(tl))
	for i, t := range tl {
		result[i] = t
	}
	return result
}
