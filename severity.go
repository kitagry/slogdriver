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

func levelStringToSeverity(s string) string {
	switch s {
	case LevelDefault.String():
		return "DEFAULT"
	case LevelDebug.String():
		return "DEBUG"
	case LevelInfo.String():
		return "INFO"
	case LevelNotice.String():
		return "NOTICE"
	case LevelWarning.String():
		return "WARNING"
	case LevelError.String():
		return "ERROR"
	case LevelCritical.String():
		return "CRITICAL"
	case LevelAlert.String():
		return "ALERT"
	case LevelEmergency.String():
		return "EMERGENCY"
	default:
		return ""
	}
}
