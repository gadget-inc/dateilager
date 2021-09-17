package db

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
)

const (
	TargetTarSize = 512 * 1024
)

var (
	//lint:ignore ST1012 All caps name to mimic io.EOF
	SKIP        = errors.New("Skip")
	ErrNotFound = errors.New("resource not found")
)

func ListProjects(ctx context.Context, tx pgx.Tx) ([]*pb.Project, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, latest_version
		FROM dl.projects
	`)
	if err != nil {
		return nil, fmt.Errorf("snapshotProjects query: %w", err)
	}

	projects := []*pb.Project{}

	for rows.Next() {
		var id, version int64
		err = rows.Scan(&id, &version)
		if err != nil {
			return nil, fmt.Errorf("snapshotProjects scan: %w", err)
		}
		projects = append(projects, &pb.Project{Id: id, Version: version})
	}

	return projects, nil
}

func getLatestVersion(ctx context.Context, tx pgx.Tx, project int64) (int64, error) {
	var latest_version int64

	err := tx.QueryRow(ctx, `
		SELECT latest_version
		FROM dl.projects WHERE id = $1
	`, project).Scan(&latest_version)
	if err == pgx.ErrNoRows {
		return -1, fmt.Errorf("get latest version for %v: %w", project, ErrNotFound)
	}
	if err != nil {
		return -1, fmt.Errorf("get latest version for %v: %w", project, err)
	}

	return latest_version, nil
}

func LockLatestVersion(ctx context.Context, tx pgx.Tx, project int64) (int64, error) {
	var latest_version int64

	err := tx.QueryRow(ctx, `
		SELECT latest_version
		FROM dl.projects WHERE id = $1
		FOR UPDATE
	`, project).Scan(&latest_version)
	if err == pgx.ErrNoRows {
		return -1, fmt.Errorf("lock latest version for %v: %w", project, ErrNotFound)
	}
	if err != nil {
		return -1, fmt.Errorf("lock latest version for %v: %w", project, err)
	}

	return latest_version, nil
}

type VersionRange struct {
	From int64
	To   int64
}

func NewVersionRange(ctx context.Context, tx pgx.Tx, project int64, from *int64, to *int64) (VersionRange, error) {
	vrange := VersionRange{}

	if from == nil {
		vrange.From = 0
	} else {
		vrange.From = *from
	}

	if to == nil {
		latest, err := getLatestVersion(ctx, tx, project)
		if err != nil {
			return vrange, err
		}
		vrange.To = latest
	} else {
		vrange.To = *to
	}

	return vrange, nil
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

	// If the array is empty Postgres will ignore every result
	// Ignoring the empty pattern is safe as object paths cannot be empty
	ignorePatterns := []string{""}
	for _, ignore := range objectQuery.Ignores {
		ignorePatterns = append(ignorePatterns, fmt.Sprintf("%s%%", ignore))
	}

	ignoreArray := &pgtype.TextArray{}
	ignoreArray.Set(ignorePatterns)

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
			  AND o.path NOT LIKE ALL($5::text[])
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
		project, vrange.From, vrange.To, path, ignoreArray,
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

func GetObjects(ctx context.Context, tx pgx.Tx, packManager *PackManager, project int64, vrange VersionRange, objectQuery *pb.ObjectQuery) (ObjectStream, error) {
	packParent := packManager.IsPathPacked(objectQuery.Path)

	originalPath := objectQuery.Path
	if packParent != nil {
		objectQuery.Path = *packParent
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

		if packParent != nil {
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

type PackManager struct {
	matchers []*regexp.Regexp
}

func NewPackManager(ctx context.Context, tx pgx.Tx, project int64) (*PackManager, error) {
	var patterns []string

	err := tx.QueryRow(ctx, `
		SELECT pack_patterns
		FROM dl.projects
		WHERE id = $1
	`, project).Scan(&patterns)
	if err != nil {
		return nil, fmt.Errorf("packManager query, project %v: %w", project, err)
	}

	var matchers []*regexp.Regexp
	for _, pattern := range patterns {
		matchers = append(matchers, regexp.MustCompile(pattern))
	}

	return &PackManager{
		matchers: matchers,
	}, nil
}

func (p *PackManager) IsPathPacked(path string) *string {
	currentPath := ""

	for _, split := range strings.Split(path, "/") {
		currentPath = fmt.Sprintf("%v%v/", currentPath, split)

		for _, matcher := range p.matchers {
			if matcher.MatchString(currentPath) {
				return &currentPath
			}
		}
	}

	return nil
}
