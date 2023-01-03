package slogdriver

import (
	"fmt"

	"golang.org/x/exp/slog"
)

type LogEntrySourceLocation struct {
	File string `json:"file"`
	Line string `json:"line"`
}

func (c *cloudLoggingHandler) makeSourceLocationAttr(r slog.Record) slog.Attr {
	f, line := r.SourceLine()
	return slog.Any(SourceLocationKey, LogEntrySourceLocation{
		File: f,
		Line: fmt.Sprint(line),
	})
}
