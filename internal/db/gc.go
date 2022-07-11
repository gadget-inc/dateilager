package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func GcProjectObjects(ctx context.Context, tx pgx.Tx, project int64, keep int64, fromVersion int64) ([]Hash, error) {
	hashes := []Hash{}

	rows, err := tx.Query(ctx, `
		WITH latest AS (
			SELECT latest_version AS version
			FROM dl.projects
			WHERE id = $1
		)
		DELETE FROM dl.objects
		WHERE project = $1
		  AND start_version > $2
		  AND stop_version IS NOT NULL
		  AND stop_version <= ((SELECT version FROM latest) - $3)
		RETURNING (hash).h1, (hash).h2
	`, project, fromVersion, keep)
	if err != nil {
		return hashes, fmt.Errorf("GcProjectObjects query, projects %v, keep %v, from %v: %w", project, keep, fromVersion, err)
	}

	for rows.Next() {
		var hash Hash
		err = rows.Scan(&hash.H1, &hash.H2)
		if err != nil {
			return hashes, fmt.Errorf("GcProjectObjects scan %v: %w", project, err)
		}

		hashes = append(hashes, hash)
	}

	return hashes, nil
}

func GcProjectsObjects(ctx context.Context, tx pgx.Tx, projects []int64, keep int64, fromVersion int64) ([]Hash, error) {
	hashes := []Hash{}

	for _, project := range projects {
		h, err := GcProjectObjects(ctx, tx, project, keep, fromVersion)
		if err != nil {
			return nil, err
		}
		hashes = append(hashes, h...)
	}

	return hashes, nil
}

func GcContentHashes(ctx context.Context, tx pgx.Tx, hashes []Hash) (int64, error) {
	if len(hashes) == 0 {
		return 0, nil
	}

	tag, err := tx.Exec(ctx, `
		WITH missing AS (
			SELECT c.hash
			FROM dl.contents c
			LEFT JOIN dl.objects o
				ON c.hash = o.hash
			WHERE c.hash = ANY($1::hash[])
			AND o.hash IS NULL
		)
		DELETE FROM dl.contents
		WHERE hash IN (SELECT hash FROM missing)
	`, hashes)
	if err != nil {
		return 0, fmt.Errorf("GcContentHashes query, hash count %v: %w", len(hashes), err)
	}

	return tag.RowsAffected(), nil
}
