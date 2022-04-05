package db

import (
	"context"
	"fmt"

	"github.com/gadget-inc/dateilager/internal/telemetry"
	"github.com/jackc/pgx/v4"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func TotalObjectsCount(ctx context.Context, tx pgx.Tx, project int64) (int64, error) {
	ctx, span := telemetry.Tracer.Start(ctx, "total-objects-count", trace.WithAttributes(
		attribute.Int64("project", project),
	))
	defer span.End()

	var count int64
	err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM dl.objects
		WHERE project = $1
	`, project).Scan(&count)
	if err != nil {
		return -1, fmt.Errorf("total object count for project %v: %w", project, err)
	}

	return count, nil
}
