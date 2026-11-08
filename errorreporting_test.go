package slogdriver_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/kitagry/slogdriver"
)

const reportedErrorEventType = "type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent"

type reportLocation struct {
	FilePath     string `json:"filePath"`
	LineNumber   int    `json:"lineNumber"`
	FunctionName string `json:"functionName"`
}

type errorReportingEntry struct {
	Type           string            `json:"@type"`
	ServiceContext map[string]string `json:"serviceContext"`
	Context        struct {
		ReportLocation reportLocation `json:"reportLocation"`
	} `json:"context"`
	StackTrace string `json:"stack_trace"`
}

func decodeErrorReportingEntry(t *testing.T, buf *bytes.Buffer) errorReportingEntry {
	t.Helper()
	var got errorReportingEntry
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode %q: %v", buf.String(), err)
	}
	return got
}

// callerLocation returns the location of the line that calls it, offset by delta lines.
func callerLocation(t *testing.T, delta int) reportLocation {
	t.Helper()
	pc, file, line, _ := runtime.Caller(1)
	return reportLocation{
		FilePath:     file,
		LineNumber:   line + delta,
		FunctionName: runtime.FuncForPC(pc).Name(),
	}
}

func TestErrorReporting_ErrorLogIsReportedErrorEvent(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ErrorReporting: &slogdriver.ErrorReportingOptions{Service: "my-service", Version: "v1"},
	})

	want := callerLocation(t, 1)
	logger.Error("something failed")

	got := decodeErrorReportingEntry(t, &buf)
	if got.Type != reportedErrorEventType {
		t.Errorf("@type = %q, want %q", got.Type, reportedErrorEventType)
	}
	if wantSC := map[string]string{"service": "my-service", "version": "v1"}; !reflect.DeepEqual(got.ServiceContext, wantSC) {
		t.Errorf("serviceContext = %v, want %v", got.ServiceContext, wantSC)
	}
	if got.Context.ReportLocation != want {
		t.Errorf("reportLocation = %+v, want %+v", got.Context.ReportLocation, want)
	}
}

func TestErrorReporting_LevelsAboveErrorAreReported(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ErrorReporting: &slogdriver.ErrorReportingOptions{Service: "my-service"},
	})

	logger.Log(t.Context(), slogdriver.LevelCritical, "critical")

	if got := decodeErrorReportingEntry(t, &buf); got.Type != reportedErrorEventType {
		t.Errorf("@type = %q, want %q", got.Type, reportedErrorEventType)
	}
}

func TestErrorReporting_LevelsBelowErrorAreNotChanged(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ErrorReporting: &slogdriver.ErrorReportingOptions{Service: "my-service"},
	})

	logger.Warn("warning", slog.String("key", "value"))

	want := `{"severity":"WARNING","message":"warning","key":"value"}`
	if got := strings.TrimSpace(removeTime(t, buf.Bytes())); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestErrorReporting_NilOptionsDoNotChangeErrorLog(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{})

	logger.Error("something failed")

	want := `{"severity":"ERROR","message":"something failed"}`
	if got := strings.TrimSpace(removeTime(t, buf.Bytes())); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestErrorReporting_ServiceContextDefaults(t *testing.T) {
	tests := map[string]struct {
		opts      slogdriver.ErrorReportingOptions
		kService  string
		kRevision string
		want      map[string]string
	}{
		"options take precedence over Cloud Run environment variables": {
			opts:      slogdriver.ErrorReportingOptions{Service: "opt-service", Version: "opt-version"},
			kService:  "env-service",
			kRevision: "env-revision",
			want:      map[string]string{"service": "opt-service", "version": "opt-version"},
		},
		"Cloud Run environment variables fill empty options": {
			kService:  "env-service",
			kRevision: "env-revision",
			want:      map[string]string{"service": "env-service", "version": "env-revision"},
		},
		"executable name is used when no service is given and version is omitted": {
			want: map[string]string{"service": filepath.Base(os.Args[0])},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("K_SERVICE", tt.kService)
			t.Setenv("K_REVISION", tt.kRevision)
			var buf bytes.Buffer
			logger := slogdriver.New(&buf, slogdriver.HandlerOptions{ErrorReporting: &tt.opts})

			logger.Error("something failed")

			if got := decodeErrorReportingEntry(t, &buf).ServiceContext; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("serviceContext = %v, want %v", got, tt.want)
			}
		})
	}
}

// goStackTrace matches the format of runtime/debug.Stack, which Error Reporting parses as a Go stack trace.
var goStackTrace = regexp.MustCompile(`^goroutine \d+ \[running\]:\n(\S+\(.*\)\n\t\S+:\d+( \+0x[0-9a-f]+)?\n)+$`)

