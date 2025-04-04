package db

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/jackc/pgx/v5"
)

func UpdateLatestVersion(ctx context.Context, tx pgx.Tx, project int64, version int64) error {
	_, err := tx.Exec(ctx, `
		UPDATE dl.projects
		SET latest_version = $1
		WHERE id = $2
	`, version, project)
	if err != nil {
		return fmt.Errorf("update project %v latest version to %v: %w", project, version, err)
	}

	return nil
}

func DeleteObjects(ctx context.Context, tx pgx.Tx, project int64, version int64, paths []string) error {
	_, err := tx.Exec(ctx, `
		UPDATE dl.objects
		SET stop_version = $1
		WHERE project = $2
		  AND path = ANY($3)
		  AND stop_version IS NULL
	`, version, project, paths)
	if err != nil {
		return fmt.Errorf("delete objects, project %v, version %v, paths %v: %w", project, version, paths, err)
	}

	return nil
}

// UpdateObjects returns true if content changed, false otherwise
func UpdateObjects(ctx context.Context, tx pgx.Tx, conn DbConnector, encoder *ContentEncoder, project int64, version int64, objects []*pb.Object) (bool, error) {
	var objectColumnValues [][]any
	for _, object := range objects {
		content := object.Content
		if content == nil {
			content = []byte("")
		}

		encoded, err := encoder.Encode(content)
		if err != nil {
			return false, fmt.Errorf("encode updated content, project %v, version %v, path %v: %w", project, version, object.Path, err)
		}

		hash := HashContent(content)
		objectColumnValues = append(objectColumnValues, []any{
			hash,
			encoded,
			object.Path,
			object.Mode,
			object.Size,
		})
	}

	tableName := fmt.Sprintf("__update_%d_%d", project, version)
	_, err := tx.Exec(ctx, fmt.Sprintf(`
		CREATE TEMPORARY TABLE
			%s (hash hash, bytes bytea, path text, mode bigint, size bigint)
		ON COMMIT DROP
	`, tableName))
	if err != nil {
		return false, fmt.Errorf("create temporary table for update failed: %w", err)
	}

	_, err = tx.CopyFrom(ctx, pgx.Identifier{tableName}, []string{"hash", "bytes", "path", "mode", "size"}, pgx.CopyFromRows(objectColumnValues))
	if err != nil {
		return false, fmt.Errorf("insert objects content, %w", err)
	}

	_, err = tx.Exec(ctx, fmt.Sprintf(`
		INSERT INTO
			dl.contents (hash, bytes)
		SELECT
			hash, bytes
		FROM
			%s
		ON CONFLICT
			DO NOTHING
	`, tableName))
	if err != nil {
		return false, fmt.Errorf("insert into contents table failed, %w", err)
	}

	rows, err := tx.Query(ctx, fmt.Sprintf(`
		INSERT INTO
			dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		SELECT
			$1 as project,
			$2 as start_version,
			NULL as stop_version,
			path,
			hash,
			mode,
			size,
			false as packed
		FROM
			%s
		ON CONFLICT
			DO NOTHING
		RETURNING
			path
		`, tableName), project, version)
	if err != nil {
		return false, fmt.Errorf("insert new object, project %v, version %v: %w", project, version, err)
	}

	previousPaths := make(map[string]bool)
	for rows.Next() {
		var path string
		err = rows.Scan(&path)
		if err != nil {
			return false, fmt.Errorf("scan path, project %v, version %v: %w", project, version, err)
		}

		previousPaths[path] = true
		pathChunks := strings.Split(path, "/")
		for i := 1; i < len(pathChunks); i++ {
			previousPaths[fmt.Sprintf("%s/", strings.Join(pathChunks[:i], "/"))] = true
		}
	}
	rows.Close()

	if len(previousPaths) == 0 {
		return false, nil
	}

	previousPathsSlice := make([]string, 0, len(previousPaths))
	for path := range previousPaths {
		previousPathsSlice = append(previousPathsSlice, path)
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			dl.objects
		SET
			stop_version = $1
		WHERE
			project = $2
		  AND path = ANY($3)
		  AND stop_version IS NULL
		  AND start_version != $4
	`, version, project, previousPathsSlice, version)
	if err != nil {
		return false, fmt.Errorf("update previous object, project %v, version %v: %w", project, version, err)
	}

	return true, nil
}

// UpdatePackedObjects returns true if content changed, false otherwise
func UpdatePackedObjects(ctx context.Context, tx pgx.Tx, conn DbConnector, project int64, version int64, updates map[string][]*pb.Object) (bool, error) {
	parents := slices.Collect(maps.Keys(updates))

	rows, err := tx.Query(ctx, `
		SELECT
			(c.hash).h1,
			(c.hash).h2,
			c.bytes,
			o.path
		FROM
			dl.objects o
		JOIN
			dl.contents c
		  ON o.hash = c.hash
		WHERE
			o.project = $1
		  AND o.path = ANY($2)
		  AND o.packed IS true
		  AND o.stop_version IS NULL
	`, project, parents)
	if err != nil {
		return false, fmt.Errorf("select existing packed object, project %v, version %v: %w", project, version, err)
	}

	hashes := make(map[string]Hash)
	contents := make(map[string][]byte)

	for rows.Next() {
		var parent string
		var hash Hash
		var content []byte

		err = rows.Scan(&hash.H1, &hash.H2, &content, &parent)
		if err != nil {
			return false, fmt.Errorf("scan hash and packed content from existing object, project %v, version %v: %w", project, version, err)
		}

		hashes[parent] = hash
		contents[parent] = content
	}
	rows.Close()

	err = rows.Err()
	if err != nil {
		return false, fmt.Errorf("failed to iterate rows: %w", err)
	}

	objectColumnValues := make([][]any, 0, len(updates))
	changedParents := make([]string, 0, len(updates))

	for parent, updates := range updates {
		updated, err := updateObjects(contents[parent], updates)
		packIsEmpty := errors.Is(err, ErrEmptyPack)
		if err != nil && !packIsEmpty {
			return false, fmt.Errorf("update packed object: %w", err)
		}

		newHash := HashContent(updated)
		if hashes[parent] != newHash {
			changedParents = append(changedParents, parent)

			if !packIsEmpty {
				objectColumnValues = append(objectColumnValues, []any{
					newHash,
					updated,
					parent,
					int64(len(updated)),
				})
			}
		}
	}

	if len(changedParents) == 0 {
		return false, nil
	}

	batch := &pgx.Batch{}

	batch.Queue(`
		UPDATE
			dl.objects
		SET
			stop_version = $1
		WHERE
			project = $2
			AND path = ANY($3)
			AND packed IS true
			AND stop_version IS NULL
	`, version, project, changedParents)

	if len(objectColumnValues) > 0 {
		tableName := fmt.Sprintf("__update_packed_%d_%d", project, version)

		_, err = tx.Exec(ctx, fmt.Sprintf(`
			CREATE TEMPORARY TABLE
				%s (hash hash, bytes bytea, path text, size bigint)
			ON COMMIT DROP
		`, tableName))
		if err != nil {
			return false, fmt.Errorf("create temporary table for update failed: %w", err)
		}

		_, err = tx.CopyFrom(ctx, pgx.Identifier{tableName}, []string{"hash", "bytes", "path", "size"}, pgx.CopyFromRows(objectColumnValues))
		if err != nil {
			return false, fmt.Errorf("insert packaged objects content, %w", err)
		}

		batch.Queue(fmt.Sprintf(`
			INSERT INTO
				dl.contents (hash, bytes)
			SELECT
				hash, bytes
			FROM
				%s
			ON CONFLICT
				DO NOTHING
	  `, tableName))

		batch.Queue(fmt.Sprintf(`
			INSERT INTO
				dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
			SELECT
				$1 as project,
				$2 as start_version,
				NULL as stop_version,
				path,
				hash,
				0 as mode,
				size,
				true as packed
			FROM
				%s
		`, tableName), project, version)
	}

	results := tx.SendBatch(ctx, batch)
	defer results.Close()

	_, err = results.Exec()
	if err != nil {
		return false, fmt.Errorf("update existing object, project %v, version %v: %w", project, version, err)
	}

	return true, nil
}
