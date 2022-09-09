package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func CopyAllObjects(ctx context.Context, tx pgx.Tx, source int64, target int64) error {
	var samePackPatterns bool
	err := tx.QueryRow(ctx, `
		SELECT COALESCE((SELECT pack_patterns FROM dl.projects WHERE id = $1), '{}') =
		       COALESCE((SELECT pack_patterns FROM dl.projects WHERE id = $2), '{}');
	`, source, target).Scan(&samePackPatterns)
	if err != nil {
		return fmt.Errorf("check matching pack patterns, source %v, target %v: %w", source, target, err)
	}

	if !samePackPatterns {
		return fmt.Errorf("cannot copy paths because pack patterns do not match for source %v and target %v", source, target)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		SELECT $1, start_version, stop_version, path, hash, mode, size, packed
		FROM dl.objects
		WHERE project = $2
	`, target, source)
	if err != nil {
		return fmt.Errorf("copy project, source %v, target %v: %w", source, target, err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE dl.projects
		SET latest_version = (SELECT latest_version FROM dl.projects WHERE id = $1)
		WHERE id = $2
	`, source, target)
	if err != nil {
		return fmt.Errorf("copy project update version, source %v, target %v: %w", source, target, err)
	}

	return nil
}
