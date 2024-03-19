package db

import (
	"context"
	"errors"
	"fmt"
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

func DeleteObject(ctx context.Context, tx pgx.Tx, project int64, version int64, path string, isStaged bool) error {
	if isStaged {
		_, err := tx.Exec(ctx, `
			INSERT INTO dl.staged_objects (project, start_version, stop_version, path, hash, mode, size, packed)
			VALUES ($1, NULL, $2, $3, NULL, NULL, NULL, NULL)
		`, project, version, path)
		if err != nil {
			return fmt.Errorf("delete staged object, project %v, version %v, path %v: %w", project, version, path, err)
		}

		return nil
	}

	_, err := tx.Exec(ctx, `
		UPDATE dl.objects
		SET stop_version = $1
		WHERE project = $2
		  AND path = $3
		  AND stop_version IS NULL
	`, version, project, path)
	if err != nil {
		return fmt.Errorf("delete object, project %v, version %v, path %v: %w", project, version, path, err)
	}

	return nil
}

// UpdateObject returns true if content changed, false otherwise
func UpdateObject(ctx context.Context, tx pgx.Tx, conn DbConnector, encoder *ContentEncoder, project int64, version int64, object *pb.Object, isStaged bool) (bool, error) {
	content := object.Content
	if content == nil {
		content = []byte("")
	}
	hash := HashContent(content)

	encoded, err := encoder.Encode(content)
	if err != nil {
		return false, fmt.Errorf("encode updated content, project %v, version %v, path %v: %w", project, version, object.Path, err)
	}

	// insert the content outside the transaction to avoid deadlocks and to keep smaller transactions
	_, err = conn.Exec(ctx, `
		INSERT INTO dl.contents (hash, bytes)
		VALUES (($1, $2), $3)
		ON CONFLICT DO NOTHING
	`, hash.H1, hash.H2, encoded)
	if err != nil {
		return false, fmt.Errorf("insert objects content, hash %x-%x: %w", hash.H1, hash.H2, err)
	}

	objectTable := "dl.objects"
	if isStaged {
		objectTable = "dl.staged_objects"
	}

	rows, err := tx.Query(ctx, fmt.Sprintf(`
		INSERT INTO %s (project, start_version, stop_version, path, hash, mode, size, packed)
		VALUES ($1, $2, NULL, $3, ($4, $5), $6, $7, $8)
		ON CONFLICT
	       DO NOTHING
		RETURNING project
	`, objectTable), project, version, object.Path, hash.H1, hash.H2, object.Mode, object.Size, false)
	if err != nil {
		return false, fmt.Errorf("insert new object, project %v, version %v, path %v: %w", project, version, object.Path, err)
	}

	isIdentical := !rows.Next()
	rows.Close()

	if isIdentical {
		return false, nil
	}

	if !isStaged {
		previousPaths := []string{object.Path}
		pathChunks := strings.Split(object.Path, "/")

		for i := 1; i < len(pathChunks); i++ {
			previousPaths = append(previousPaths, fmt.Sprintf("%s/", strings.Join(pathChunks[:i], "/")))
		}

		_, err = tx.Exec(ctx, `
			UPDATE dl.objects SET stop_version = $1
			WHERE project = $2
			AND path = ANY($3)
			AND stop_version IS NULL
			AND start_version != $4
		`, version, project, previousPaths, version)
		if err != nil {
			return false, fmt.Errorf("update previous object, project %v, version %v, path %v: %w", project, version, object.Path, err)
		}
	}

	return true, nil
}

// UpdatePackedObjects returns true if content changed, false otherwise
func UpdatePackedObjects(ctx context.Context, tx pgx.Tx, conn DbConnector, project int64, version int64, parent string, updates []*pb.Object, isStaged bool) (bool, error) {
	var hash Hash
	var content []byte

	rows, err := tx.Query(ctx, `
		SELECT (o.hash).h1, (o.hash).h2, c.bytes
		FROM dl.objects o
		JOIN dl.contents c
		  ON o.hash = c.hash
		WHERE project = $1
		  AND path = $2
		  AND packed IS true
		  AND stop_version IS NULL
	`, project, parent)
	if err != nil {
		return false, fmt.Errorf("select existing packed object, project %v, version %v, parent %v: %w", project, version, parent, err)
	}

	if rows.Next() {
		err = rows.Scan(&hash.H1, &hash.H2, &content)
		if err != nil {
			return false, fmt.Errorf("scan hash and packed content from existing object, project %v, version %v, parent %v: %w", project, version, parent, err)
		}
	}
	rows.Close()

	shouldInsert := true
	updated, err := updateObjects(content, updates)
	if errors.Is(err, ErrEmptyPack) {
		// If the newly packed object is empty, we only need to delete the old one.
		shouldInsert = false
	} else if err != nil {
		return false, fmt.Errorf("update packed object: %w", err)
	}

	newHash := HashContent(updated)

	if hash == newHash {
		// content didn't change
		return false, nil
	}

	batch := &pgx.Batch{}

	if isStaged {
		batch.Queue(`
			INSERT INTO dl.staged_objects (project, start_version, stop_version, path, hash, mode, size, packed)
			VALUES ($1, NULL, $2, $3, NULL, NULL, NULL, NULL)
		`, project, version, parent)
	} else {
		batch.Queue(`
			UPDATE dl.objects SET stop_version = $1
			WHERE project = $2
			AND path = $3
			AND packed IS true
			AND stop_version IS NULL
		`, version, project, parent)
	}

	if shouldInsert {
		// insert the content outside the transaction to avoid deadlocks and to keep smaller transactions
		_, err = conn.Exec(ctx, `
			INSERT INTO dl.contents (hash, bytes)
			VALUES (($1, $2), $3)
			ON CONFLICT
				DO NOTHING
		`, newHash.H1, newHash.H2, updated)

		if err != nil {
			return false, fmt.Errorf("insert packed content, hash %x-%x: %w", newHash.H1, newHash.H2, err)
		}

		objectTable := "dl.objects"
		if isStaged {
			objectTable = "dl.staged_objects"
		}

		batch.Queue(fmt.Sprintf(`
			INSERT INTO %s (project, start_version, stop_version, path, hash, mode, size, packed)
			VALUES ($1, $2, NULL, $3, ($4, $5), $6, $7, $8)
		`, objectTable), project, version, parent, newHash.H1, newHash.H2, 0, len(updated), true)
	}

	results := tx.SendBatch(ctx, batch)
	defer results.Close()

	_, err = results.Exec()
	if err != nil {
		return false, fmt.Errorf("update existing object, project %v, version %v, parent %v: %w", project, version, parent, err)
	}

	if shouldInsert {
		_, err = results.Exec()
		if err != nil {
			return false, fmt.Errorf("insert new packed object, project %v, version %v, parent %v: %w", project, version, parent, err)
		}
	}

	// content did change
	return true, nil
}

func CommitStagedObjects(ctx context.Context, tx pgx.Tx, project int64, version int64) error {
	batch := &pgx.Batch{}

	batch.Queue(`
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		SELECT project, start_version, NULL, path, hash, mode, size, packed
		FROM dl.staged_objects
		WHERE project = $1
		  AND start_version = $2
		  AND stop_version IS NULL
	`, project, version)

	batch.Queue(`
		UPDATE dl.objects SET stop_version = $2
		FROM (
			SELECT project, path
			FROM dl.staged_objects
			WHERE project = $1
			  AND stop_version = $2
			  AND start_version IS NULL
		) staged
		WHERE staged.project = dl.objects.project
		  AND staged.path = dl.objects.path
	`, project, version)

	batch.Queue(`
		DELETE FROM dl.staged_objects
		WHERE project = $1
		  AND (start_version = $2 OR stop_version = $2)
	`, project, version)

	results := tx.SendBatch(ctx, batch)
	defer results.Close()

	_, err := results.Exec()
	if err != nil {
		return fmt.Errorf("commit new objects, project %v, version %v: %w", project, version, err)
	}
	_, err = results.Exec()
	if err != nil {
		return fmt.Errorf("commit deleted objects, project %v, version %v: %w", project, version, err)
	}
	_, err = results.Exec()
	if err != nil {
		return fmt.Errorf("deleted staged objects, project %v, version %v: %w", project, version, err)
	}

	return nil
}
