package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4"
)

func CreateProject(ctx context.Context, tx pgx.Tx, project int64, packPatterns []string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO dl.projects (id, latest_version, pack_patterns)
		VALUES ($1, 0, $2)
	`, project, packPatterns)
	if err != nil {
		return fmt.Errorf("create project %v, packPatterns %v: %w", project, packPatterns, err)
	}

	return nil
}

func DeleteProject(ctx context.Context, tx pgx.Tx, project int64) error {
	_, err := tx.Exec(ctx, `
		DELETE FROM dl.objects
		WHERE project = $1
	`, project)
	if err != nil {
		return fmt.Errorf("delete objects for %v %w", project, err)
	}

	_, err = tx.Exec(ctx, `
		DELETE FROM dl.projects
		WHERE id = $1
	`, project)
	if err != nil {
		return fmt.Errorf("delete project %v %w", project, err)
	}

	return nil
}
