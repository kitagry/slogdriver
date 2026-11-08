package slogdriver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"os"
	"reflect"
	"slices"
	"testing"
	"testing/slogtest"

	"github.com/kitagry/slogdriver"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func ExampleNew() {
	logger := slogdriver.New(
		os.Stdout,
		slogdriver.HandlerOptions{
			AddSource:     true,
			DefaultLabels: []slog.Attr{slog.String("defaultLabel", "hoge")},
		},
	)
	logger = logger.With(slog.Group(slogdriver.LabelKey, slog.String("commonLabel", "fuga")))
	logger.Info("Hello World", slog.Group(slogdriver.LabelKey, slog.String("specifiedLabel", "piyo")))
}

func TestHandler(t *testing.T) {
	var buf bytes.Buffer
	slogtest.Run(
		t,
		func(t *testing.T) slog.Handler {
			buf.Reset()
			return slogdriver.NewHandler(&buf, slogdriver.HandlerOptions{})
		},
		func(t *testing.T) map[string]any {
			line := buf.Bytes()
			if len(line) == 0 {
				return nil
			}

			var m map[string]any
			if err := json.Unmarshal(line, &m); err != nil {
				t.Fatal(err)
			}

			for k := range m {
				switch k {
				case slogdriver.SeverityKey:
					m[slog.LevelKey] = m[k]
					delete(m, k)
				case slogdriver.MessageKey:
					m[slog.MessageKey] = m[k]
					delete(m, k)
				}
			}

			return m
		},
	)
}

func TestLabels(t *testing.T) {
	var buf bytes.Buffer

	logger := slogdriver.New(
		&buf,
		slogdriver.HandlerOptions{
			AddSource:     true,
			DefaultLabels: []slog.Attr{slog.String("defaultLabel", "hoge")},
		},
	)
	logger = logger.With(slog.Group(slogdriver.LabelKey, slog.String("commonLabel1", "fuga")))
	logger = logger.With(slog.Group(slogdriver.LabelKey, slog.String("commonLabel2", "piyo")))
	logger.Info("Hello World", slog.Group(slogdriver.LabelKey, slog.String("specifiedLabel", "hogera")))

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	if err != nil {
		t.Fatal(err)
	}

	labels, ok := result[slogdriver.LabelKey].(map[string]any)
	if !ok {
		t.Fatalf("unexpected type: %T", result[slogdriver.LabelKey])
	}

	if labels["defaultLabel"] != "hoge" {
		t.Errorf("unexpected defaultLabel: %s", labels["defaultLabel"])
	}

	if labels["commonLabel1"] != "fuga" {
		t.Errorf("unexpected commonLabel1: %s", labels["commonLabel"])
	}

	if labels["commonLabel2"] != "piyo" {
		t.Errorf("unexpected commonLabel2: %s", labels["commonLabel2"])
	}

	if labels["specifiedLabel"] != "hogera" {
		t.Errorf("unexpected specifiedLabel: %s", labels["specifiedLabel"])
	}
}

func TestShouldSetDefaultLabelsWithoutSpecifiedLabel(t *testing.T) {
	var buf bytes.Buffer

	logger := slogdriver.New(
		&buf,
		slogdriver.HandlerOptions{
			AddSource:     true,
			DefaultLabels: []slog.Attr{slog.String("defaultLabel", "hoge")},
		},
	)
	logger.Info("Hello World")

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	if err != nil {
		t.Fatal(err)
	}

	labels, ok := result[slogdriver.LabelKey].(map[string]any)
	if !ok {
		t.Fatalf("unexpected type: %T", result[slogdriver.LabelKey])
	}

	if labels["defaultLabel"] != "hoge" {
		t.Errorf("unexpected defaultLabel: %s", labels["defaultLabel"])
	}
}

