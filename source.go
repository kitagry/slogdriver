package slogdriver

import (
	"fmt"
	"log/slog"
	"runtime"
)

type LogEntrySourceLocation struct {
	File     string `json:"file"`
	Line     string `json:"line"`
	Function string `json:"function"`
}

func makeSourceLocationAttr(f runtime.Frame) slog.Attr {
	return slog.Any(SourceLocationKey, LogEntrySourceLocation{
		File:     f.File,
		Line:     fmt.Sprint(f.Line),
		Function: f.Function,
	})
}

func recordFrame(r slog.Record) runtime.Frame {
	fs := runtime.CallersFrames([]uintptr{r.PC})
	f, _ := fs.Next()
	return f
}
