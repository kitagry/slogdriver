package slogdriver

import (
	"context"
	"encoding"
	"io"
	"log/slog"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"time"
)

// builtinKeyPrefix is prepended to the key of a top-level user attribute named MessageKey or SeverityKey,
// so that it does not duplicate the built-in one. See escapeTopLevelAttrs.
const builtinKeyPrefix = "fields."

const (
	MessageKey        = "message"
	SeverityKey       = "severity"
	HTTPKey           = "httpRequest"
	SourceLocationKey = "logging.googleapis.com/sourceLocation"
	// LabelKey is the key of the labels group. Its values are written as strings and nested groups are flattened with ".",
	// because Cloud Logging labels are map<string,string>.
	LabelKey = "logging.googleapis.com/labels"

	InsertIDKey  = "logging.googleapis.com/insertId"
	OperationKey = "logging.googleapis.com/operation"

	TraceKey        = "logging.googleapis.com/trace"
	SpanIDKey       = "logging.googleapis.com/spanId"
	TraceSampledKey = "logging.googleapis.com/trace_sampled"
)

// knownKeys are written at the top level even under WithGroup.
// MessageKey and SeverityKey are not included, because they are written from the record; see builtinKeyPrefix.
var knownKeys = map[string]struct{}{
	HTTPKey:           {},
	SourceLocationKey: {},
	InsertIDKey:       {},
	OperationKey:      {},
	TraceKey:          {},
	SpanIDKey:         {},
	TraceSampledKey:   {},
}

type cloudLoggingHandler struct {
	slog.Handler
	// labels holds DefaultLabels and the labels passed to WithAttrs, already converted by appendLabels and unique by key.
	labels []slog.Attr
	groups []group
	// knownAttrs holds known keys passed to WithAttrs. They are written in Handle instead of by Handler,
	// so that the log arguments can override them.
	knownAttrs []slog.Attr
	opts       HandlerOptions
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
	// Like the other labels, the values are written as strings and nested groups are flattened; see LabelKey.
	// They are converted once in NewHandler, so a slog.LogValuer is resolved only once, as in With.
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
			// Top-level user level/msg attrs are marked by escapeTopLevelAttrs.
			if v, ok := a.Value.Any().(userAttrValue); ok {
				a.Value = v.Value
				return a
			}
			switch a.Key {
			case slog.LevelKey:
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
		labels:  uniqueByKey(appendLabels(nil, "", opts.DefaultLabels)),
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
	var argLabels, argKnownAttrs []slog.Attr
	normalAttrs := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		a, kind := classifyAttr(a)
		switch kind {
		case labelAttr:
			argLabels = appendLabels(argLabels, "", a.Value.Group())
		case knownAttr:
			argKnownAttrs = append(argKnownAttrs, a)
		default:
			normalAttrs = append(normalAttrs, a)
		}
		return true
	})
	knownAttrs := c.mergeKnownAttrs(argKnownAttrs)
	labels := c.labels
	if len(argLabels) > 0 {
		labels = uniqueByKey(slices.Concat(c.labels, argLabels))
	}

	if len(c.groups) == 0 {
		escapeTopLevelAttrs(normalAttrs)
	}
	groupedAttr := toAnySlice(normalAttrs)
	for _, group := range slices.Backward(c.groups) {
		groupedAttr = []any{slog.Group(group.name, slices.Concat(group.attrs, groupedAttr)...)}
	}
	newRecord.Add(groupedAttr...)

	if len(labels) > 0 {
		newRecord.AddAttrs(slog.Attr{Key: LabelKey, Value: slog.GroupValue(labels...)})
	}

	if len(knownAttrs) > 0 {
		newRecord.AddAttrs(knownAttrs...)
	}

	reportError := c.serviceContext != nil && r.Level >= slog.LevelError
	addSource := c.opts.AddSource && !slices.ContainsFunc(knownAttrs, isSourceLocationAttr)
	var frame runtime.Frame
	if addSource || reportError {
		frame = recordFrame(r)
	}

	if addSource {
		newRecord.AddAttrs(makeSourceLocationAttr(frame))
	}

	c.handleTrace(ctx, &newRecord, knownAttrs)

	if reportError {
		newRecord.AddAttrs(c.errorReportingAttrs(r, frame)...)
	}

	return c.Handler.Handle(ctx, newRecord)
}