func TestGroup(t *testing.T) {
	var buf bytes.Buffer

	logger := slogdriver.New(
		&buf,
		slogdriver.HandlerOptions{
			ProjectID: "test-project",
			AddSource: true,
		},
	)

	ctx := context.Background()

	res, err := resource.New(ctx, resource.WithDetectors(gcp.NewDetector()), resource.WithTelemetrySDK())
	if err != nil {
		t.Fatalf("failed to create resource: %+v", err)
	}

	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res))
	defer tp.Shutdown(ctx)
	otel.SetTracerProvider(tp)

	tracer := otel.Tracer("test-tracer")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	logger = logger.
		With(slog.Bool("commonKey1", true)).
		WithGroup("group1").With(slog.Int("commonKey2", 2)).
		WithGroup("group2").With(slog.String("commonKey3", "piyo"))
	logger.InfoContext(
		ctx,
		"Hello World",
		slogdriver.MakeHTTPAttrFromHTTPPayload(slogdriver.HTTPPayload{}),
		slog.Group(slogdriver.LabelKey, slog.String("label", "hoge")),
		slog.String("key1", "fuga"),
		slog.Int("key2", 1),
	)

	type entry struct {
		Group1 struct {
			Group2 struct {
				Key1       string `json:"key1"`
				Key2       int    `json:"key2"`
				CommonKey3 string `json:"commonKey3"`
			} `json:"group2"`
			CommonKey2 int `json:"commonKey2"`
		} `json:"group1"`
		CommonKey1 bool `json:"commonKey1"`

		HttpRequest  *slogdriver.HTTPPayload            `json:"httpRequest"`
		Source       *slogdriver.LogEntrySourceLocation `json:"logging.googleapis.com/sourceLocation"`
		Labels       map[string]any                     `json:"logging.googleapis.com/labels"`
		Trace        *string                            `json:"logging.googleapis.com/trace"`
		SpanID       *string                            `json:"logging.googleapis.com/spanId"`
		TraceSampled *bool                              `json:"logging.googleapis.com/trace_sampled"`
	}

	var result entry
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}

	if result.Group1.Group2.Key1 != "fuga" {
		t.Errorf("unexpected group1.group2.key1: %s", result.Group1.Group2.Key1)
	}

	if result.Group1.Group2.Key2 != 1 {
		t.Errorf("unexpected group1.group2.key2: %d", result.Group1.Group2.Key2)
	}

	if !result.CommonKey1 {
		t.Errorf("unexpected commonKey1: %v", result.CommonKey1)
	}

	if result.Group1.CommonKey2 != 2 {
		t.Errorf("unexpected group1.commonKey2: %d", result.Group1.CommonKey2)
	}

	if result.Group1.Group2.CommonKey3 != "piyo" {
		t.Errorf("unexpected group1.group2.commonKey3: %s", result.Group1.Group2.CommonKey3)
	}

	if result.HttpRequest == nil {
		t.Error("http request key not found")
	}

	if result.Source == nil {
		t.Error("source location key not found")
	}

	expectedLabels := map[string]any{"label": "hoge"}
	if !maps.Equal(result.Labels, expectedLabels) {
		t.Errorf("labels should be %v got %v", expectedLabels, result.Labels)
	}

	if result.Trace == nil {
		t.Error("trace key not found")
	}
	if result.SpanID == nil {
		t.Errorf("span id key not found")
	}
	if result.TraceSampled == nil {
		t.Errorf("trace sampled key not found")
	}
}

