package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func TotalObjectsCount(ctx context.Context, tx pgx.Tx, project int64) (int64, error) {
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
