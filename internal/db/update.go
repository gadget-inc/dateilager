package db

import (
	"context"
	"fmt"

	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/jackc/pgx/v4"
)

func DeleteObject(ctx context.Context, tx pgx.Tx, project int32, version int64, path string) error {
	_, err := tx.Exec(ctx, `
		UPDATE dl.objects
		SET stop_version = $1
		WHERE project = $2
		  AND path = $3
		  AND stop_version IS NULL
	`, version, project, path)
	if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}

	return nil
}

func DeleteObjects(ctx context.Context, tx pgx.Tx, project int32, version int64, path string) error {
	_, err := tx.Exec(ctx, `
		UPDATE dl.objects
		SET stop_version = $1
		WHERE project = $2
		  AND path LIKE $3
		  AND stop_version IS NULL
		RETURNING path;
	`, version, project, fmt.Sprintf("%s%%", path))
	if err != nil {
		return fmt.Errorf("delete objects: %w", err)
	}

	return nil
}

func UpdateObject(ctx context.Context, tx pgx.Tx, encoder *ContentEncoder, project int32, version int64, object *pb.Object) error {
	content := object.Content
	if content == nil {
		content = []byte("")
	}
	h1, h2 := HashContent(content)

	encoded, err := encoder.Encode(content)
	if err != nil {
		return fmt.Errorf("encode updated content: %w", err)
	}

	batch := &pgx.Batch{}

	batch.Queue(`
		UPDATE dl.objects SET stop_version = $1
		WHERE project = $2
			AND path = $3
			AND stop_version IS NULL
	`, version, project, object.Path)

	batch.Queue(`
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		VALUES ($1, $2, NULL, $3, ($4, $5), $6, $7, $8)
	`, project, version, object.Path, h1, h2, object.Mode, object.Size, false)

	batch.Queue(`
		INSERT INTO dl.contents (hash, bytes, names_tar)
		VALUES (($1, $2), $3, NULL)
		ON CONFLICT
		   DO NOTHING
	`, h1, h2, encoded)

	results := tx.SendBatch(ctx, batch)
	defer results.Close()

	_, err = results.Exec()
	if err != nil {
		return fmt.Errorf("update previous object version: %w", err)
	}

	_, err = results.Exec()
	if err != nil {
		return fmt.Errorf("insert new object version: %w", err)
	}

	_, err = results.Exec()
	if err != nil {
		return fmt.Errorf("insert updated content: %w", err)
	}

	return nil
}

func UpdatePackedObjects(ctx context.Context, tx pgx.Tx, project int32, version int64, parent string, updates []*pb.Object) error {
	var h1, h2 []byte
	var content []byte

	err := tx.QueryRow(ctx, `
		UPDATE dl.objects SET stop_version = $1
		WHERE project = $2
		  AND path = $3
		  AND packed IS true
		  AND stop_version IS NULL
		RETURNING (hash).h1, (hash).h2
	`, version, project, parent).Scan(&h1, &h2)
	if err != nil {
		return fmt.Errorf("update latest version: %w", err)
	}

	err = tx.QueryRow(ctx, `
		SELECT bytes
		FROM dl.contents
		WHERE (hash).h1 = $1
		  AND (hash).h2 = $2
	`, h1, h2).Scan(&content)
	if err != nil {
		return fmt.Errorf("fetch latest packed content: %w", err)
	}

	updated, namesTar, err := updateObjects(content, updates)
	if err != nil {
		return fmt.Errorf("update packed object: %w", err)
	}

	h1, h2 = HashContent(updated)
	batch := &pgx.Batch{}

	batch.Queue(`
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		VALUES ($1, $2, NULL, $3, ($4, $5), $6, $7, $8)
	`, project, version, parent, h1, h2, 0, len(updated), true)

	batch.Queue(`
		INSERT INTO dl.contents (hash, bytes, names_tar)
		VALUES (($1, $2), $3, $4)
		ON CONFLICT
		DO NOTHING
	`, h1, h2, updated, namesTar)

	results := tx.SendBatch(ctx, batch)
	defer results.Close()

	_, err = results.Exec()
	if err != nil {
		return fmt.Errorf("insert new packed object version: %w", err)
	}

	_, err = results.Exec()
	if err != nil {
		return fmt.Errorf("insert packed content: %w", err)
	}

	return nil
}

func InsertPackedObject(ctx context.Context, tx pgx.Tx, project int32, version int64, path string, contentTar, namesTar []byte) error {
	h1, h2 := HashContent(contentTar)
	batch := &pgx.Batch{}

	batch.Queue(`
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		VALUES ($1, $2, NULL, $3, ($4, $5), $6, $7, $8)
	`, project, version, path, h1, h2, 0, len(contentTar), true)

	batch.Queue(`
		INSERT INTO dl.contents (hash, bytes, names_tar)
		VALUES (($1, $2), $3, $4)
		ON CONFLICT
		DO NOTHING
	`, h1, h2, contentTar, namesTar)

	results := tx.SendBatch(ctx, batch)
	defer results.Close()

	_, err := results.Exec()
	if err != nil {
		return fmt.Errorf("insert new packed object: %w", err)
	}

	_, err = results.Exec()
	if err != nil {
		return fmt.Errorf("insert content: %w", err)
	}

	return nil
}