func TestLabels_MultipleLabelGroupsInOneWith(t *testing.T) {
	var buf bytes.Buffer

	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{})
	logger = logger.With(
		slog.Group(slogdriver.LabelKey, slog.String("a", "1")),
		slog.Group(slogdriver.LabelKey, slog.String("b", "2")),
	)
	logger.Info("Hello World")

	var result struct {
		Labels map[string]any `json:"logging.googleapis.com/labels"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}

	expected := map[string]any{"a": "1", "b": "2"}
	if !maps.Equal(result.Labels, expected) {
		t.Errorf("labels should be %v got %v", expected, result.Labels)
	}
}

func TestGroup_InsertIDAndOperationShouldBeTopLevel(t *testing.T) {
	var buf bytes.Buffer

	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{})
	logger.WithGroup("g").
		With(slog.String(slogdriver.InsertIDKey, "insert-id")).
		Info("Hello World", slog.Group(slogdriver.OperationKey, slog.String("id", "op-id")))

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}

	if result[slogdriver.InsertIDKey] != "insert-id" {
		t.Errorf("%s should be top-level, got %v", slogdriver.InsertIDKey, result)
	}
	expectedOperation := map[string]any{"id": "op-id"}
	if op, _ := result[slogdriver.OperationKey].(map[string]any); !maps.Equal(op, expectedOperation) {
		t.Errorf("%s should be top-level %v, got %v", slogdriver.OperationKey, expectedOperation, result)
	}
}

// assertKeyOnce counts the key in the raw output, because duplicated keys disappear when unmarshaled into a map.
func assertKeyOnce(t *testing.T, out []byte, key string) map[string]any {
	t.Helper()
	if n := bytes.Count(out, []byte(`"`+key+`":`)); n != 1 {
		t.Errorf("key=%s should appear once, got %d: %s", key, n, out)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestKnownKeys_LogArgumentShouldOverrideWith(t *testing.T) {
	tests := map[string]struct {
		key       string
		withValue any
		argValue  any
		want      any
	}{
		"insertId": {
			key:       slogdriver.InsertIDKey,
			withValue: "with",
			argValue:  "arg",
			want:      "arg",
		},
		"operation": {
			key:       slogdriver.OperationKey,
			withValue: map[string]string{"id": "with"},
			argValue:  map[string]string{"id": "arg"},
			want:      map[string]any{"id": "arg"},
		},
		"httpRequest": {
			key:       slogdriver.HTTPKey,
			withValue: slogdriver.HTTPPayload{RequestMethod: "GET"},
			argValue:  slogdriver.HTTPPayload{RequestMethod: "POST"},
			want:      "POST",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slogdriver.New(&buf, slogdriver.HandlerOptions{})

			logger.With(slog.Any(tt.key, tt.withValue)).Info("Hello World", slog.Any(tt.key, tt.argValue))

			got := assertKeyOnce(t, buf.Bytes(), tt.key)[tt.key]
			if p, ok := got.(map[string]any); ok && tt.key == slogdriver.HTTPKey {
				got = p["requestMethod"]
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("key=%s expected %v, got %v", tt.key, tt.want, got)
			}
		})
	}
}

func TestKnownKeys_LaterWithShouldOverrideEarlierWith(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{})

	logger.With(slog.String(slogdriver.InsertIDKey, "first")).
		WithGroup("g").
		With(slog.String(slogdriver.InsertIDKey, "second")).
		Info("Hello World")

	if got := assertKeyOnce(t, buf.Bytes(), slogdriver.InsertIDKey)[slogdriver.InsertIDKey]; got != "second" {
		t.Errorf("expected second, got %v", got)
	}
}

func TestKnownKeys_WithShouldNotAffectParentLogger(t *testing.T) {
	var buf bytes.Buffer
	parent := slogdriver.New(&buf, slogdriver.HandlerOptions{}).With(slog.String(slogdriver.InsertIDKey, "parent"))
	_ = parent.With(slog.String(slogdriver.InsertIDKey, "child"))

	parent.Info("Hello World")

	if got := assertKeyOnce(t, buf.Bytes(), slogdriver.InsertIDKey)[slogdriver.InsertIDKey]; got != "parent" {
		t.Errorf("expected parent, got %v", got)
	}
}

func TestLabels_NonGroupLabelKeyShouldNotDuplicateLabels(t *testing.T) {
	tests := map[string]func(*slog.Logger){
		"log argument": func(l *slog.Logger) {
			l.Info("Hello World", slog.String(slogdriver.LabelKey, "x"))
		},
		"With": func(l *slog.Logger) {
			l.With(slog.String(slogdriver.LabelKey, "x")).Info("Hello World")
		},
	}
	for name, log := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			log(slogdriver.New(&buf, slogdriver.HandlerOptions{
				DefaultLabels: []slog.Attr{slog.String("d", "1")},
			}))

			got := assertKeyOnce(t, buf.Bytes(), slogdriver.LabelKey)
			if labels, _ := got[slogdriver.LabelKey].(map[string]any); !maps.Equal(labels, map[string]any{"d": "1"}) {
				t.Errorf("labels should be {d:1}, got %v", got[slogdriver.LabelKey])
			}
			// The value is kept as an ordinary attribute, like slog does for malformed attributes.
			if got["!BADKEY"] != "x" {
				t.Errorf("non-group label value should be kept in !BADKEY, got %v", got)
			}
		})
	}
}

func TestLabels_NonGroupLabelKeyUnderGroupShouldBeInGroup(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{})

	logger.WithGroup("g").Info("Hello World", slog.String(slogdriver.LabelKey, "x"))

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got[slogdriver.LabelKey]; ok {
		t.Errorf("labels should not be written: %s", buf.Bytes())
	}
	if g, _ := got["g"].(map[string]any); g["!BADKEY"] != "x" {
		t.Errorf("non-group label value should be in g.!BADKEY, got %v", got)
	}
}

type labelsValuer struct{}

func (labelsValuer) LogValue() slog.Value {
	return slog.GroupValue(slog.String("valuer", "v"))
}

func TestLabels_LogValuerResolvedToGroupShouldBeMerged(t *testing.T) {
	tests := map[string]func(*slog.Logger){
		"log argument": func(l *slog.Logger) {
			l.Info("Hello World", slog.Any(slogdriver.LabelKey, labelsValuer{}))
		},
		"With": func(l *slog.Logger) {
			l.With(slog.Any(slogdriver.LabelKey, labelsValuer{})).Info("Hello World")
		},
	}
	for name, log := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			log(slogdriver.New(&buf, slogdriver.HandlerOptions{
				DefaultLabels: []slog.Attr{slog.String("d", "1")},
			}))

			got := assertKeyOnce(t, buf.Bytes(), slogdriver.LabelKey)
			want := map[string]any{"d": "1", "valuer": "v"}
			if labels, _ := got[slogdriver.LabelKey].(map[string]any); !maps.Equal(labels, want) {
				t.Errorf("labels should be %v, got %v", want, got[slogdriver.LabelKey])
			}
		})
	}
}

func TestGroup_SiblingWithShouldNotOverwriteEachOther(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{})
	// The second With grows the group attrs with spare capacity, which siblings must not share.
	base := logger.WithGroup("g").With("a", 1, "b", 2).With("c", 3)
	first := base.With("d", 4)
	_ = base.With("e", 5)

	first.Info("Hello World")

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"a": 1.0, "b": 2.0, "c": 3.0, "d": 4.0}
	if g, _ := got["g"].(map[string]any); !maps.Equal(g, want) {
		t.Errorf("g should be %v, got %v", want, got["g"])
	}
}

type countingValuer struct{ n *int }

func (v countingValuer) LogValue() slog.Value {
	*v.n++
	return slog.StringValue("resolved")
}

func TestKnownKeys_LogValuerInWithShouldBeResolvedOnce(t *testing.T) {
	var buf bytes.Buffer
	n := 0
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{}).With(slog.Any(slogdriver.InsertIDKey, countingValuer{&n}))

	logger.Info("first")
	logger.Info("second")

	if n != 1 {
		t.Errorf("LogValue passed to With should be called once like other attrs, got %d", n)
	}
}

func TestLabels_MapShouldBeMerged(t *testing.T) {
	tests := map[string]func(*slog.Logger){
		"map[string]string in log argument": func(l *slog.Logger) {
			l.Info("Hello World", slog.Any(slogdriver.LabelKey, map[string]string{"m": "v"}))
		},
		"map[string]any in With": func(l *slog.Logger) {
			l.With(slog.Any(slogdriver.LabelKey, map[string]any{"m": "v"})).Info("Hello World")
		},
	}
	for name, log := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			log(slogdriver.New(&buf, slogdriver.HandlerOptions{
				DefaultLabels: []slog.Attr{slog.String("d", "1")},
			}))

			got := assertKeyOnce(t, buf.Bytes(), slogdriver.LabelKey)
			want := map[string]any{"d": "1", "m": "v"}
			if labels, _ := got[slogdriver.LabelKey].(map[string]any); !maps.Equal(labels, want) {
				t.Errorf("labels should be %v, got %v", want, got[slogdriver.LabelKey])
			}
		})
	}
}

// topLevelKeys returns the top-level keys of a JSON object in order, keeping duplicates.
func topLevelKeys(t *testing.T, out []byte) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(out))
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, tok.(string))
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

func TestBuiltinKeys_UserAttrsShouldNotDuplicateSeverityAndMessage(t *testing.T) {
	tests := map[string]struct {
		logFn func(*slog.Logger)
		want  map[string]any
	}{
		"log arguments": {
			logFn: func(l *slog.Logger) {
				l.Info("m", slog.String("severity", "X"), slog.String("message", "Y"))
			},
			want: map[string]any{"fields.severity": "X", "fields.message": "Y"},
		},
		"with": {
			logFn: func(l *slog.Logger) {
				l.With(slog.String("severity", "X"), slog.String("message", "Y")).Info("m")
			},
			want: map[string]any{"fields.severity": "X", "fields.message": "Y"},
		},
		"log arguments under group": {
			logFn: func(l *slog.Logger) {
				l.WithGroup("g").Info("m", slog.String("severity", "X"), slog.String("message", "Y"))
			},
			want: map[string]any{"g": map[string]any{"severity": "X", "message": "Y"}},
		},
		"with under group": {
			logFn: func(l *slog.Logger) {
				l.WithGroup("g").With(slog.String("severity", "X"), slog.String("message", "Y")).Info("m")
			},
			want: map[string]any{"g": map[string]any{"severity": "X", "message": "Y"}},
		},
		"msg in log arguments": {
			logFn: func(l *slog.Logger) {
				l.Info("m", slog.String(slog.MessageKey, "Y"))
			},
			want: map[string]any{slog.MessageKey: "Y"},
		},
		"msg in with": {
			logFn: func(l *slog.Logger) {
				l.With(slog.String(slog.MessageKey, "Y")).Info("m")
			},
			want: map[string]any{slog.MessageKey: "Y"},
		},
		"group values": {
			logFn: func(l *slog.Logger) {
				l.Info("m", slog.Group("severity", slog.String("a", "X")), slog.Group("message", slog.String("b", "Y")))
			},
			want: map[string]any{"fields.severity": map[string]any{"a": "X"}, "fields.message": map[string]any{"b": "Y"}},
		},
		"group values in with": {
			logFn: func(l *slog.Logger) {
				l.With(slog.Group("severity", slog.String("a", "X"))).Info("m")
			},
			want: map[string]any{"fields.severity": map[string]any{"a": "X"}},
		},
		"empty-key group": {
			logFn: func(l *slog.Logger) {
				l.Info("m", slog.Group("", slog.String("severity", "X"), slog.String(slog.MessageKey, "Y")))
			},
			want: map[string]any{"fields.severity": "X", slog.MessageKey: "Y"},
		},
		"level holding slog.Level": {
			logFn: func(l *slog.Logger) {
				l.Info("m", slog.Any(slog.LevelKey, slog.LevelWarn))
			},
			want: map[string]any{slog.LevelKey: "WARN"},
		},
		"group argument": {
			logFn: func(l *slog.Logger) {
				l.Info("m", slog.Group("g", slog.String("severity", "X"), slog.String("message", "Y")))
			},
			want: map[string]any{"g": map[string]any{"severity": "X", "message": "Y"}},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.logFn(slogdriver.New(&buf, slogdriver.HandlerOptions{}))

			keys := topLevelKeys(t, buf.Bytes())
			for _, key := range []string{slogdriver.SeverityKey, slogdriver.MessageKey} {
				if n := len(slices.DeleteFunc(slices.Clone(keys), func(k string) bool { return k != key })); n != 1 {
					t.Errorf("key=%s should appear once at the top level, got %d: %s", key, n, buf.Bytes())
				}
			}
			var got map[string]any
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got[slogdriver.SeverityKey] != "INFO" || got[slogdriver.MessageKey] != "m" {
				t.Errorf("built-in severity and message should be kept, got %s", buf.Bytes())
			}
			for k, want := range tt.want {
				if !reflect.DeepEqual(got[k], want) {
					t.Errorf("key=%s expected %v, got %v", k, want, got[k])
				}
			}
		})
	}
}
