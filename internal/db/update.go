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

func DeleteObject(ctx context.Context, tx pgx.Tx, project int64, version int64, path string) error {
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

func DeleteObjects(ctx context.Context, tx pgx.Tx, project int64, version int64, path string) error {
	pathPredicate := fmt.Sprintf("%s%%", path)
	_, err := tx.Exec(ctx, `
		UPDATE dl.objects
		SET stop_version = $1
		WHERE project = $2
		  AND path LIKE $3
		  AND stop_version IS NULL
	`, version, project, pathPredicate)
	if err != nil {
		return fmt.Errorf("delete objects, project %v, version %v, path %v: %w", project, version, pathPredicate, err)
	}

	return nil
}

// UpdateObject returns true if content changed, false otherwise
func UpdateObject(ctx context.Context, tx pgx.Tx, encoder *ContentEncoder, project int64, version int64, object *pb.Object) (bool, error) {
	content := object.Content
	if content == nil {
		content = []byte("")
	}
	hash := HashContent(content)

	encoded, err := encoder.Encode(content)
	if err != nil {
		return false, fmt.Errorf("encode updated content, project %v, version %v, path %v: %w", project, version, object.Path, err)
	}

	rows, err := tx.Query(ctx, `
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		VALUES ($1, $2, NULL, $3, ($4, $5), $6, $7, $8)
		ON CONFLICT
	       DO NOTHING
		RETURNING project
	`, project, version, object.Path, hash.H1, hash.H2, object.Mode, object.Size, false)
	if err != nil {
		return false, fmt.Errorf("insert new object, project %v, version %v, path %v: %w", project, version, object.Path, err)
	}

	isIdentical := !rows.Next()
	rows.Close()

	if isIdentical {
		return false, nil
	}

	batch := &pgx.Batch{}

	previousPaths := []string{object.Path}
	pathChunks := strings.Split(object.Path, "/")

	for i := 1; i < len(pathChunks); i++ {
		previousPaths = append(previousPaths, fmt.Sprintf("%s/", strings.Join(pathChunks[:i], "/")))
	}

	batch.Queue(`
		UPDATE dl.objects SET stop_version = $1
		WHERE project = $2
		  AND path = ANY($3)
		  AND stop_version IS NULL
		  AND start_version != $4
	`, version, project, previousPaths, version)

	batch.Queue(`
		INSERT INTO dl.contents (hash, bytes)
		VALUES (($1, $2), $3)
		ON CONFLICT
		   DO NOTHING
	`, hash.H1, hash.H2, encoded)

	results := tx.SendBatch(ctx, batch)
	defer results.Close()

	_, err = results.Exec()
	if err != nil {
		return false, fmt.Errorf("update previous object, project %v, version %v, path %v: %w", project, version, object.Path, err)
	}

	_, err = results.Exec()
	if err != nil {
		return false, fmt.Errorf("insert updated content, hash %x-%x: %w", hash.H1, hash.H2, err)
	}

	return true, nil
}

// UpdatePackedObjects returns true if content changed, false otherwise
func UpdatePackedObjects(ctx context.Context, tx pgx.Tx, project int64, version int64, parent string, updates []*pb.Object) (bool, error) {
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

	batch.Queue(`
		UPDATE dl.objects SET stop_version = $1
		WHERE project = $2
		  AND path = $3
		  AND packed IS true
		  AND stop_version IS NULL
	`, version, project, parent)

	if shouldInsert {
		batch.Queue(`
			INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
			VALUES ($1, $2, NULL, $3, ($4, $5), $6, $7, $8)
		`, project, version, parent, newHash.H1, newHash.H2, 0, len(updated), true)

		batch.Queue(`
			INSERT INTO dl.contents (hash, bytes)
			VALUES (($1, $2), $3)
			ON CONFLICT
			   DO NOTHING
		`, newHash.H1, newHash.H2, updated)
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

		_, err = results.Exec()
		if err != nil {
			return false, fmt.Errorf("insert packed content, hash %x-%x: %w", newHash.H1, newHash.H2, err)
		}
	}

	// content did change
	return true, nil
}
