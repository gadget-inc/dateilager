package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4"
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
