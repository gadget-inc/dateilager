package telemetry

import (
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type sampler struct{}

func (s sampler) ShouldSample(params sdktrace.SamplingParameters) sdktrace.SamplingResult {
	psc := trace.SpanContextFromContext(params.ParentContext)
	if params.Name == "grpc.health.v1.Health/Check" {
		return sdktrace.SamplingResult{
			Decision:   sdktrace.Drop,
			Tracestate: psc.TraceState(),
		}
	}

	return sdktrace.SamplingResult{
		Decision:   sdktrace.RecordAndSample,
		Tracestate: psc.TraceState(),
	}
}

func (s sampler) Description() string {
	return "DateilagerSampler"
}
