package telemetry

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gadget-inc/dateilager/pkg/version"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
)

var (
	Tracer = otel.Tracer("github.com/gadget-inc/dateilager")
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
	endpoint := os.Getenv("DL_OTEL_COLLECTOR_TRACE_ENDPOINT")
	if endpoint == "" {
		return func() {}, nil
	}

	var client otlptrace.Client
	if strings.HasPrefix(endpoint, "http") {
		client = otlptracehttp.NewClient(
			otlptracehttp.WithEndpoint(endpoint),
		)
	} else {
		client = otlptracegrpc.NewClient(
			otlptracegrpc.WithEndpoint(endpoint),
			otlptracegrpc.WithInsecure(),
		)
	}

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

	return func() {
		_ = traceProvider.Shutdown(ctx)
	}, nil
}

// AttributeInt64p is a wrapper around attribute.Int64 that accepts a pointer.
func AttributeInt64p(k string, v *int64) attribute.KeyValue {
	if v == nil {
		return attribute.String(k, "nil")
	}
	return attribute.Int64(k, *v)
}
