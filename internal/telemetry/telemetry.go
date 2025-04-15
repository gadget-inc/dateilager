package telemetry

import (
	"context"
	"time"

	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/version"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var tracer = otel.Tracer("github.com/gadget-inc/dateilager")

type Type int

const (
	Server Type = iota
	Client
)

func (t Type) String() string {
	switch t {
	case Client:
		return "client"
	case Server:
		return "server"
	default:
		return "unknown"
	}
}

func Init(ctx context.Context, t Type) func() {
	traceExporter, err := otlptrace.New(ctx, otlptracehttp.NewClient())
	if err != nil {
		logger.Error(ctx, "failed to initialize telemetry", zap.Error(err))
		return func() {}
	}

	resourceOptions := []resource.Option{
		resource.WithContainer(),
		resource.WithFromEnv(),
		resource.WithHost(),
		resource.WithOS(),
		resource.WithProcessExecutableName(),
		resource.WithProcessExecutablePath(),
		resource.WithProcessOwner(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
		resource.WithProcessRuntimeDescription(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String("dateilager."+t.String()),
			semconv.ServiceNamespaceKey.String("dateilager"),
			semconv.ServiceVersionKey.String(version.Version)),
	}

	// only add command line args for clients
	if t == Client {
		resourceOptions = append(resourceOptions, resource.WithProcessCommandArgs())
	}

	res, err := resource.New(ctx, resourceOptions...)
	if err != nil {
		logger.Error(ctx, "failed to initialize telemetry", zap.Error(err))
		return func() {}
	}

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithSampler(sdktrace.ParentBased(sampler{})),
	)

	otel.SetTracerProvider(traceProvider)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()

		err := traceProvider.Shutdown(ctx)
		if err != nil {
			logger.Error(ctx, "failed to shutdown telemetry", zap.Error(err))
		}
	}
}

func Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return tracer.Start(ctx, spanName, opts...)
}

func Trace(ctx context.Context, spanName string, fn func(context.Context, trace.Span) error) error {
	ctx, span := Start(ctx, spanName)
	defer span.End()

	return fn(ctx, span)
}