func TestErrorReporting_StackTraceIsGoFormatStartingAtCaller(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ErrorReporting: &slogdriver.ErrorReportingOptions{Service: "my-service"},
	})

	want := callerLocation(t, 1)
	logger.Error("something failed")

	// Same layout as a panic: the message, a blank line, then the goroutine trace.
	firstLine, stack, ok := strings.Cut(decodeErrorReportingEntry(t, &buf).StackTrace, "\n\n")
	if !ok {
		t.Fatalf("stack_trace has no blank line after the message: %q", firstLine)
	}
	if firstLine != "something failed" {
		t.Errorf("first line of stack_trace = %q, want the message", firstLine)
	}
	if !goStackTrace.MatchString(stack) {
		t.Fatalf("stack_trace is not in Go format:\n%s", stack)
	}
	// The first frame must be the log call site, not slog or slogdriver internals.
	frames := strings.Split(stack, "\n")
	if wantFunc := want.FunctionName + "(...)"; frames[1] != wantFunc {
		t.Errorf("first frame function = %q, want %q", frames[1], wantFunc)
	}
	if wantFile := fmt.Sprintf("\t%s:%d ", want.FilePath, want.LineNumber); !strings.HasPrefix(frames[2], wantFile) {
		t.Errorf("first frame location = %q, want prefix %q", frames[2], wantFile)
	}
}

func TestErrorReporting_StackTraceFirstLineHasErrorAttr(t *testing.T) {
	tests := map[string]struct {
		msg  string
		want string
	}{
		"message and error are joined":     {msg: "failed to save user", want: "failed to save user: connection refused"},
		"error only when message is empty": {msg: "", want: "connection refused"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
				ErrorReporting: &slogdriver.ErrorReportingOptions{Service: "my-service"},
			})

			logger.Error(tt.msg, slog.Any("error", errors.New("connection refused")))

			firstLine, _, _ := strings.Cut(decodeErrorReportingEntry(t, &buf).StackTrace, "\n")
			if firstLine != tt.want {
				t.Errorf("first line of stack_trace = %q, want %q", firstLine, tt.want)
			}
		})
	}
}

func TestErrorReporting_FieldsAreTopLevelUnderGroup(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ErrorReporting: &slogdriver.ErrorReportingOptions{Service: "my-service"},
	}).WithGroup("group")

	logger.Error("something failed", slog.String("key", "value"))

	got := decodeErrorReportingEntry(t, &buf)
	if got.Type != reportedErrorEventType {
		t.Errorf("@type = %q, want %q", got.Type, reportedErrorEventType)
	}
	if got.ServiceContext["service"] != "my-service" {
		t.Errorf("serviceContext = %v, want service my-service", got.ServiceContext)
	}
	if got.StackTrace == "" {
		t.Errorf("stack_trace is empty")
	}
}

type pointerError struct{ msg string }

func (e *pointerError) Error() string { return e.msg }

func TestErrorReporting_TypedNilErrorDoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ErrorReporting: &slogdriver.ErrorReportingOptions{Service: "my-service"},
	})
	var err *pointerError

	logger.Error("something failed", slog.Any("error", err))

	firstLine, _, _ := strings.Cut(decodeErrorReportingEntry(t, &buf).StackTrace, "\n")
	if want := "something failed: <nil>"; firstLine != want {
		t.Errorf("first line of stack_trace = %q, want %q", firstLine, want)
	}
}

func logAtDepth(logger *slog.Logger, depth int) {
	if depth == 0 {
		logger.Error("deep")
		return
	}
	logAtDepth(logger, depth-1)
}

func TestErrorReporting_DeepStackIsNotTruncated(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ErrorReporting: &slogdriver.ErrorReportingOptions{Service: "my-service"},
	})

	logAtDepth(logger, 300)

	stack := decodeErrorReportingEntry(t, &buf).StackTrace
	if got := strings.Count(stack, "logAtDepth(...)"); got != 301 {
		t.Errorf("logAtDepth frames = %d, want 301", got)
	}
}

func removeTime(t *testing.T, b []byte) string {
	t.Helper()
	return string(regexp.MustCompile(`"time":"[^"]*",`).ReplaceAll(b, nil))
}

func TestErrorReporting_StackTraceStartsWithGoroutineWhenNoMessageAndNoError(t *testing.T) {
	var buf bytes.Buffer
	logger := slogdriver.New(&buf, slogdriver.HandlerOptions{
		ErrorReporting: &slogdriver.ErrorReportingOptions{Service: "my-service"},
	})

	logger.Error("")

	if stack := decodeErrorReportingEntry(t, &buf).StackTrace; !goStackTrace.MatchString(stack) {
		t.Errorf("stack_trace is not in Go format:\n%q", stack)
	}
}
