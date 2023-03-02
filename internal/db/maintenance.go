package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func VacuumAnalyze(ctx context.Context, conn *pgx.Conn, workers int64, table string) error {
	prepared_statement := fmt.Sprintf("VACUUM (ANALYZE, PARALLEL %d) %s", workers, table)

	_, err := conn.Exec(ctx, prepared_statement)
	if err != nil {
		return fmt.Errorf("failed to vacuum database: QUERY: %s, err: %w", prepared_statement, err)
	}

	return nil
}
