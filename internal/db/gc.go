package db

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
)

func GcProjectObjects(ctx context.Context, tx pgx.Tx, project int64, keep int64, fromVersion int64) ([]Hash, error) {
	hashes := []Hash{}

	rows, err := tx.Query(ctx, `
		WITH latest AS (
			SELECT latest_version AS version
			FROM dl.projects
			WHERE id = $1
		)
		DELETE FROM dl.objects
		WHERE project = $1
		  AND start_version > $2
		  AND stop_version IS NOT NULL
		  AND stop_version <= ((SELECT version FROM latest) - $3)
		RETURNING (hash).h1, (hash).h2
	`, project, fromVersion, keep)
	if err != nil {
		return hashes, fmt.Errorf("GcProjectObjects query, project %v, keep %v, from %v: %w", project, keep, fromVersion, err)
	}

	for rows.Next() {
		var hash Hash
		err = rows.Scan(&hash.H1, &hash.H2)
		if err != nil {
			return hashes, fmt.Errorf("GcProjectObjects scan %v: %w", project, err)
		}

		hashes = append(hashes, hash)
	}

	return hashes, nil
}

func GcContentHashes(ctx context.Context, tx pgx.Tx, hashes []Hash) (int64, error) {
	count := int64(0)

	connInfo := tx.Conn().ConnInfo()
	dtype, ok := connInfo.DataTypeForValue(hashes)
	if ok {
		fmt.Printf("name: %v, oid: %v\n", dtype.Name, dtype.OID)
		fmt.Printf("encodeHashes(hashes)[0]: %v\n\n", encodeHashes(hashes)[0])

		err := dtype.Value.Set(encodeHashes(hashes))
		if err != nil {
			log.Fatal("cannot set _hash dtype: ", err)
		}
		fmt.Println("did I make it here?")
	} else {
		fmt.Println("invalid dtype for hashes")
	}

	// pgx.QuerySimpleProtocol(true),
	tag, err := tx.Exec(ctx, `
		WITH missing AS (
			SELECT hash
			FROM dl.objects
			WHERE hash = ANY($1::hash[])
			GROUP BY hash
			HAVING count(*) = 0
		)
		DELETE FROM dl.contents
		WHERE hash IN (SELECT hash from missing)
	`, encodeHashes(hashes))
	if err != nil {
		return count, fmt.Errorf("GcContentHashes query, hash count %v: %w", len(hashes), err)
	}

	return tag.RowsAffected(), nil
}

func encodeHashes(hashes []Hash) []pgtype.CompositeFields {
	fmt.Printf("len(hashes): %v\n", len(hashes))
	fields := make([]pgtype.CompositeFields, len(hashes))

	for idx, hash := range hashes {
		entry := pgtype.CompositeFields{hash.H1, hash.H2}
		fmt.Printf("entry: %v\n", entry)
		fmt.Printf("entry[0]: %v, entry[1]: %v\n", entry[0], entry[1])
		fields = append(fields, entry)
		fmt.Printf("fields[%v]: %v\n", idx, fields[idx])
	}

	fmt.Printf("fields[0]: %v\n", fields[0])
	return fields
}
