package logging

import (
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
)

func NewLogger(level slog.Level) *slog.Logger {
	opts := &tint.Options{
		Level:      level,
		TimeFormat: time.RFC3339,
		NoColor:    true,
	}

	if isInteractive() {
		opts.TimeFormat = time.TimeOnly
		opts.NoColor = false
	}

	return slog.New(tint.NewHandler(os.Stdout, opts))
}

func NoopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func isInteractive() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}
