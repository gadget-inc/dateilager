package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type contextKey struct{}

var key = &contextKey{}

func Init(config zap.Config, opts ...zap.Option) error {
	log, err := config.Build(append(opts, zap.AddCallerSkip(2))...)
	if err != nil {
		return err
	}
	zap.ReplaceGlobals(log)
	return nil
}

func Logger(ctx context.Context) *zap.Logger {
	// check if it's in the context
	if log, ok := ctx.Value(key).(*zap.Logger); ok {
		return log
	}
	// otherwise, use global
	return zap.L()
}

func Sync() error {
	return zap.L().Sync()
}

func Debug(ctx context.Context, msg string, fields ...zap.Field) {
	Write(ctx, zapcore.DebugLevel, msg, fields...)
}

func Info(ctx context.Context, msg string, fields ...zap.Field) {
	Write(ctx, zapcore.InfoLevel, msg, fields...)
}

func Warn(ctx context.Context, msg string, fields ...zap.Field) {
	Write(ctx, zapcore.WarnLevel, msg, fields...)
}

func Error(ctx context.Context, msg string, fields ...zap.Field) {
	Write(ctx, zapcore.ErrorLevel, msg, fields...)
}

func Fatal(ctx context.Context, msg string, fields ...zap.Field) {
	Write(ctx, zapcore.FatalLevel, msg, fields...)
}

func Write(ctx context.Context, level zapcore.Level, msg string, fields ...zap.Field) {
	for _, hook := range hooks {
		hook(ctx, level, msg, fields...)
	}
	Logger(ctx).Check(level, msg).Write(fields...)
}

func With(ctx context.Context, fields ...zap.Field) context.Context {
	return context.WithValue(ctx, key, Logger(ctx).With(fields...))
}
