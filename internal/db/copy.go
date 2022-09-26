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

func CopyToProject(ctx context.Context, tx pgx.Tx, source int64, target int64, vrange VersionRange) (*int64, error) {
	objectQuery := &pb.ObjectQuery{
		Path:        "",
		IsPrefix:    true,
		WithContent: false,
		WithHash:    true,
	}

	latestVersion, err := LockLatestVersion(ctx, tx, target)

	if err != nil {
		return nil, fmt.Errorf("copy to project could not lock target (%d) version: %w", target, err)
	}

	newTargetVersion := latestVersion + 1

	builder := newQueryBuilder(source, vrange, objectQuery)
	removedSql, removedArgs := builder.buildRemoved()
	removedArgsLength := len(removedArgs)

	sql := fmt.Sprintf(`
	%v
	UPDATE dl.objects
	SET stop_version = $%d
	FROM removed_objects
	WHERE "removed_objects".path = "objects".path
	AND "objects".project = $%d
	AND "objects".stop_version IS NULL
	`, AsCte(removedSql, "removed_objects"), removedArgsLength+1, removedArgsLength+2)

	updatedRemovedArgs := append(removedArgs, newTargetVersion, target)

	_, err = tx.Exec(ctx, sql, updatedRemovedArgs...)

	if err != nil {
		return nil, fmt.Errorf("copy to project could not update removed files to version (%d): %w", latestVersion, err)
	}

	changedSql, changedArgs := builder.buildChanged()
	changedArgsLength := len(changedArgs)

	sql = fmt.Sprintf(`
	%v
	INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
	SELECT $%d, $%d, null, path, hash, mode, size, packed
	FROM changed_objects
	WHERE deleted IS false
	ON CONFLICT
	   DO NOTHING
	`, AsCte(changedSql, "changed_objects"), changedArgsLength+1, changedArgsLength+2)

	insertArgs := append(changedArgs, target, newTargetVersion)

	_, err = tx.Exec(ctx, sql, insertArgs...)

	if err != nil {
		return nil, fmt.Errorf("copy to project could not insert updated files: %w", err)
	}

	err = UpdateLatestVersion(ctx, tx, target, newTargetVersion)

	if err != nil {
		return nil, fmt.Errorf("copy to project could not update target (%d) to latest version (%d): %w", target, newTargetVersion, err)
	}

	return &newTargetVersion, nil
}
