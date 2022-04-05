package server

import (
	"context"
	"net"

	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/telemetry"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"go.opentelemetry.io/otel/trace"
)

type DbPoolConnector struct {
	pool             *pgxpool.Pool
	spanStartOptions []trace.SpanStartOption
}

func NewDbPoolConnector(ctx context.Context, uri string) (*DbPoolConnector, error) {
	pool, err := pgxpool.Connect(ctx, uri)
	if err != nil {
		return nil, err
	}

	traceAttributes := []attribute.KeyValue{
		semconv.DBSystemPostgreSQL,
		semconv.DBUserKey.String(pool.Config().ConnConfig.User),
		semconv.DBNameKey.String(pool.Config().ConnConfig.Database),
		semconv.NetPeerPortKey.Int(int(pool.Config().ConnConfig.Port)),
	}

	if ip := net.ParseIP(pool.Config().ConnConfig.Host); ip != nil {
		traceAttributes = append(traceAttributes, semconv.NetPeerIPKey.String(ip.String()))
	} else {
		traceAttributes = append(traceAttributes, semconv.NetPeerNameKey.String(pool.Config().ConnConfig.Host))
		if ip, err := net.LookupIP(pool.Config().ConnConfig.Host); err == nil {
			traceAttributes = append(traceAttributes, semconv.NetPeerIPKey.String(ip[0].String()))
		}
	}

	spanStartOptions := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(traceAttributes...),
	}

	return &DbPoolConnector{
		pool:             pool,
		spanStartOptions: spanStartOptions,
	}, nil
}

func (d *DbPoolConnector) Ping(ctx context.Context) error {
	return d.pool.Ping(ctx)
}

func (d *DbPoolConnector) Close() {
	d.pool.Close()
}

func (d *DbPoolConnector) Connect(ctx context.Context) (pgx.Tx, db.CloseFunc, error) {
	ctx, span := telemetry.Tracer.Start(ctx, "db-pool-connector.connect", d.spanStartOptions...)
	defer span.End()

	txImpl, err := d.pool.Begin(ctx)
	if err != nil {
		span.RecordError(err, trace.WithAttributes(semconv.ExceptionEscapedKey.Bool(true)))
		span.SetStatus(codes.Error, err.Error())
		return nil, nil, err
	}

	tx := otelTx{txImpl, d.spanStartOptions}

	return tx, func(ctx context.Context) { tx.Rollback(ctx) }, nil
}

// Wrapper around pgx.Tx that adds spans around its methods
type otelTx struct {
	pgx.Tx

	spanStartOptions []trace.SpanStartOption
}

func (t otelTx) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	ctx, span := telemetry.Tracer.Start(ctx, "tx.exec",
		append(t.spanStartOptions, trace.WithAttributes(semconv.DBStatementKey.String(sql)))...,
	)
	defer span.End()

	return t.Tx.Exec(ctx, sql, arguments...)
}

func (t otelTx) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	ctx, span := telemetry.Tracer.Start(ctx, "tx.query",
		append(t.spanStartOptions, trace.WithAttributes(semconv.DBStatementKey.String(sql)))...,
	)
	defer span.End()

	rows, err := t.Tx.Query(ctx, sql, args...)
	if err != nil && err != pgx.ErrNoRows {
		span.RecordError(err, trace.WithAttributes(semconv.ExceptionEscapedKey.Bool(true)))
		span.SetStatus(codes.Error, err.Error())
	}

	return rows, err
}

func (t otelTx) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	ctx, span := telemetry.Tracer.Start(ctx, "tx.query-row",
		append(t.spanStartOptions, trace.WithAttributes(semconv.DBStatementKey.String(sql)))...,
	)
	defer span.End()

	return t.Tx.QueryRow(ctx, sql, args...)
}

func (t otelTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	ctx, span := telemetry.Tracer.Start(ctx, "tx.send-batch", t.spanStartOptions...)
	defer span.End()

	return otelBatchResults{t.Tx.SendBatch(ctx, b), ctx, t.spanStartOptions}
}

func (t otelTx) Commit(ctx context.Context) error {
	ctx, span := telemetry.Tracer.Start(ctx, "tx.commit", t.spanStartOptions...)
	defer span.End()

	err := t.Tx.Commit(ctx)
	if err != nil {
		span.RecordError(err, trace.WithAttributes(semconv.ExceptionEscapedKey.Bool(true)))
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

func (t otelTx) Rollback(ctx context.Context) error {
	ctx, span := telemetry.Tracer.Start(ctx, "tx.rollback", t.spanStartOptions...)
	defer span.End()

	err := t.Tx.Rollback(ctx)
	if err != nil && err != pgx.ErrTxClosed {
		span.RecordError(err, trace.WithAttributes(semconv.ExceptionEscapedKey.Bool(true)))
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// Wrapper around pgx.BatchResults that adds spans around its methods
type otelBatchResults struct {
	pgx.BatchResults

	ctx              context.Context
	spanStartOptions []trace.SpanStartOption
}

func (b otelBatchResults) Exec() (pgconn.CommandTag, error) {
	_, span := telemetry.Tracer.Start(b.ctx, "batch-results.exec", b.spanStartOptions...)
	defer span.End()

	tag, err := b.BatchResults.Exec()
	if err != nil {
		span.RecordError(err, trace.WithAttributes(semconv.ExceptionEscapedKey.Bool(true)))
		span.SetStatus(codes.Error, err.Error())
	}

	return tag, err
}

func (b otelBatchResults) Query() (pgx.Rows, error) {
	_, span := telemetry.Tracer.Start(b.ctx, "batch-results.query", b.spanStartOptions...)
	defer span.End()

	rows, err := b.BatchResults.Query()
	if err != nil && err != pgx.ErrNoRows {
		span.RecordError(err, trace.WithAttributes(semconv.ExceptionEscapedKey.Bool(true)))
		span.SetStatus(codes.Error, err.Error())
	}

	return rows, err
}

func (b otelBatchResults) QueryRow() pgx.Row {
	_, span := telemetry.Tracer.Start(b.ctx, "batch-results.query-row", b.spanStartOptions...)
	defer span.End()

	return b.BatchResults.QueryRow()
}

func (b otelBatchResults) Close() error {
	_, span := telemetry.Tracer.Start(b.ctx, "batch-results.close", b.spanStartOptions...)
	defer span.End()

	err := b.BatchResults.Close()
	if err != nil {
		span.RecordError(err, trace.WithAttributes(semconv.ExceptionEscapedKey.Bool(true)))
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}
