package telemetry

import (
	"context"
	"fmt"
	"math"
	"os"
	"reflect"
	"time"

	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/version"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	tracer = otel.Tracer("github.com/gadget-inc/dateilager")
)

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

func Init(ctx context.Context, t Type) (shutdown func(), err error) {
	endpoint := os.Getenv("OTEL_COLLECTOR_TRACE_ENDPOINT")
	if endpoint == "" {
		return func() {}, nil
	}

	// FIXME: Make this secure
	client := otlptracehttp.NewClient(
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)

	traceExporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	resourceOptions := []resource.Option{
		resource.WithSchemaURL(semconv.SchemaURL),
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
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter),
	)

	otel.SetTracerProvider(traceProvider)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	logger.AddHook(func(ctx context.Context, level zapcore.Level, msg string, fields ...zap.Field) {
		span := trace.SpanFromContext(ctx)
		if !span.IsRecording() {
			return
		}

		if level >= zapcore.ErrorLevel {
			span.SetStatus(codes.Error, msg)
		}

		attrs := []attribute.KeyValue{
			attribute.Stringer("log.level", level),
			attribute.String("log.message", msg),
		}

		// ported from https://github.com/uptrace/opentelemetry-go-extra/blob/086241ab069b10060e01699e2944834a37d29fbb/otelzap/otelzap.go#L651
		for _, field := range fields {
			switch field.Type {
			case zapcore.BoolType:
				attrs = append(attrs, attribute.Bool(field.Key, field.Integer == 1))

			case zapcore.Int8Type, zapcore.Int16Type, zapcore.Int32Type, zapcore.Int64Type,
				zapcore.Uint32Type, zapcore.Uint8Type, zapcore.Uint16Type, zapcore.Uint64Type,
				zapcore.UintptrType:
				attrs = append(attrs, attribute.Int64(field.Key, field.Integer))

			case zapcore.Float32Type, zapcore.Float64Type:
				attrs = append(attrs, attribute.Float64(field.Key, math.Float64frombits(uint64(field.Integer))))

			case zapcore.BinaryType, zapcore.ByteStringType:
				attrs = append(attrs, attribute.String(field.Key, string(field.Interface.([]byte))))

			case zapcore.StringType:
				attrs = append(attrs, attribute.String(field.Key, field.String))

			case zapcore.StringerType:
				attrs = append(attrs, attribute.Stringer(field.Key, field.Interface.(fmt.Stringer)))

			case zapcore.DurationType, zapcore.TimeType:
				attrs = append(attrs, attribute.Int64(field.Key, field.Integer))

			case zapcore.TimeFullType:
				attrs = append(attrs, attribute.Int64(field.Key, field.Interface.(time.Time).UnixNano()))

			case zapcore.ErrorType:
				err := field.Interface.(error)
				attrs = append(attrs, semconv.ExceptionTypeKey.String(reflect.TypeOf(err).String()))
				attrs = append(attrs, semconv.ExceptionMessageKey.String(err.Error()))
				span.RecordError(err)
			}
		}

		span.AddEvent("log", trace.WithAttributes(attrs...))
	})

	return func() {
		_ = traceProvider.Shutdown(ctx)
	}, nil
}

func Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return tracer.Start(ctx, spanName, opts...)
}

func Wrap(ctx context.Context, spanName string, fn func(context.Context, trace.Span) error) error {
	ctx, span := Start(ctx, spanName)
	defer span.End()

	return fn(ctx, span)
}
