package db

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/jackc/pgx/v5"
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

func HasSamePackPattern(ctx context.Context, tx pgx.Tx, project_1 int64, project_2 int64) (bool, error) {
	var samePackPatterns bool

	err := tx.QueryRow(ctx, `
		SELECT COALESCE((SELECT pack_patterns FROM dl.projects WHERE id = $1), '{}') =
		       COALESCE((SELECT pack_patterns FROM dl.projects WHERE id = $2), '{}');
	`, project_1, project_2).Scan(&samePackPatterns)
	if err != nil {
		return false, fmt.Errorf("check matching pack patterns, source %v, target %v: %w", project_1, project_2, err)
	}

	return samePackPatterns, nil
}

func getLatestVersion(ctx context.Context, tx pgx.Tx, project int64) (int64, error) {
	var latestVersion int64

	err := tx.QueryRow(ctx, `
		SELECT latest_version
		FROM dl.projects WHERE id = $1
	`, project).Scan(&latestVersion)
	if err == pgx.ErrNoRows {
		return -1, fmt.Errorf("get latest version for %v: %w", project, ErrNotFound)
	}
	if err != nil {
		return -1, fmt.Errorf("get latest version for %v: %w", project, err)
	}

	return latestVersion, nil
}

func LockLatestVersion(ctx context.Context, tx pgx.Tx, project int64) (int64, error) {
	var latestVersion int64

	err := tx.QueryRow(ctx, `
		SELECT latest_version
		FROM dl.projects WHERE id = $1
		FOR UPDATE
	`, project).Scan(&latestVersion)
	if err == pgx.ErrNoRows {
		return -1, fmt.Errorf("lock latest version for %v: %w", project, ErrNotFound)
	}
	if err != nil {
		return -1, fmt.Errorf("lock latest version for %v: %w", project, err)
	}

	return latestVersion, nil
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

func unpackObjects(content []byte) ([]*pb.Object, error) {
	var objects []*pb.Object
	tarReader := NewTarReader(content)

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

	builder := newQueryBuilder(project, vrange, objectQuery, nil, false)
	sql, args := builder.build()
	rows, err := tx.Query(ctx, sql, args...)

	if err != nil {
		return nil, fmt.Errorf("1getObjects query, project %v vrange %v: %w", project, vrange, err)
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
		var cache_h1, cache_h2 []byte
		var encoded []byte
		var packed bool
		var deleted bool
		var h1, h2 []byte

		err := rows.Scan(&path, &mode, &size, &cache_h1, &cache_h2, &encoded, &packed, &deleted, &h1, &h2)
		if err != nil {
			return nil, fmt.Errorf("getObjects scan, project %v vrange %v: %w", project, vrange, err)
		}

		if cache_h1 != nil {
			return nil, fmt.Errorf("getObjects scan, project %v vrange %v: returned non-nil hash when queried without cache", project, vrange)
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

type tarStream func() ([]byte, *string, error)

func GetTars(ctx context.Context, tx pgx.Tx, project int64, availableCacheVersions []int64, vrange VersionRange, objectQuery *pb.ObjectQuery) (tarStream, error) {
	builder := newQueryBuilder(project, vrange, objectQuery, availableCacheVersions, false)
	sql, args := builder.build()
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("getObjects query, project %v vrange %v: %w", project, vrange, err)
	}

	tarWriter := NewTarWriter()
	contentDecoder := NewContentDecoder()

	return func() ([]byte, *string, error) {
		if !rows.Next() {
			if tarWriter.Size() > 0 {
				bytes, err := tarWriter.BytesAndReset()
				return bytes, nil, err
			}

			return nil, nil, io.EOF
		}

		var path string
		var mode, size int64
		var cache_h1, cache_h2 []byte
		var cache_hash []byte
		var encoded []byte
		var packed bool
		var deleted bool
		var h1, h2 []byte

		err := rows.Scan(&path, &mode, &size, &cache_h1, &cache_h2, &encoded, &packed, &deleted, &h1, &h2)
		if err != nil {
			return nil, nil, fmt.Errorf("getTars scan, project %v vrange %v: %w", project, vrange, err)
		}

		if cache_h1 != nil {
			cache_hash = make([]byte, 32)
			copy(cache_hash[0:16], cache_h1)
			copy(cache_hash[16:], cache_h2)
		}

		if packed && cache_hash == nil {
			return encoded, &path, nil
		}

		var content []byte
		var cached = false

		if cache_hash != nil {
			content = cache_hash
			cached = true
		} else {
			content, err = contentDecoder.Decoder(encoded)
			if err != nil {
				return nil, nil, fmt.Errorf("getTars decode content %v: %w", path, err)
			}
		}

		object := pb.Object{
			Path:    path,
			Mode:    mode,
			Size:    size,
			Deleted: deleted,
			Content: content,
			Cached:  cached,
		}

		err = tarWriter.WriteObject(&object, true)
		if err != nil {
			return nil, nil, err
		}

		if tarWriter.Size() > TargetTarSize {
			bytes, err := tarWriter.BytesAndReset()
			return bytes, nil, err
		}

		return nil, nil, SKIP
	}, nil
}

func latestCacheTarsQuery() string {
	return `
		WITH latest_cache_version AS (
			SELECT version, hashes
			FROM dl.cache_versions
			ORDER BY version DESC LIMIT 1
		),
		version_hash AS (
			SELECT version, unnest(hashes) AS hash
			FROM latest_cache_version
		)
		SELECT version, (version_hash.hash).h1, (version_hash.hash).h2, bytes
		FROM version_hash
		JOIN dl.contents
		ON version_hash.hash = dl.contents.hash
	`
}

type cacheTarStream func() (int64, []byte, []byte, error)

func GetCacheTars(ctx context.Context, tx pgx.Tx) (cacheTarStream, error) {
	rows, err := tx.Query(ctx, latestCacheTarsQuery())
	if err != nil {
		return nil, fmt.Errorf("GetCachedTars query: %w", err)
	}

	var version int64

	stream := func() (int64, []byte, []byte, error) {
		if !rows.Next() {
			return 0, nil, nil, io.EOF
		}
		var rowVersion int64
		var h1, h2 []byte
		var encoded []byte

		err := rows.Scan(&rowVersion, &h1, &h2, &encoded)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("GetCacheTars scan: %w", err)
		}
		if version != 0 && rowVersion != version {
			return 0, nil, nil, fmt.Errorf("GetCacheTars rowVersion mismatch: %d vs %d", version, rowVersion)
		}

		version = rowVersion

		hash := make([]byte, 32)
		copy(hash[0:16], h1)
		copy(hash[16:], h2)

		return rowVersion, encoded, hash, nil
	}

	return stream, nil
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

func CreateCache(ctx context.Context, tx pgx.Tx, prefix string) (int64, error) {
	prefix = filepath.Join(prefix, "%")
	sql := `
		WITH impactful_packed_objects AS (
			SELECT hash, count(*) as count
			FROM dl.objects
			WHERE path LIKE $1
				AND packed = true
				AND stop_version IS NULL
			GROUP BY hash
			ORDER BY count DESC
			LIMIT 100
		)
		INSERT INTO dl.cache_versions (hashes)
		SELECT COALESCE(array_agg(hash), '{}') as hashes from impactful_packed_objects
		RETURNING version
`

	row := tx.QueryRow(ctx, sql, prefix)
	var version int64

	err := row.Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("CreateCache query, %w", err)
	}

	return version, nil
}
