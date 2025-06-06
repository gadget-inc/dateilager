package logger

import (
	"context"
	stdlog "log"

	"github.com/gadget-inc/dateilager/internal/environment"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/klog/v2"
)

type contextKey struct{}

var key = &contextKey{}

func Init(env environment.Env, encoding string, level zap.AtomicLevel) error {
	var config zap.Config
	if env == environment.Prod {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
	}

	config.Encoding = encoding
	config.Level = level
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	log, err := config.Build(zap.AddCallerSkip(2))
	if err != nil {
		return err
	}

	zap.ReplaceGlobals(log)
	klog.SetLogger(zapr.NewLogger(log))
	stdlog.SetOutput(zap.NewStdLog(log).Writer())

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

func Sync(ctx context.Context) error {
	return Logger(ctx).Sync()
}

func Debug(ctx context.Context, msg string, fields ...zap.Field) {
	write(ctx, zapcore.DebugLevel, msg, fields...)
}

func Info(ctx context.Context, msg string, fields ...zap.Field) {
	write(ctx, zapcore.InfoLevel, msg, fields...)
}

func Warn(ctx context.Context, msg string, fields ...zap.Field) {
	write(ctx, zapcore.WarnLevel, msg, fields...)
}

func Error(ctx context.Context, msg string, fields ...zap.Field) {
	write(ctx, zapcore.ErrorLevel, msg, fields...)
}

func Fatal(ctx context.Context, msg string, fields ...zap.Field) {
	write(ctx, zapcore.FatalLevel, msg, fields...)
}

func Log(ctx context.Context, level zapcore.Level, msg string, fields ...zap.Field) {
	write(ctx, level, msg, fields...)
}

func With(ctx context.Context, fields ...zap.Field) context.Context {
	return context.WithValue(ctx, key, Logger(ctx).With(fields...))
}

// write is a helper function that writes the log entry.
//
// This function shouldn't be called directly because the logger is configured to skip two stack frames.
// Instead, use one of the exported functions above.
func write(ctx context.Context, level zapcore.Level, msg string, fields ...zap.Field) {
	for _, hook := range hooks {
		hook(ctx, level, msg, fields...)
	}
	Logger(ctx).Check(level, msg).Write(fields...)
}
