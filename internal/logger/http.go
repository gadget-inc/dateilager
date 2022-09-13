package logger

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

func Middleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		ctx := r.Context()
		ctx = context.WithValue(ctx, key, Logger(ctx).With(
			zap.String("http.proto", r.Proto),
			zap.String("http.method", r.Method),
			zap.String("http.path", r.URL.Path),
			zap.String("http.request_id", middleware.GetReqID(ctx)),
		))

		start := time.Now()
		next.ServeHTTP(ww, r.WithContext(ctx))
		duration := time.Since(start)

		lvl := zap.ErrorLevel
		switch {
		case ww.Status() < 400:
			lvl = zap.InfoLevel
		case ww.Status() < 500:
			lvl = zap.WarnLevel
		}

		Log(ctx, lvl, "finished request",
			zap.String("http.status", http.StatusText(ww.Status())),
			zap.Int("http.code", ww.Status()),
			zap.Duration("http.duration", duration),
			zap.Int("http.size", ww.BytesWritten()),
		)
	}

	return http.HandlerFunc(fn)
}
