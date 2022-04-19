package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type contextKey struct{}

var key = &contextKey{}

func Init(config zap.Config, opts ...zap.Option) error {
	log, err := config.Build(append(opts, zap.AddCallerSkip(1))...)
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
	Logger(ctx).Debug(msg, fields...)
}

func Info(ctx context.Context, msg string, fields ...zap.Field) {
	Logger(ctx).Info(msg, fields...)
}

func Warn(ctx context.Context, msg string, fields ...zap.Field) {
	Logger(ctx).Warn(msg, fields...)
}

func Error(ctx context.Context, msg string, fields ...zap.Field) {
	Logger(ctx).Error(msg, fields...)
}

func Fatal(ctx context.Context, msg string, fields ...zap.Field) {
	Logger(ctx).Fatal(msg, fields...)
}

func Check(ctx context.Context, level zapcore.Level, msg string, fields ...zap.Field) {
	Logger(ctx).Check(level, msg).Write(fields...)
}
