package db

import (
	"context"
	"fmt"

	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/jackc/pgx/v5"
)

func CopyAllObjects(ctx context.Context, tx pgx.Tx, source int64, target int64) error {
	samePackPatterns, err := HasSamePackPattern(ctx, tx, source, target)
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

func CloneToProject(ctx context.Context, tx pgx.Tx, source int64, target int64, vrange VersionRange, newTargetVersion int64) error {
	objectQuery := &pb.ObjectQuery{
		Path:        "",
		IsPrefix:    true,
		WithContent: false,
	}

	builder := newQueryBuilder(source, vrange, objectQuery, nil, true)
	innerSql, args := builder.build()
	argsLength := len(args)

	sql := fmt.Sprintf(`
		WITH changed_objects AS (
			%s
		)
		UPDATE dl.objects
		SET stop_version = $%d
		FROM changed_objects
		WHERE "changed_objects".path = "objects".path
		  AND "objects".project = $%d
		  AND "objects".stop_version IS NULL
	`, innerSql, argsLength+1, argsLength+2)

	_, err := tx.Exec(ctx, sql, append(args, newTargetVersion, target)...)
	if err != nil {
		return fmt.Errorf("copy to project could not update removed files to version (%d): %w", newTargetVersion, err)
	}

	sql = fmt.Sprintf(`
		WITH changed_objects AS (
			%s
		)
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		SELECT $%d, $%d, null, path, (h1, h2)::hash, mode, size, packed
		FROM changed_objects
		WHERE deleted IS false
		ON CONFLICT
		   DO NOTHING
	`, innerSql, argsLength+1, argsLength+2)

	_, err = tx.Exec(ctx, sql, append(args, target, newTargetVersion)...)
	if err != nil {
		return fmt.Errorf("copy to project could not insert updated files: %w", err)
	}

	return nil
}
