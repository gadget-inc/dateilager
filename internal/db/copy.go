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

func CloneToProject(ctx context.Context, tx pgx.Tx, source int64, target int64, version int64, newTargetVersion int64) error {
	objectQuery := &pb.ObjectQuery{
		Path:     "",
		IsPrefix: true,
	}

	sourceBuilder := newQueryBuilder(source, VersionRange{
		To: version,
	}, objectQuery).withHashes(true)
	sourceSql, sourceArgs := sourceBuilder.build()

	targetBuilder := newQueryBuilder(target, VersionRange{
		To: newTargetVersion - 1,
	}, objectQuery).withHashes(true).withArgsOffset(len(sourceArgs))
	targetSql, targetArgs := targetBuilder.build()

	sql := fmt.Sprintf(`
		WITH live_source_objects AS (
			%s
		), to_remove AS (
			%s
			EXCEPT
			SELECT path, mode, size, is_cached, bytes, packed, deleted, h1, h2
			FROM live_source_objects
		)
		UPDATE dl.objects o
		SET stop_version = $%d
		FROM to_remove r
		WHERE o.project = $%d
		  AND o.path = r.path
		  AND o.stop_version IS NULL
	`, sourceSql, targetSql, len(sourceArgs)+len(targetArgs)+1, len(sourceArgs)+len(targetArgs)+2)

	_, err := tx.Exec(ctx, sql, append(append(sourceArgs, targetArgs...), newTargetVersion, target)...)
	if err != nil {
		return fmt.Errorf("copy to project could not update removed files to version (%d): %w", newTargetVersion, err)
	}

	sql = fmt.Sprintf(`
		WITH live_source_objects AS (
			%s
		)
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		SELECT $%d, $%d, null, path, (h1, h2)::hash, mode, size, packed
		FROM live_source_objects
		WHERE deleted = false
		ON CONFLICT
		   DO NOTHING
	`, sourceSql, len(sourceArgs)+1, len(sourceArgs)+2)

	_, err = tx.Exec(ctx, sql, append(sourceArgs, target, newTargetVersion)...)
	if err != nil {
		return fmt.Errorf("copy to project could not insert updated files: %w", err)
	}

	return nil
}
