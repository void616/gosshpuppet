package logging

import (
	"context"
	"log/slog"
)

type loggerContextKey struct{}

func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

func NewContextWith(ctx context.Context, args ...any) context.Context {
	return NewContext(ctx, FromContext(ctx).With(args...))
}

func NewContextGroupWith(ctx context.Context, group string, args ...any) context.Context {
	return NewContext(ctx, FromContext(ctx).WithGroup(group).With(args...))
}

func FromContext(ctx context.Context) *slog.Logger {
	if v := ctx.Value(loggerContextKey{}); v != nil {
		if logger, ok := v.(*slog.Logger); ok {
			return logger
		}
	}
	return NoopLogger()
}
