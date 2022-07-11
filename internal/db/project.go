package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
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

func RandomProjects(ctx context.Context, tx pgx.Tx, sample float32) ([]int64, error) {
	var projects []int64

	for i := 0; i < 5; i++ {
		// The SYSTEM sampling method would be quicker but it often produces no or all data
		// on tables with just a few rows.
		rows, err := tx.Query(ctx, fmt.Sprintf(`
			SELECT id
			FROM dl.projects
			TABLESAMPLE BERNOULLI(%f)
		`, sample))
		if err != nil {
			return nil, fmt.Errorf("random projects: %w", err)
		}

		for rows.Next() {
			var project int64
			err = rows.Scan(&project)
			if err != nil {
				return nil, fmt.Errorf("random projects scan: %w", err)
			}

			projects = append(projects, project)
		}

		// With only few projects this sometimes returns no results.
		if len(projects) > 0 {
			break
		}
	}

	return projects, nil
}
