package api

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/angelini/dateilager/internal/pb"
	"github.com/jackc/pgx/v4"
)

func buildQuery(project int32, vrange versionRange, objectQuery *pb.ObjectQuery) (string, []interface{}) {
	bytesSelector := "c.bytes"
	joinClause := `
		JOIN dl.contents c
		  ON o.hash = c.hash
	`
	if !objectQuery.WithContent {
		bytesSelector = "''::bytea AS bytes"
		joinClause = ""
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
			SELECT o.path, o.mode, 0 AS size, ''::bytea AS bytes, o.packed, true AS deleted
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
	tarReader := tar.NewReader(bytes.NewBuffer(content))

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return objects, nil
		}
		if err != nil {
			return nil, fmt.Errorf("unpack objects from TAR: %w", err)
		}

		var buffer bytes.Buffer
		_, err = io.Copy(&buffer, tarReader)
		if err != nil {
			return nil, err
		}

		objects = append(objects, &pb.Object{
			Path:    header.Name,
			Mode:    int32(header.Mode),
			Size:    int32(header.Size),
			Deleted: false,
			Content: buffer.Bytes(),
		})
	}
}

type objectStream func() (*pb.Object, error)

func getObjects(ctx context.Context, tx pgx.Tx, project int32, vrange versionRange, objectQuery *pb.ObjectQuery) (objectStream, error) {
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
			return object, nil
		}

		remaining := rows.Next()
		if !remaining {
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
			return object, nil
		}

		return &pb.Object{
			Path:    path,
			Mode:    mode,
			Size:    size,
			Deleted: deleted,
			Content: content,
		}, nil
	}, nil
}

func writeObjectToTar(tarWriter *tar.Writer, object *pb.Object) error {
	typeFlag := tar.TypeReg
	if object.Deleted {
		// Custom dateilager type flag to represent deleted files
		typeFlag = 'D'
	}

	header := &tar.Header{
		Name:     object.Path,
		Mode:     int64(object.Mode),
		Size:     int64(object.Size),
		Format:   tar.FormatPAX,
		Typeflag: byte(typeFlag),
	}

	err := tarWriter.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("write header to TAR %v: %w", object.Path, err)
	}

	_, err = tarWriter.Write(object.Content)
	if err != nil {
		return fmt.Errorf("write content to TAR %v: %w", object.Path, err)
	}

	return nil
}

type tarStream func() ([]byte, error)

func getTars(ctx context.Context, tx pgx.Tx, project int32, vrange versionRange, objectQuery *pb.ObjectQuery) (tarStream, error) {
	sql, args := buildQuery(project, vrange, objectQuery)
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("getObjects query, project %v vrange %v: %w", project, vrange, err)
	}

	var buffer bytes.Buffer
	tarWriter := tar.NewWriter(&buffer)
	currentSize := 0

	return func() ([]byte, error) {
		remaining := rows.Next()
		if !remaining {
			if currentSize > 0 {
				err = tarWriter.Close()
				if err != nil {
					return nil, fmt.Errorf("close tar writer: %w", err)
				}

				currentSize = 0
				return buffer.Bytes(), nil
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

		err = writeObjectToTar(tarWriter, &object)
		if err != nil {
			return nil, err
		}

		currentSize = currentSize + int(object.Size)
		if currentSize > TargetTarSize {
			err = tarWriter.Close()
			if err != nil {
				return nil, fmt.Errorf("close tar writer: %w", err)
			}

			output := buffer.Bytes()

			currentSize = 0
			buffer.Truncate(0)
			tarWriter = tar.NewWriter(&buffer)

			return output, nil
		}

		return nil, nil
	}, nil
}
