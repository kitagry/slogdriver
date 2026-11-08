package slogdriver_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kitagry/slogdriver"
)

type replaceAttrCall struct {
	groups []string
	key    string
}

// recordReplaceAttr returns a ReplaceAttr that records its calls and returns the attr unchanged.
func recordReplaceAttr(calls *[]replaceAttrCall) func([]string, slog.Attr) slog.Attr {
	return func(groups []string, a slog.Attr) slog.Attr {
		*calls = append(*calls, newReplaceAttrCall(groups, a))
		return a
	}
}

func newReplaceAttrCall(groups []string, a slog.Attr) replaceAttrCall {
	// slog passes an empty slice for no groups; nil makes the wants shorter.
	if len(groups) == 0 {
		groups = nil
	}
	return replaceAttrCall{groups: slices.Clone(groups), key: a.Key}
}

func decodeLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("failed to decode %q: %v", buf.String(), err)
	}
	return m
}

func TestReplaceAttr_ShouldReplaceUserAttr(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == "password" {
				return slog.String(a.Key, "***")
			}
			return a
		},
	})

	logger.Info("login", slog.String("password", "secret"))

	got := decodeLine(t, &buf)
	if got["password"] != "***" {
		t.Errorf("password = %v, want ***", got["password"])
	}
}

func TestReplaceAttr_EmptyKeyShouldRemoveAttr(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == "password" || a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})

	logger.Info("login", slog.String("password", "secret"), slog.String("user", "alice"))

	got := decodeLine(t, &buf)
	want := map[string]any{"severity": "INFO", "message": "login", "user": "alice"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestReplaceAttr_ShouldReceiveGroups(t *testing.T) {
	tests := map[string]struct {
		log  func(*slog.Logger)
		want []replaceAttrCall
	}{
		"slog.Group": {
			log: func(l *slog.Logger) {
				l.Info("msg", slog.Group("a", slog.Group("b", slog.String("k", "v"))))
			},
			want: []replaceAttrCall{{groups: []string{"a", "b"}, key: "k"}},
		},
		"WithGroup": {
			log: func(l *slog.Logger) {
				l.WithGroup("a").WithGroup("b").Info("msg", slog.String("k", "v"))
			},
			want: []replaceAttrCall{{groups: []string{"a", "b"}, key: "k"}},
		},
		"With under WithGroup": {
			log: func(l *slog.Logger) {
				l.WithGroup("a").With(slog.String("w", "v")).WithGroup("b").Info("msg", slog.String("k", "v"))
			},
			want: []replaceAttrCall{
				{groups: []string{"a"}, key: "w"},
				{groups: []string{"a", "b"}, key: "k"},
			},
		},
		"With at top level": {
			log: func(l *slog.Logger) {
				l.With(slog.String("w", "v")).Info("msg")
			},
			want: []replaceAttrCall{{groups: nil, key: "w"}},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var calls []replaceAttrCall
			var buf bytes.Buffer
			logger := slogdriver.New(&buf, slogdriver.HandlerOptions{ReplaceAttr: recordReplaceAttr(&calls)})

			tt.log(logger)

			got := slices.DeleteFunc(calls, func(c replaceAttrCall) bool {
				return c.key == slog.TimeKey || c.key == slogdriver.SeverityKey || c.key == slogdriver.MessageKey
			})
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("calls = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReplaceAttr_ShouldReceiveBuiltinKeysAfterConversion(t *testing.T) {
	var calls []replaceAttrCall
	var values = map[string]slog.Value{}
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			calls = append(calls, newReplaceAttrCall(groups, a))
			values[a.Key] = a.Value
			return a
		},
	})

	logger.Warn("hello")

	want := []replaceAttrCall{{key: slog.TimeKey}, {key: slogdriver.SeverityKey}, {key: slogdriver.MessageKey}}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %v, want %v", calls, want)
	}
	if got := values[slogdriver.SeverityKey].String(); got != "WARNING" {
		t.Errorf("severity = %q, want WARNING", got)
	}
}

func TestReplaceAttr_ShouldNotBeCalledForEmptyMessage(t *testing.T) {
	var calls []replaceAttrCall
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{ReplaceAttr: recordReplaceAttr(&calls)})

	logger.Info("")

	if slices.ContainsFunc(calls, func(c replaceAttrCall) bool { return c.key == slogdriver.MessageKey || c.key == slog.MessageKey }) {
		t.Errorf("ReplaceAttr was called for the empty message: %v", calls)
	}
}

func TestReplaceAttr_UserLevelAndMsgAttrsShouldBeUnwrapped(t *testing.T) {
	var values = map[string]any{}
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			values[a.Key] = a.Value.Any()
			return a
		},
	})

	logger.Info("hello", slog.String(slog.LevelKey, "user-level"), slog.String(slog.MessageKey, "user-msg"))

	if values[slog.LevelKey] != "user-level" {
		t.Errorf("level = %#v, want user-level", values[slog.LevelKey])
	}
	if values[slog.MessageKey] != "user-msg" {
		t.Errorf("msg = %#v, want user-msg", values[slog.MessageKey])
	}
}

