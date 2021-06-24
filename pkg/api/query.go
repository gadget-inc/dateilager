package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/angelini/dateilager/internal/pb"
	"github.com/jackc/pgx/v4"
)

const (
	TargetTarSize = 512 * 1024
)

var (
	SKIP = errors.New("Skip")
)

func buildQuery(project int32, vrange versionRange, objectQuery *pb.ObjectQuery) (string, []interface{}) {
	bytesSelector := "c.bytes"
	joinClause := `
		JOIN dl.contents c
		  ON o.hash = c.hash
	`

	if !objectQuery.WithContent {
		bytesSelector = "c.names_tar as bytes"
		joinClause = `
			LEFT JOIN dl.contents c
			       ON o.hash = c.hash
				  AND o.packed IS true
		`
	}

	path := objectQuery.Path
	pathPredicate := "o.path = $4"
	if objectQuery.IsPrefix {
		path = fmt.Sprintf("%s%%", objectQuery.Path)
		pathPredicate = "o.path LIKE $4"
	}

	fetchDeleted := `
		UNION
		SELECT path, mode, size, bytes, packed, deleted
		FROM removed_files
	`
	if vrange.from == 0 {
		fetchDeleted = ""
	}

	sqlTemplate := `
		WITH updated_files AS (
			SELECT o.path, o.mode, o.size, %s, o.packed, false AS deleted
			FROM dl.objects o
			%s
			WHERE o.project = $1
			  AND o.start_version > $2
			  AND o.start_version <= $3
			  AND (o.stop_version IS NULL OR o.stop_version > $3)
			  AND %s
			ORDER BY o.path
		), removed_files AS (
			SELECT o.path, o.mode, 0 AS size, ''::bytea as bytes, o.packed, true AS deleted
			FROM dl.objects o
			WHERE o.project = $1
			  AND o.start_version <= $3
			  AND o.stop_version > $2
			  AND o.stop_version <= $3
			  AND o.path not in (SELECT path FROM updated_files)
			ORDER BY o.path
		)
		SELECT path, mode, size, bytes, packed, deleted
		FROM updated_files
		%s;
	`

	query := fmt.Sprintf(sqlTemplate, bytesSelector, joinClause, pathPredicate, fetchDeleted)

	return query, []interface{}{
		project, vrange.from, vrange.to, path,
	}
}

func unpackObjects(content []byte) ([]*pb.Object, error) {
	var objects []*pb.Object
	tarReader := NewTarReader(content)
	defer tarReader.Close()

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return objects, nil
		}
		if err != nil {
			return nil, fmt.Errorf("unpack objects from TAR: %w", err)
		}

		content, err := tarReader.ReadContent()
		if err != nil {
			return nil, err
		}

		objects = append(objects, &pb.Object{
			Path:    header.Name,
			Mode:    int32(header.Mode),
			Size:    int32(header.Size),
			Deleted: false,
			Content: content,
		})
	}
}

type objectStream func() (*pb.Object, error)

func filterObject(path string, objectQuery *pb.ObjectQuery, object *pb.Object) (*pb.Object, error) {
	if objectQuery.IsPrefix && strings.HasPrefix(object.Path, path) {
		return object, nil
	}

	if object.Path == path {
		return object, nil
	}

	return nil, SKIP
}

func getObjects(ctx context.Context, tx pgx.Tx, project int32, vrange versionRange, objectQuery *pb.ObjectQuery) (objectStream, error) {
	parent, err := isParentPacked(ctx, tx, project, vrange, objectQuery)
	if err != nil {
		return nil, fmt.Errorf("getObjects searching for packed parents, project %v vrange %v: %w", project, vrange, err)
	}

	originalPath := objectQuery.Path
	if parent != "" {
		objectQuery.Path = parent
	}

	sql, args := buildQuery(project, vrange, objectQuery)
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("getObjects query, project %v vrange %v: %w", project, vrange, err)
	}

	var buffer []*pb.Object

	return func() (*pb.Object, error) {
		if len(buffer) > 0 {
			object := buffer[0]
			buffer = buffer[1:]
			return filterObject(originalPath, objectQuery, object)
		}

		if !rows.Next() {
			return nil, io.EOF
		}

		var path string
		var mode, size int32
		var content []byte
		var packed bool
		var deleted bool

		err := rows.Scan(&path, &mode, &size, &content, &packed, &deleted)
		if err != nil {
			return nil, fmt.Errorf("getObjects scan, project %v vrange %v: %w", project, vrange, err)
		}

		if packed {
			buffer, err = unpackObjects(content)
			if err != nil {
				return nil, err
			}

			object := buffer[0]
			buffer = buffer[1:]
			return filterObject(originalPath, objectQuery, object)
		}

		return filterObject(originalPath, objectQuery, &pb.Object{
			Path:    path,
			Mode:    mode,
			Size:    size,
			Deleted: deleted,
			Content: content,
		})
	}, nil
}

type tarStream func() ([]byte, error)

func getTars(ctx context.Context, tx pgx.Tx, project int32, vrange versionRange, objectQuery *pb.ObjectQuery) (tarStream, error) {
	sql, args := buildQuery(project, vrange, objectQuery)
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("getObjects query, project %v vrange %v: %w", project, vrange, err)
	}

	tarWriter := NewTarWriter()

	return func() ([]byte, error) {
		if !rows.Next() {
			if tarWriter.Size() > 0 {
				return tarWriter.BytesAndReset()
			}

			return nil, io.EOF
		}

		var path string
		var mode, size int32
		var content []byte
		var packed bool
		var deleted bool

		err := rows.Scan(&path, &mode, &size, &content, &packed, &deleted)
		if err != nil {
			return nil, fmt.Errorf("getTars scan, project %v vrange %v: %w", project, vrange, err)
		}

		if packed {
			return content, nil
		}

		object := pb.Object{
			Path:    path,
			Mode:    mode,
			Size:    size,
			Deleted: deleted,
			Content: content,
		}

		err = tarWriter.WriteObject(&object, true)
		if err != nil {
			return nil, err
		}

		if tarWriter.Size() > TargetTarSize {
			return tarWriter.BytesAndReset()
		}

		return nil, nil
	}, nil
}

func isParentPacked(ctx context.Context, tx pgx.Tx, project int32, vrange versionRange, objectQuery *pb.ObjectQuery) (string, error) {
	sql := `
		SELECT o.path
		FROM dl.objects o
		WHERE o.project = $1
		  AND o.start_version > $2
		  AND o.start_version <= $3
		  AND (o.stop_version IS NULL OR o.stop_version > $3)
		  AND o.packed IS true
		  AND (%s)
	`

	var parents []string
	for _, split := range strings.Split(path.Dir(objectQuery.Path), "/") {
		if split == "" {
			continue
		}

		if len(parents) == 0 {
			parents = append(parents, fmt.Sprintf("/%s/", split))
		} else {
			parents = append(parents, fmt.Sprintf("%s%s/", parents[len(parents)-1], split))
		}
	}

	if len(parents) == 0 {
		return "", nil
	}

	args := []interface{}{
		project, vrange.from, vrange.to,
	}

	var predicate string
	for idx, parent := range parents {
		if idx > 0 {
			predicate += " OR "
		}
		predicate += fmt.Sprintf("o.path = $%d", idx+4)

		args = append(args, parent)
	}

	var path string
	err := tx.QueryRow(ctx, fmt.Sprintf(sql, predicate), args...).Scan(&path)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	return path, nil
}
