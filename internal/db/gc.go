package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
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
		return hashes, fmt.Errorf("GcProjectObjects query, project %v, keep %v, from %v: %w", project, keep, fromVersion, err)
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

func GcContentHashes(ctx context.Context, tx pgx.Tx, hashes []Hash) (int64, error) {
	count := int64(0)

	tag, err := tx.Exec(ctx, `
		WITH missing AS (
			SELECT hash
			FROM dl.objects
			WHERE hash = ANY($1::hash[])
			GROUP BY hash
			HAVING count(*) = 0
		)
		DELETE FROM dl.contents
		WHERE hash IN (SELECT hash from missing)
	`, pgx.QuerySimpleProtocol(true), hashes)
	if err != nil {
		return count, fmt.Errorf("GcContentHashes query, hash count %v: %w", len(hashes), err)
	}

	return tag.RowsAffected(), nil
}

func encodeHashes(hashes []Hash) []pgtype.CompositeFields {
	fields := make([]pgtype.CompositeFields, len(hashes))

	for _, hash := range hashes {
		fields = append(fields, pgtype.CompositeFields{hash.H1, hash.H2})
	}

	return fields
}