func TestReplaceAttr_SpecialKeysShouldBeTopLevelUnderWithGroup(t *testing.T) {
	var calls []replaceAttrCall
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ProjectID:   "my-project",
		AddSource:   true,
		ReplaceAttr: recordReplaceAttr(&calls),
	})

	logger.WithGroup("g").Info("hello",
		slogdriver.MakeHTTPAttrFromHTTPPayload(slogdriver.HTTPPayload{RequestMethod: "GET"}),
		slog.String(slogdriver.InsertIDKey, "id"),
		slog.Group(slogdriver.OperationKey, slog.String("id", "op")),
		slog.String(slogdriver.TraceKey, "trace"),
		slog.String(slogdriver.SpanIDKey, "span"),
		slog.Group(slogdriver.LabelKey, slog.String("l", "v")),
	)

	want := []replaceAttrCall{
		{key: slog.TimeKey},
		{key: slogdriver.SeverityKey},
		{key: slogdriver.MessageKey},
		{groups: []string{slogdriver.LabelKey}, key: "l"},
		{key: slogdriver.HTTPKey},
		{key: slogdriver.InsertIDKey},
		// slog never calls ReplaceAttr for a group, but for the attrs in it.
		{groups: []string{slogdriver.OperationKey}, key: "id"},
		{key: slogdriver.TraceKey},
		{key: slogdriver.SpanIDKey},
		{key: slogdriver.SourceLocationKey},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %v\nwant %v", calls, want)
	}
}

func TestReplaceAttr_ShouldRemoveSpecialKeys(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slogdriver.SeverityKey, slogdriver.SourceLocationKey, slogdriver.HTTPKey, slog.TimeKey:
				return slog.Attr{}
			}
			return a
		},
	})

	logger.Info("hello", slogdriver.MakeHTTPAttrFromHTTPPayload(slogdriver.HTTPPayload{RequestMethod: "GET"}))

	got := decodeLine(t, &buf)
	want := map[string]any{"message": "hello"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestReplaceAttr_ShouldReplaceLabelValues(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		DefaultLabels: []slog.Attr{slog.String("token", "secret")},
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 1 && groups[0] == slogdriver.LabelKey && a.Key == "token" {
				return slog.String(a.Key, "***")
			}
			return a
		},
	})

	logger.Info("hello", slog.Group(slogdriver.LabelKey, slog.Int("n", 1)))

	got := decodeLine(t, &buf)
	want := map[string]any{"token": "***", "n": "1"}
	if !reflect.DeepEqual(got[slogdriver.LabelKey], want) {
		t.Errorf("labels = %v, want %v", got[slogdriver.LabelKey], want)
	}
}

func TestReplaceAttr_ShouldReceiveErrorReportingKeys(t *testing.T) {
	var calls []replaceAttrCall
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ErrorReporting: &slogdriver.ErrorReportingOptions{Service: "s"},
		ReplaceAttr:    recordReplaceAttr(&calls),
	})

	logger.Error("failed")

	want := []replaceAttrCall{
		{key: slog.TimeKey},
		{key: slogdriver.SeverityKey},
		{key: "@type"},
		{key: "serviceContext"},
		{groups: []string{"context"}, key: "reportLocation"},
		{key: "stack_trace"},
	}
	// The message is also passed to build the first line of stack_trace; see TestReplaceAttr_StackTraceFirstLineShouldBeReplaced.
	got := slices.DeleteFunc(calls, func(c replaceAttrCall) bool { return c.key == slogdriver.MessageKey })
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v\nwant %v", got, want)
	}
}

func TestReplaceAttr_StackTraceFirstLineShouldBeReplaced(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ErrorReporting: &slogdriver.ErrorReportingOptions{Service: "s"},
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch {
			case a.Key == slogdriver.MessageKey && len(groups) == 0:
				return slog.String(a.Key, strings.ReplaceAll(a.Value.String(), "secret-user", "***"))
			case a.Key == "err" && slices.Equal(groups, []string{"g"}):
				return slog.String(a.Key, "masked error")
			}
			return a
		},
	})

	logger.WithGroup("g").Error("failed for secret-user", slog.Any("err", errors.New("token secret-token is invalid")))

	got := decodeLine(t, &buf)
	stack, _ := got["stack_trace"].(string)
	firstLine, _, _ := strings.Cut(stack, "\n")
	if want := "failed for ***: masked error"; firstLine != want {
		t.Errorf("first line of stack_trace = %q, want %q", firstLine, want)
	}
	if strings.Contains(buf.String(), "secret") {
		t.Errorf("output has a secret: %s", buf.String())
	}
}

func TestReplaceAttr_RemovedErrorShouldNotBeInStackTrace(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ErrorReporting: &slogdriver.ErrorReportingOptions{Service: "s"},
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == "err" {
				return slog.Attr{}
			}
			return a
		},
	})

	logger.Error("failed", slog.Any("err", errors.New("secret")))

	if strings.Contains(buf.String(), "secret") {
		t.Errorf("output has the removed error: %s", buf.String())
	}
}

func TestReplaceAttr_RemovingAllLabelsShouldOmitLabels(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if slices.Equal(groups, []string{slogdriver.LabelKey}) {
				return slog.Attr{}
			}
			return a
		},
	})

	logger.Info("hello", slog.Group(slogdriver.LabelKey, slog.String("l", "v")))

	got := decodeLine(t, &buf)
	if _, ok := got[slogdriver.LabelKey]; ok {
		t.Errorf("labels should be omitted: %v", got)
	}
}

func TestReplaceAttr_EscapedErrorKeyShouldNotBeInStackTrace(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ErrorReporting: &slogdriver.ErrorReportingOptions{Service: "s"},
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// A top-level user attr named MessageKey is written as "fields.message".
			if a.Key == "fields."+slogdriver.MessageKey {
				return slog.Attr{}
			}
			return a
		},
	})

	logger.Error("failed", slog.Any(slogdriver.MessageKey, errors.New("secret")))

	if strings.Contains(buf.String(), "secret") {
		t.Errorf("output has the removed error: %s", buf.String())
	}
}
