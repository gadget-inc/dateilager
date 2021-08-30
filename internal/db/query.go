package db

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/jackc/pgx/v4"
)

const (
	TargetTarSize = 512 * 1024
)

var (
	//lint:ignore ST1012 All caps name to mimic io.EOF
	SKIP = errors.New("Skip")
)

type VersionRange struct {
	From int64
	To   int64
}

func buildQuery(project int64, vrange VersionRange, objectQuery *pb.ObjectQuery) (string, []interface{}) {
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
	if vrange.From == 0 {
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
			  AND o.path NOT IN (SELECT path FROM updated_files)
			ORDER BY o.path
		)
		SELECT path, mode, size, bytes, packed, deleted
		FROM updated_files
		%s;
	`

	query := fmt.Sprintf(sqlTemplate, bytesSelector, joinClause, pathPredicate, fetchDeleted)

	return query, []interface{}{
		project, vrange.From, vrange.To, path,
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

		objects = append(objects, pb.ObjectFromTarHeader(header, content))
	}
}

type ObjectStream func() (*pb.Object, error)

func filterObject(path string, objectQuery *pb.ObjectQuery, object *pb.Object) (*pb.Object, error) {
	if objectQuery.IsPrefix && strings.HasPrefix(object.Path, path) {
		return object, nil
	}

	if object.Path == path {
		return object, nil
	}

	return nil, SKIP
}

func GetObjects(ctx context.Context, tx pgx.Tx, packedCache *PackedCache, project int64, vrange VersionRange, objectQuery *pb.ObjectQuery) (ObjectStream, error) {
	parent, isPacked := packedCache.IsParentPacked(objectQuery.Path)

	originalPath := objectQuery.Path
	if isPacked {
		objectQuery.Path = parent
	}

	sql, args := buildQuery(project, vrange, objectQuery)
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("getObjects query, project %v vrange %v: %w", project, vrange, err)
	}

	var buffer []*pb.Object
	contentDecoder := NewContentDecoder()

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
		var mode, size int64
		var encoded []byte
		var packed bool
		var deleted bool

		err := rows.Scan(&path, &mode, &size, &encoded, &packed, &deleted)
		if err != nil {
			return nil, fmt.Errorf("getObjects scan, project %v vrange %v: %w", project, vrange, err)
		}

		if isPacked {
			buffer, err = unpackObjects(encoded)
			if err != nil {
				return nil, err
			}

			object := buffer[0]
			buffer = buffer[1:]
			return filterObject(originalPath, objectQuery, object)
		}

		content, err := contentDecoder.Decoder(encoded)
		if err != nil {
			return nil, fmt.Errorf("getObjects decode content %v: %w", path, err)
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

func GetTars(ctx context.Context, tx pgx.Tx, project int64, vrange VersionRange, objectQuery *pb.ObjectQuery) (tarStream, error) {
	sql, args := buildQuery(project, vrange, objectQuery)
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("getObjects query, project %v vrange %v: %w", project, vrange, err)
	}

	tarWriter := NewTarWriter()
	contentDecoder := NewContentDecoder()

	return func() ([]byte, error) {
		if !rows.Next() {
			if tarWriter.Size() > 0 {
				return tarWriter.BytesAndReset()
			}

			return nil, io.EOF
		}

		var path string
		var mode, size int64
		var encoded []byte
		var packed bool
		var deleted bool

		err := rows.Scan(&path, &mode, &size, &encoded, &packed, &deleted)
		if err != nil {
			return nil, fmt.Errorf("getTars scan, project %v vrange %v: %w", project, vrange, err)
		}

		if packed {
			return encoded, nil
		}

		content, err := contentDecoder.Decoder(encoded)
		if err != nil {
			return nil, fmt.Errorf("getTars decode content %v: %w", path, err)
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

		return nil, SKIP
	}, nil
}

type PackedCache struct {
	packs map[string]bool
}

func NewPackedCache(ctx context.Context, tx pgx.Tx, project int64, vrange VersionRange) (*PackedCache, error) {
	sql := `
		SELECT o.path
		FROM dl.objects o
		WHERE o.project = $1
		  AND o.start_version > $2
		  AND o.start_version <= $3
		  AND (o.stop_version IS NULL OR o.stop_version > $3)
		  AND o.packed IS true
	`

	rows, err := tx.Query(ctx, sql, project, vrange.From, vrange.To)
	if err != nil {
		return nil, fmt.Errorf("packedCache query, project %v, vrange %v: %w", project, vrange, err)
	}

	packs := make(map[string]bool)

	for rows.Next() {
		var path string
		err = rows.Scan(&path)
		if err != nil {
			return nil, fmt.Errorf("packedCache scan, project %v, vrange %v: %w", project, vrange, err)
		}

		packs[path] = true
	}

	return &PackedCache{
		packs: packs,
	}, nil
}

func (p *PackedCache) IsParentPacked(path string) (string, bool) {
	currentPath := ""

	for _, split := range strings.Split(path, "/") {
		currentPath = fmt.Sprintf("%v%v/", currentPath, split)

		_, ok := p.packs[currentPath]
		if ok {
			return currentPath, true
		}
	}

	return "", false
}
