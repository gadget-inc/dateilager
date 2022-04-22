package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var hooks []Hook

// Hook is a function that gets called right before a message is logged.
type Hook = func(ctx context.Context, level zapcore.Level, msg string, fields ...zap.Field)

func AddHook(fn Hook) {
	hooks = append(hooks, fn)
}
