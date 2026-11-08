package slogdriver

import "log/slog"

const (
	LevelDefault   slog.Level = slog.LevelDebug - 2
	LevelDebug     slog.Level = slog.LevelDebug
	LevelInfo      slog.Level = slog.LevelInfo
	LevelNotice    slog.Level = slog.LevelWarn - 2
	LevelWarning   slog.Level = slog.LevelWarn
	LevelError     slog.Level = slog.LevelError
	LevelCritical  slog.Level = slog.LevelError + 2
	LevelAlert     slog.Level = slog.LevelError + 4
	LevelEmergency slog.Level = slog.LevelError + 6
)

// levelToSeverity maps a level to the nearest defined severity at or below it,
// because slog allows arbitrary levels such as slog.LevelInfo+1.
func levelToSeverity(l slog.Level) string {
	switch {
	case l >= LevelEmergency:
		return "EMERGENCY"
	case l >= LevelAlert:
		return "ALERT"
	case l >= LevelCritical:
		return "CRITICAL"
	case l >= LevelError:
		return "ERROR"
	case l >= LevelWarning:
		return "WARNING"
	case l >= LevelNotice:
		return "NOTICE"
	case l >= LevelInfo:
		return "INFO"
	case l >= LevelDebug:
		return "DEBUG"
	default:
		return "DEFAULT"
	}
}
