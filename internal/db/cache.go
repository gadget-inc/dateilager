package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func CreateCache(ctx context.Context, tx pgx.Tx, prefix string, count int64) (int64, error) {
	sql := `
		WITH impactful_packed_objects AS (
			SELECT hash, count(*) AS count
			FROM dl.objects
			WHERE path LIKE $1
			  AND packed = true
			  AND stop_version IS NULL
			GROUP BY hash
			ORDER BY count DESC
			LIMIT $2
		)
		INSERT INTO dl.cache_versions (hashes)
		SELECT COALESCE(array_agg(hash), '{}') AS hashes
		FROM impactful_packed_objects
		RETURNING version
	`

	var version int64
	err := tx.QueryRow(ctx, sql, fmt.Sprintf("%s%%", prefix), count).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("CreateCache query, %w", err)
	}

	return version, nil
}
