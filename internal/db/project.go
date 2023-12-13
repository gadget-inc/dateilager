package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/jackc/pgx/v5"
)

func CreateProject(ctx context.Context, tx pgx.Tx, project int64, packPatterns []string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO dl.projects (id, latest_version, pack_patterns)
		VALUES ($1, 0, $2)
	`, project, packPatterns)

	var projectExistsError = errors.New("ERROR: duplicate key value violates unique constraint \"projects_pkey\" (SQLSTATE 23505)")

	if err != nil && err.Error() == projectExistsError.Error() {
		return fmt.Errorf("project id already exists")
	}

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

func ListProjects(ctx context.Context, tx pgx.Tx) ([]*pb.Project, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, latest_version
		FROM dl.projects
	`)
	if err != nil {
		return nil, fmt.Errorf("snapshotProjects query: %w", err)
	}

	projects := []*pb.Project{}

	for rows.Next() {
		var id, version int64
		err = rows.Scan(&id, &version)
		if err != nil {
			return nil, fmt.Errorf("snapshotProjects scan: %w", err)
		}
		projects = append(projects, &pb.Project{Id: id, Version: version})
	}

	return projects, nil
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

func getLatestVersion(ctx context.Context, tx pgx.Tx, project int64) (int64, error) {
	var latestVersion int64

	err := tx.QueryRow(ctx, `
		SELECT latest_version
		FROM dl.projects WHERE id = $1
	`, project).Scan(&latestVersion)
	if err == pgx.ErrNoRows {
		return -1, fmt.Errorf("get latest version for %v: %w", project, ErrNotFound)
	}
	if err != nil {
		return -1, fmt.Errorf("get latest version for %v: %w", project, err)
	}

	return latestVersion, nil
}

func LockLatestVersion(ctx context.Context, tx pgx.Tx, project int64) (int64, error) {
	var latestVersion int64

	err := tx.QueryRow(ctx, `
		SELECT latest_version
		FROM dl.projects WHERE id = $1
		FOR UPDATE
	`, project).Scan(&latestVersion)
	if err == pgx.ErrNoRows {
		return -1, fmt.Errorf("lock latest version for %v: %w", project, ErrNotFound)
	}
	if err != nil {
		return -1, fmt.Errorf("lock latest version for %v: %w", project, err)
	}

	return latestVersion, nil
}

func HasSamePackPattern(ctx context.Context, tx pgx.Tx, project_1 int64, project_2 int64) (bool, error) {
	var samePackPatterns bool

	err := tx.QueryRow(ctx, `
		SELECT COALESCE((SELECT pack_patterns FROM dl.projects WHERE id = $1), '{}') =
		       COALESCE((SELECT pack_patterns FROM dl.projects WHERE id = $2), '{}');
	`, project_1, project_2).Scan(&samePackPatterns)
	if err != nil {
		return false, fmt.Errorf("check matching pack patterns, source %v, target %v: %w", project_1, project_2, err)
	}

	return samePackPatterns, nil
}