func (c *cloudLoggingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var labels, knownAttrs []slog.Attr
	normalAttrs := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		a, kind := classifyAttr(a)
		switch kind {
		case labelAttr:
			labels = appendLabels(labels, "", a.Value.Group())
		case knownAttr:
			knownAttrs = append(knownAttrs, a)
		default:
			normalAttrs = append(normalAttrs, a)
		}
	}

	handler := c.Handler
	if len(c.groups) == 0 && len(normalAttrs) > 0 {
		escapeTopLevelAttrs(normalAttrs)
		handler = c.Handler.WithAttrs(normalAttrs)
	}
	h := c.clone(handler)
	if len(labels) > 0 {
		h.labels = uniqueByKey(slices.Concat(c.labels, labels))
	}
	if len(knownAttrs) > 0 {
		h.knownAttrs = uniqueByKey(slices.Concat(c.knownAttrs, knownAttrs))
	}
	if len(c.groups) > 0 && len(normalAttrs) > 0 {
		// clone shares the backing array of attrs, so appending in place would overwrite a sibling's attrs.
		last := &h.groups[len(h.groups)-1]
		last.attrs = slices.Concat(last.attrs, toAnySlice(normalAttrs))
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
		labels:         c.labels, // read-only; WithAttrs replaces it with a new slice
		groups:         slices.Clone(c.groups),
		knownAttrs:     c.knownAttrs, // read-only; WithAttrs replaces it with a new slice
		opts:           c.opts,
		serviceContext: c.serviceContext,
	}
}

// userAttrValue marks a top-level user attr named slog.LevelKey or slog.MessageKey,
// because ReplaceAttr cannot tell it from the built-in one, which is converted to SeverityKey or MessageKey.
type userAttrValue struct{ slog.Value }

// escapeTopLevelAttrs keeps top-level user attrs from duplicating the built-in severity and message.
// It must be done before the attrs are passed to slog, because slog does not call ReplaceAttr for groups.
// It modifies attrs in place, so callers must pass a slice they own.
func escapeTopLevelAttrs(attrs []slog.Attr) {
	for i, a := range attrs {
		switch a.Key {
		case SeverityKey, MessageKey:
			attrs[i].Key = builtinKeyPrefix + a.Key
		case slog.LevelKey, slog.MessageKey:
			// Groups are written as is, because ReplaceAttr is not called for them.
			if v := a.Value.Resolve(); v.Kind() != slog.KindGroup {
				attrs[i].Value = slog.AnyValue(userAttrValue{v})
			}
		case "":
			// slog inlines the attrs of a group with an empty key into the top level.
			if v := a.Value.Resolve(); v.Kind() == slog.KindGroup {
				group := slices.Clone(v.Group())
				escapeTopLevelAttrs(group)
				attrs[i].Value = slog.GroupValue(group...)
			}
		}
	}
}

func toAnySlice[T any](tl []T) []any {
	result := make([]any, len(tl))
	for i, t := range tl {
		result[i] = t
	}
	return result
}

type attrKind int

const (
	normalAttr attrKind = iota
	labelAttr
	knownAttr
)

// badKey is the key slog uses for malformed attributes.
const badKey = "!BADKEY"

// classifyAttr resolves the value of LabelKey, because a LogValuer may be resolved to a labels group.
// It also resolves known keys, because those passed to WithAttrs are kept unformatted and must not be resolved on every log.
func classifyAttr(a slog.Attr) (slog.Attr, attrKind) {
	if a.Key == LabelKey {
		a.Value = a.Value.Resolve()
		if a.Value.Kind() == slog.KindGroup {
			return a, labelAttr
		}
		if v, ok := mapToGroupValue(a.Value); ok {
			a.Value = v
			return a, labelAttr
		}
		// Cloud Logging labels must be a map, so a non-group value would be written as another labels key.
		// Keep it as an ordinary attribute instead of dropping it.
		a.Key = badKey
		return a, normalAttr
	}
	if _, ok := knownKeys[a.Key]; ok {
		a.Value = a.Value.Resolve()
		return a, knownAttr
	}
	return a, normalAttr
}

