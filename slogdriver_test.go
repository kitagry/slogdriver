package slogdriver_test

import (
	"os"

	"github.com/kitagry/slogdriver"
	"golang.org/x/exp/slog"
)

func ExampleNew() {
	logger := slogdriver.New(os.Stdout, slogdriver.HandlerOptions{AddSource: true})
	logger = logger.With(slog.Group(slogdriver.LabelKey, slog.String("commonLabel", "hoge")))
	logger.Info("Hello World", slog.Group(slogdriver.LabelKey, slog.String("specifiedLabel", "fuga")))
}
