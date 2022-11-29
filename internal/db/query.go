package db

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/jackc/pgx/v5"
)

const (
	TargetTarSize = 512 * 1024
	chunkSize     = 200
)

var (
	//lint:ignore ST1012 All caps name to mimic io.EOF
	SKIP        = errors.New("Skip")
	ErrNotFound = errors.New("resource not found")
)

type EncodedContent = []byte
type DecodedContent = []byte

type DbObject struct {
	hash    Hash
	path    string
	mode    int64
	size    int64
	deleted bool
	cached  bool
	packed  bool
}

func (d *DbObject) ToTarObject(content []byte) TarObject {
	if d.cached {
		content = d.hash.Bytes()
	}

	return TarObject{
		path:    d.path,
		mode:    d.mode,
		size:    d.size,
		deleted: d.deleted,
		cached:  d.cached,
		packed:  d.packed,
		content: content,
	}
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
	tarReader := NewTarReader()
	tarReader.FromBytes(content)

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

func filterObject(path string, objectQuery *pb.ObjectQuery, object *pb.Object) (*pb.Object, error) {
	if objectQuery.IsPrefix && strings.HasPrefix(object.Path, path) {
		return object, nil
	}

	if object.Path == path {
		return object, nil
	}

	return nil, SKIP
}

func executeQuery(ctx context.Context, tx pgx.Tx, queryBuilder *queryBuilder) ([]DbObject, error) {
	sql, args := queryBuilder.build()
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dbObjects []DbObject
	for {
		if !rows.Next() {
			break
		}

		var object DbObject

		err := rows.Scan(&object.path, &object.mode, &object.size, &object.cached, &object.packed, &object.deleted, &object.hash.H1, &object.hash.H2)
		if err != nil {
			return nil, err
		}

		dbObjects = append(dbObjects, object)
	}

	return dbObjects, nil
}

func loadChunk(ctx context.Context, tx pgx.Tx, lookup *ContentLookup, dbObjects []DbObject, startIdx int, chunkSize int) ([]DecodedContent, error) {
	hashes := make(map[Hash]bool, chunkSize)

	for idx := 0; idx < chunkSize && idx+startIdx < len(dbObjects); idx++ {
		dbObject := dbObjects[idx+startIdx]

		if !dbObject.cached {
			hashes[dbObject.hash] = !dbObject.packed
		}
	}

	uncachedContents, err := lookup.Lookup(ctx, tx, hashes)
	if err != nil {
		return nil, err
	}

	var decoded []DecodedContent

	for idx := 0; idx < chunkSize && idx+startIdx < len(dbObjects); idx++ {
		dbObject := dbObjects[idx+startIdx]
		if dbObject.cached {
			decoded = append(decoded, dbObject.hash.Bytes())
		} else {
			decoded = append(decoded, uncachedContents[dbObject.hash])
		}
	}

	return decoded, nil
}

type ObjectStream func() (*pb.Object, error)

func GetObjects(ctx context.Context, tx pgx.Tx, lookup *ContentLookup, packManager *PackManager, project int64, vrange VersionRange, objectQuery *pb.ObjectQuery) (ObjectStream, error) {
	packParent := packManager.IsPathPacked(objectQuery.Path)
	originalPath := objectQuery.Path
	if packParent != nil {
		objectQuery.Path = *packParent
	}

	builder := newQueryBuilder(project, vrange, objectQuery)
	dbObjects, err := executeQuery(ctx, tx, builder)
	if err != nil {
		return nil, fmt.Errorf("get objects query, project %v vrange %v: %w", project, vrange, err)
	}

	idx := 0
	chunkIdx := 0
	chunk, err := loadChunk(ctx, tx, lookup, dbObjects, idx, chunkSize)
	if err != nil {
		return nil, fmt.Errorf("failed to load chunk: %w", err)
	}

	var packBuffer []*pb.Object

	return func() (*pb.Object, error) {
		if len(packBuffer) > 0 {
			object := packBuffer[0]
			packBuffer = packBuffer[1:]
			return filterObject(originalPath, objectQuery, object)
		}

		if idx >= len(dbObjects) {
			return nil, io.EOF
		}

		if chunkIdx >= len(chunk) {
			chunkIdx = 0
			chunk, err = loadChunk(ctx, tx, lookup, dbObjects, idx, chunkSize)
			if err != nil {
				return nil, fmt.Errorf("failed to load chunk: %w", err)
			}
		}

		dbObject := dbObjects[idx]
		content := chunk[chunkIdx]

		idx += 1
		chunkIdx += 1

		if dbObject.cached {
			return nil, fmt.Errorf("getObjects scan, project %v vrange %v: returned non-nil hash when queried without cache", project, vrange)
		}

		if dbObject.packed {
			packBuffer, err = unpackObjects(content)
			if err != nil {
				return nil, err
			}

			object := packBuffer[0]
			packBuffer = packBuffer[1:]
			return filterObject(originalPath, objectQuery, object)
		}

		return filterObject(originalPath, objectQuery, &pb.Object{
			Path:    dbObject.path,
			Mode:    dbObject.mode,
			Size:    dbObject.size,
			Deleted: dbObject.deleted,
			Content: content,
		})
	}, nil
}

type tarStream func() ([]byte, *string, error)

func GetTars(ctx context.Context, tx pgx.Tx, lookup *ContentLookup, project int64, cacheVersions []int64, vrange VersionRange, objectQuery *pb.ObjectQuery) (tarStream, error) {
	builder := newQueryBuilder(project, vrange, objectQuery).withCacheVersions(cacheVersions)
	dbObjects, err := executeQuery(ctx, tx, builder)
	if err != nil {
		return nil, fmt.Errorf("get tars query, project %v vrange %v: %w", project, vrange, err)
	}

	tarWriter := NewTarWriter()

	idx := 0
	chunkIdx := 0
	chunk, err := loadChunk(ctx, tx, lookup, dbObjects, idx, chunkSize)
	if err != nil {
		return nil, fmt.Errorf("failed to load chunk: %w", err)
	}

	return func() ([]byte, *string, error) {
		if idx >= len(dbObjects) {
			if tarWriter.Size() > 0 {
				bytes, err := tarWriter.BytesAndReset()
				return bytes, nil, err
			}

			return nil, nil, io.EOF
		}

		if chunkIdx >= len(chunk) {
			chunkIdx = 0
			chunk, err = loadChunk(ctx, tx, lookup, dbObjects, idx, chunkSize)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to load chunk: %w", err)
			}
		}

		dbObject := dbObjects[idx]
		content := chunk[chunkIdx]

		idx += 1
		chunkIdx += 1

		if dbObject.packed && !dbObject.cached {
			return content, &dbObject.path, nil
		}

		tarObject := dbObject.ToTarObject(content)
		err = tarWriter.WriteObject(&tarObject)
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

type cacheTarStream func() (int64, []byte, *Hash, error)

func GetCacheTars(ctx context.Context, tx pgx.Tx) (cacheTarStream, CloseFunc, error) {
	var version int64

	err := tx.QueryRow(ctx, `
		SELECT version
		FROM dl.cache_versions
		ORDER BY version DESC
		LIMIT 1
	`).Scan(&version)
	if err == pgx.ErrNoRows {
		return func() (int64, []byte, *Hash, error) { return 0, nil, nil, io.EOF }, func(_ context.Context) {}, nil
	}
	if err != nil {
		return nil, func(_ context.Context) {}, fmt.Errorf("GetCacheTars latest cache version: %w", err)
	}

	rows, err := tx.Query(ctx, `
		WITH version_hashes AS (
			SELECT unnest(hashes) AS hash
			FROM dl.cache_versions
			WHERE version = $1
		)
		SELECT (h.hash).h1, (h.hash).h2, c.bytes
		FROM version_hashes h
		JOIN dl.contents c
		  ON h.hash = c.hash
	`, version)
	closeFunc := func(_ context.Context) { rows.Close() }
	if err != nil {
		return nil, closeFunc, fmt.Errorf("GetCacheTars query: %w", err)
	}

	return func() (int64, []byte, *Hash, error) {
		if !rows.Next() {
			return 0, nil, nil, io.EOF
		}

		var hash Hash
		var encoded []byte

		err := rows.Scan(&hash.H1, &hash.H2, &encoded)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("GetCacheTars scan: %w", err)
		}

		return version, encoded, &hash, nil
	}, closeFunc, nil
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
