package slogdriver

import (
	"bytes"
	"cmp"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

const reportedErrorEventType = "type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent"

// ErrorReportingOptions configures how logs at slog.LevelError or above are formatted
// so that Cloud Error Reporting groups them as errors.
// See https://cloud.google.com/error-reporting/docs/formatting-error-messages
type ErrorReportingOptions struct {
	// Service is the name of the service that reported the error.
	// If empty, K_SERVICE (set by Cloud Run) is used, and then the executable name.
	Service string

	// Version is the version of the service that reported the error.
	// If empty, K_REVISION (set by Cloud Run) is used. It is omitted when both are empty.
	Version string
}

type serviceContext struct {
	Service string `json:"service"`
	Version string `json:"version,omitempty"`
}

type reportLocation struct {
	FilePath     string `json:"filePath"`
	LineNumber   int    `json:"lineNumber"`
	FunctionName string `json:"functionName"`
}

func newServiceContext(opts *ErrorReportingOptions) serviceContext {
	return serviceContext{
		Service: cmp.Or(opts.Service, os.Getenv("K_SERVICE"), filepath.Base(os.Args[0])),
		Version: cmp.Or(opts.Version, os.Getenv("K_REVISION")),
	}
}

// errorReportingAttrs must be called from Handle, because the stack trace is taken from the current call stack.
func (c *cloudLoggingHandler) errorReportingAttrs(r slog.Record) []slog.Attr {
	attrs := []slog.Attr{
		slog.String("@type", reportedErrorEventType),
		slog.Any("serviceContext", *c.serviceContext),
	}
	if r.PC == 0 {
		return attrs
	}

	f := recordFrame(r)
	attrs = append(attrs, slog.Group("context", slog.Any("reportLocation", reportLocation{
		FilePath:     f.File,
		LineNumber:   f.Line,
		FunctionName: f.Function,
	})))
	if stack := stackFrom(r.PC); stack != "" {
		attrs = append(attrs, slog.String("stack_trace", errorSummary(r)+"\n\n"+stack))
	}
	return attrs
}

// errorSummary returns the first line of stack_trace, which Error Reporting shows as the error message.
func errorSummary(r slog.Record) string {
	var errMsg string
	r.Attrs(func(a slog.Attr) bool {
		if err, ok := a.Value.Resolve().Any().(error); ok {
			errMsg = err.Error()
			return false
		}
		return true
	})
	switch {
	case errMsg == "":
		return r.Message
	case r.Message == "":
		return errMsg
	default:
		return r.Message + ": " + errMsg
	}
}

// stackFrom formats the current call stack from the frame of pc in the format of runtime/debug.Stack,
// which Error Reporting parses as a Go stack trace.
// It returns "" if pc is not in the current call stack, e.g. when the record is handled in another goroutine.
func stackFrom(pc uintptr) string {
	pcs := make([]uintptr, 128)
	pcs = pcs[:runtime.Callers(0, pcs)]
	i := slices.Index(pcs, pc)
	if i < 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "goroutine %s [running]:\n", goroutineID())
	frames := runtime.CallersFrames(pcs[i:])
	for {
		f, more := frames.Next()
		// runtime/debug.Stack also hides runtime frames such as runtime.main and runtime.goexit.
		if !strings.HasPrefix(f.Function, "runtime.") {
			fmt.Fprintf(&b, "%s(...)\n\t%s:%d", f.Function, f.File, f.Line)
			if f.Entry != 0 {
				fmt.Fprintf(&b, " +0x%x", f.PC-f.Entry)
			}
			b.WriteByte('\n')
		}
		if !more {
			break
		}
	}
	return b.String()
}

func goroutineID() string {
	// The header of runtime.Stack is "goroutine N [running]:".
	buf := make([]byte, 64)
	buf = buf[:runtime.Stack(buf, false)]
	buf = bytes.TrimPrefix(buf, []byte("goroutine "))
	id, _, _ := bytes.Cut(buf, []byte(" "))
	return string(id)
}