// mapToGroupValue converts a map with string keys to a group sorted by key,
// so that a map passed as labels is merged with other labels instead of being written as another labels key.
func mapToGroupValue(v slog.Value) (slog.Value, bool) {
	if v.Kind() != slog.KindAny {
		return slog.Value{}, false
	}
	rv := reflect.ValueOf(v.Any())
	if rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return slog.Value{}, false
	}
	attrs := make([]slog.Attr, 0, rv.Len())
	for iter := rv.MapRange(); iter.Next(); {
		attrs = append(attrs, slog.Any(iter.Key().String(), iter.Value().Interface()))
	}
	slices.SortFunc(attrs, func(a, b slog.Attr) int { return strings.Compare(a.Key, b.Key) })
	return slog.GroupValue(attrs...), true
}

// appendLabels appends attrs to dst as string labels, because Cloud Logging labels are map<string,string>.
// Nested groups and maps are flattened by joining the keys with "." under prefix, and LogValuers are resolved.
func appendLabels(dst []slog.Attr, prefix string, attrs []slog.Attr) []slog.Attr {
	for _, a := range attrs {
		v := a.Value.Resolve()
		if g, ok := mapToGroupValue(v); ok {
			v = g
		}
		if v.Kind() == slog.KindGroup {
			// Like slog, the attrs of a group with an empty key are inlined.
			p := prefix
			if a.Key != "" {
				p = joinLabelKey(prefix, a.Key)
			}
			dst = appendLabels(dst, p, v.Group())
			continue
		}
		if a.Key == "" {
			continue
		}
		dst = append(dst, slog.String(joinLabelKey(prefix, a.Key), labelString(v)))
	}
	return dst
}

func joinLabelKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// labelString formats a label value like slog.TextHandler, except that a time is written in RFC 3339 with nanoseconds.
func labelString(v slog.Value) string {
	switch v.Kind() {
	case slog.KindTime:
		// Value.String uses time.Time.String, which is not meant to be parsed.
		return v.Time().Format(time.RFC3339Nano)
	case slog.KindAny:
		switch x := v.Any().(type) {
		case encoding.TextMarshaler:
			if text, err := x.MarshalText(); err == nil {
				return string(text)
			}
		case []byte:
			return string(x)
		}
	}
	return v.String()
}

// mergeKnownAttrs merges the known keys passed to WithAttrs and the log arguments, preferring the log arguments.
// Trace keys passed to WithAttrs are dropped as a whole if the log arguments have any of them, so as not to mix two traces.
func (c *cloudLoggingHandler) mergeKnownAttrs(argAttrs []slog.Attr) []slog.Attr {
	if len(argAttrs) == 0 {
		// c.knownAttrs is already unique.
		return c.knownAttrs
	}
	withAttrs := c.knownAttrs
	if slices.ContainsFunc(argAttrs, isTraceAttr) {
		withAttrs = slices.DeleteFunc(slices.Clone(withAttrs), isTraceAttr)
	}
	return uniqueByKey(slices.Concat(withAttrs, argAttrs))
}

// uniqueByKey keeps the last attr of each key in place, so callers must pass a slice they own.
// It scans linearly instead of using a map, because it runs on every log with known keys or labels, and they are usually few.
func uniqueByKey(attrs []slog.Attr) []slog.Attr {
	result := attrs[:0]
	for i, a := range attrs {
		if !slices.ContainsFunc(attrs[i+1:], func(b slog.Attr) bool { return b.Key == a.Key }) {
			result = append(result, a)
		}
	}
	return result
}
