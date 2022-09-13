package telemetry

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"go.opentelemetry.io/otel/trace"
)

func NewQueryTracer() pgx.QueryTracer {
	return &pgxTracer{}
}

type pgxTracer struct{}

// make sure the tracer implements the following interfaces
var (
	_ pgx.BatchTracer    = (*pgxTracer)(nil)
	_ pgx.ConnectTracer  = (*pgxTracer)(nil)
	_ pgx.CopyFromTracer = (*pgxTracer)(nil)
	_ pgx.PrepareTracer  = (*pgxTracer)(nil)
	_ pgx.QueryTracer    = (*pgxTracer)(nil)
)

func (t *pgxTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return t.start(ctx, "pgx.query", conn.Config(), semconv.DBStatementKey.String(strings.Trim(data.SQL, " ")))
}

func (t *pgxTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	t.end(ctx, data.Err, attribute.String("pgx.command-tag", data.CommandTag.String()))
}

func (t *pgxTracer) TraceBatchStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceBatchStartData) context.Context {
	return t.start(ctx, "pgx.batch", conn.Config(), attribute.Int("pgx.batch.size", data.Batch.Len()))
}

func (t *pgxTracer) TraceBatchQuery(ctx context.Context, conn *pgx.Conn, data pgx.TraceBatchQueryData) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	attrs := []attribute.KeyValue{
		semconv.DBStatementKey.String(strings.Trim(data.SQL, " ")),
		attribute.String("pgx.command-tag", data.CommandTag.String()),
	}

	if data.Err != nil {
		span.RecordError(data.Err, trace.WithAttributes(t.attrs(conn.Config(), attrs...)...))
		span.SetStatus(codes.Error, data.Err.Error())
	} else {
		span.AddEvent("pgx.batch-query", trace.WithAttributes(t.attrs(conn.Config(), attrs...)...))
	}
}

func (t *pgxTracer) TraceBatchEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceBatchEndData) {
	t.end(ctx, data.Err)
}

func (t *pgxTracer) TraceCopyFromStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceCopyFromStartData) context.Context {
	return t.start(ctx, "pgx.copy-from", conn.Config(),
		semconv.DBSQLTableKey.String(data.TableName.Sanitize()),
		attribute.StringSlice("db.sql.columns", data.ColumnNames),
	)
}

func (t *pgxTracer) TraceCopyFromEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceCopyFromEndData) {
	t.end(ctx, data.Err, attribute.String("pgx.command-tag", data.CommandTag.String()))
}

func (t *pgxTracer) TracePrepareStart(ctx context.Context, conn *pgx.Conn, data pgx.TracePrepareStartData) context.Context {
	return t.start(ctx, "pgx.prepare", conn.Config(), semconv.DBStatementKey.String(strings.Trim(data.SQL, " ")))
}

func (t *pgxTracer) TracePrepareEnd(ctx context.Context, conn *pgx.Conn, data pgx.TracePrepareEndData) {
	t.end(ctx, data.Err, attribute.Bool("pgx.already-prepared", data.AlreadyPrepared))
}

func (t *pgxTracer) TraceConnectStart(ctx context.Context, data pgx.TraceConnectStartData) context.Context {
	return t.start(ctx, "pgx.connect", data.ConnConfig)
}

func (t *pgxTracer) TraceConnectEnd(ctx context.Context, data pgx.TraceConnectEndData) {
	t.end(ctx, data.Err)
}

func (t *pgxTracer) attrs(config *pgx.ConnConfig, attributes ...attribute.KeyValue) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.DBSystemPostgreSQL,
		semconv.DBUserKey.String(config.User),
		semconv.DBNameKey.String(config.Database),
		semconv.NetPeerNameKey.String(config.Host),
		semconv.NetPeerPortKey.Int(int(config.Port)),
	}

	if len(attributes) > 0 {
		attrs = append(attrs, attributes...)
	}

	return attrs
}

func (t *pgxTracer) start(ctx context.Context, name string, config *pgx.ConnConfig, attrs ...attribute.KeyValue) context.Context {
	if !trace.SpanFromContext(ctx).IsRecording() {
		return ctx
	}

	ctx, _ = Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(t.attrs(config, attrs...)...),
	)

	return ctx
}

func (t *pgxTracer) end(ctx context.Context, err error, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
