package slogdriver

import (
	"fmt"
	"runtime"

	"golang.org/x/exp/slog"
)

type LogEntrySourceLocation struct {
	File     string `json:"file"`
	Line     string `json:"line"`
	Function string `json:"function"`
}

func (c *cloudLoggingHandler) makeSourceLocationAttr(r slog.Record) slog.Attr {
	fs := runtime.CallersFrames([]uintptr{r.PC})
	f, _ := fs.Next()
	return slog.Any(SourceLocationKey, LogEntrySourceLocation{
		File:     f.File,
		Line:     fmt.Sprint(f.Line),
		Function: f.Function,
	})
}
