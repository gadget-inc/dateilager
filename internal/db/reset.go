package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func ResetAll(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, "TRUNCATE dl.projects;")
	if err != nil {
		return fmt.Errorf("truncate projects: %w", err)
	}

	_, err = tx.Exec(ctx, "TRUNCATE dl.objects;")
	if err != nil {
		return fmt.Errorf("truncate objects: %w", err)
	}

	_, err = tx.Exec(ctx, "TRUNCATE dl.contents;")
	if err != nil {
		return fmt.Errorf("truncate contents: %w", err)
	}

	return nil
}

func ResetProject(ctx context.Context, tx pgx.Tx, project, version int64) error {
	_, err := tx.Exec(ctx, `
		UPDATE dl.projects
		SET latest_version = $1
		WHERE id = $2
	`, version, project)
	if err != nil {
		return fmt.Errorf("reset project %v to version %v: %w", project, version, err)
	}

	_, err = tx.Exec(ctx, `
		DELETE FROM dl.objects
		WHERE project = $1
		  AND start_version > $2
	`, project, version)
	if err != nil {
		return fmt.Errorf("reset objects for %v above version %v: %w", project, version, err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE dl.objects
		SET stop_version = NULL
		WHERE project = $1
		  AND stop_version > $2
	`, project, version)
	if err != nil {
		return fmt.Errorf("reset objects for %v with stop_version %v: %w", project, version, err)
	}

	return nil
}

func DropOtherProjects(ctx context.Context, tx pgx.Tx, projects []int64) error {
	_, err := tx.Exec(ctx, `
		DELETE FROM dl.projects
		WHERE id != ALL($1::bigint[])
	`, projects)
	if err != nil {
		return fmt.Errorf("drop other projects: %w", err)
	}

	_, err = tx.Exec(ctx, `
		DELETE FROM dl.objects
		WHERE project != ALL($1::bigint[])
	`, projects)
	if err != nil {
		return fmt.Errorf("drop objects from other projects: %w", err)
	}

	return nil
}
