package test

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/files"
	"github.com/gadget-inc/dateilager/internal/pb"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/gadget-inc/dateilager/pkg/api"
	"github.com/gadget-inc/dateilager/pkg/client"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/klauspost/compress/s2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type Type int

const (
	bufSize       = 1024 * 1024
	symlinkMode   = 0755 | int64(fs.ModeSymlink)
	directoryMode = 0755 | int64(fs.ModeDir)
)

const (
	typeRegular Type = iota
	typeDirectory
	typeSymlink
)

func i(i int64) *int64 {
	return &i
}

type expectedObject struct {
	content string
	deleted bool
	mode    int64
}

type expectedFile struct {
	content  string
	fileType Type
}

type expectedResponse struct {
	version int64
	count   uint32
}

var (
	emptyVersionRange = client.VersionRange{From: nil, To: nil}
)

func toVersion(to int64) client.VersionRange {
	return client.VersionRange{From: nil, To: &to}
}

func fromVersion(from int64) client.VersionRange {
	return client.VersionRange{From: &from, To: nil}
}

func writeProject(tc util.TestCtx, id int64, latestVersion int64, packPatterns ...string) {
	conn := tc.Connect()
	_, err := conn.Exec(tc.Context(), `
		INSERT INTO dl.projects (id, latest_version, pack_patterns)
		VALUES ($1, $2, $3)
	`, id, latestVersion, packPatterns)
	require.NoError(tc.T(), err, "insert project")
}

func countObjects(tc util.TestCtx) int {
	conn := tc.Connect()

	var count int
	err := conn.QueryRow(tc.Context(), `
		SELECT count(*)
		FROM dl.objects
	`).Scan(&count)
	require.NoError(tc.T(), err, "count objects")

	return count
}

func countContents(tc util.TestCtx) int {
	conn := tc.Connect()

	var count int
	err := conn.QueryRow(tc.Context(), `
		SELECT count(*)
		FROM dl.contents
	`).Scan(&count)
	require.NoError(tc.T(), err, "count contents")

	return count
}

func writeObjectFull(tc util.TestCtx, project int64, start int64, stop *int64, path, content string, mode fs.FileMode) {
	conn := tc.Connect()

	contentBytes := []byte(content)
	hash := db.HashContent(contentBytes)

	_, err := conn.Exec(tc.Context(), `
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		VALUES ($1, $2, $3, $4, ($5, $6), $7, $8, $9)
	`, project, start, stop, path, hash.H1, hash.H2, mode, len(contentBytes), false)
	require.NoError(tc.T(), err, "insert object")

	contentEncoder := db.NewContentEncoder()
	defer contentEncoder.Close()

	encoded, err := contentEncoder.Encode(contentBytes)
	require.NoError(tc.T(), err, "encode content")

	_, err = conn.Exec(tc.Context(), `
		INSERT INTO dl.contents (hash, bytes)
		VALUES (($1, $2), $3)
		ON CONFLICT
		   DO NOTHING
	`, hash.H1, hash.H2, encoded)
	require.NoError(tc.T(), err, "insert contents")
}

func writeObject(tc util.TestCtx, project int64, start int64, stop *int64, path string, contents ...string) {
	var content string
	if len(contents) == 0 {
		content = ""
	} else {
		content = contents[0]
	}

	writeObjectFull(tc, project, start, stop, path, content, 0755)
}

func deleteObject(tc util.TestCtx, project int64, start int64, path string) {
	conn := tc.Connect()

	_, err := conn.Exec(tc.Context(), `
		DELETE
		FROM dl.objects
		WHERE project = $1
		  AND start_version = $2
		  AND path = $3
	`, project, start, path)
	require.NoError(tc.T(), err, "delete object")
}

func writeEmptyDir(tc util.TestCtx, project int64, start int64, stop *int64, path string) {
	mode := fs.FileMode(0755)
	mode |= fs.ModeDir

	writeObjectFull(tc, project, start, stop, path, "", mode)
}

func writeSymlink(tc util.TestCtx, project int64, start int64, stop *int64, path, target string) {
	mode := fs.FileMode(0755)
	mode |= fs.ModeSymlink

	writeObjectFull(tc, project, start, stop, path, target, mode)
}

func writePackedObjects(tc util.TestCtx, project int64, start int64, stop *int64, path string, objects map[string]expectedObject) db.Hash {
	conn := tc.Connect()

	contentsTar := packObjects(tc, objects)
	hash := db.HashContent(contentsTar)

	_, err := conn.Exec(tc.Context(), `
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		VALUES ($1, $2, $3, $4, ($5, $6), $7, $8, $9)
	`, project, start, stop, path, hash.H1, hash.H2, 0755, len(contentsTar), true)
	require.NoError(tc.T(), err, "insert object")

	_, err = conn.Exec(tc.Context(), `
		INSERT INTO dl.contents (hash, bytes)
		VALUES (($1, $2), $3)
		ON CONFLICT
		DO NOTHING
	`, hash.H1, hash.H2, contentsTar)
	require.NoError(tc.T(), err, "insert contents")

	return hash
}

func writePackedFiles(tc util.TestCtx, project int64, start int64, stop *int64, path string) db.Hash {
	return writePackedObjects(tc, project, start, stop, path, map[string]expectedObject{
		filepath.Join(path, "1"): {content: fmt.Sprintf("%s v%d", filepath.Join(path, "1"), start)},
		filepath.Join(path, "2"): {content: fmt.Sprintf("%s v%d", filepath.Join(path, "2"), start)},
	})
}

func packObjects(tc util.TestCtx, objects map[string]expectedObject) []byte {
	contentWriter := db.NewTarWriter()

	for path, info := range objects {
		mode := info.mode
		if mode == 0 {
			mode = 0755
		}

		object := db.NewUncachedTarObject(path, mode, int64(len(info.content)), info.deleted, []byte(info.content))

		err := contentWriter.WriteObject(&object)
		require.NoError(tc.T(), err, "write content to TAR")
	}

	contentTar, err := contentWriter.BytesAndReset()
	require.NoError(tc.T(), err, "write content TAR to bytes")

	return contentTar
}

// verifyObjects asserts that the given objects contain all the expected paths and file contents
func verifyObjects(t *testing.T, objects []*pb.Object, expected map[string]string) {
	contents := make(map[string]string)

	for _, object := range objects {
		contents[object.Path] = string(object.Content)
	}

	// This gives in a much nicer diff in the error message over asserting each object separately.
	assert.EqualValues(t, expected, contents, "unexpected contents for objects")
}

func writeFile(t *testing.T, dir string, path string, content string) {
	fullPath := filepath.Join(dir, path)
	err := os.MkdirAll(filepath.Dir(fullPath), 0755)
	require.NoError(t, err, "mkdir %v", filepath.Dir(fullPath))
	err = os.WriteFile(fullPath, []byte(content), 0755)
	require.NoError(t, err, "write file %v", path)
}

func emptyTmpDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "dateilager_tests_")
	require.NoError(t, err, "create temp dir")

	return dir
}

func writeTmpFiles(t *testing.T, version int64, files map[string]string) string {
	dir, err := os.MkdirTemp("", "dateilager_tests_")
	require.NoError(t, err, "create temp dir")

	for name, content := range files {
		writeFile(t, dir, name, content)
	}

	err = client.WriteVersionFile(dir, version)
	require.NoError(t, err, "write version file")

	_, err = client.DiffAndSummarize(context.Background(), dir)
	require.NoError(t, err, "diff and summarize")

	return dir
}

func verifyDir(t *testing.T, dir string, version int64, files map[string]expectedFile) {
	dirEntries := make(map[string]fs.FileInfo)

	// Only keep track of empty walked directories
	var maybeEmptyDir *string
	var maybeEmptyInfo *fs.FileInfo

	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == dir {
			return nil
		}

		if strings.HasPrefix(path, filepath.Join(dir, ".dl")) {
			return nil
		}

		if maybeEmptyDir != nil {
			if !strings.HasPrefix(path, *maybeEmptyDir) {
				dirEntries[fmt.Sprintf("%s/", *maybeEmptyDir)] = *maybeEmptyInfo
			}
			maybeEmptyDir = nil
			maybeEmptyInfo = nil
		}

		if info.IsDir() {
			maybeEmptyDir = &path
			maybeEmptyInfo = &info
			return nil
		}

		dirEntries[path] = info
		return nil
	})
	require.NoError(t, err, "walk directory %v", dir)

	if maybeEmptyDir != nil {
		dirEntries[fmt.Sprintf("%s/", *maybeEmptyDir)] = *maybeEmptyInfo
	}

	fileVersion, err := client.ReadVersionFile(dir)
	require.NoError(t, err, "read version file")

	assert.Equal(t, version, fileVersion, "expected file version %v", version)
	assert.Equal(t, len(files), len(dirEntries), "expected %v files in %v", len(files), dir)

	for name, file := range files {
		path := filepath.Join(dir, name)
		if strings.HasSuffix(name, "/") {
			// filepath.Join removes trailing slashes
			path = fmt.Sprintf("%s/", path)
		}
		info := dirEntries[path]
		require.True(t, info != nil, "can't verify, no file found at expected path %v", path)

		switch file.fileType {
		case typeDirectory:
			assert.True(t, info.IsDir(), "%v is not a directory", name)

		case typeSymlink:
			assert.Equal(t, fs.ModeSymlink, info.Mode()&fs.ModeSymlink, "%v is not a symlink", name)

			target, err := os.Readlink(path)
			require.NoError(t, err, "read link %v", path)

			assert.Equal(t, file.content, target, "symlink target mismatch in %v", name)

		case typeRegular:
			bytes, err := os.ReadFile(path)
			require.NoError(t, err, "read file %v", path)

			assert.Equal(t, file.content, string(bytes), "content mismatch in %v", name)
		}
	}
}

func createTestClient(tc util.TestCtx) (*client.Client, *api.Fs, func()) {
	fs := tc.FsApi()
	reqAuth := tc.Context().Value(auth.AuthCtxKey).(auth.Auth)

	lis := bufconn.Listen(bufSize)
	s := grpc.NewServer(
		grpc.UnaryInterceptor(
			grpc.UnaryServerInterceptor(func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
				return handler(context.WithValue(ctx, auth.AuthCtxKey, reqAuth), req)
			}),
		),
		grpc.StreamInterceptor(
			grpc.StreamServerInterceptor(func(srv interface{}, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
				wrapped := grpc_middleware.WrapServerStream(stream)
				wrapped.WrappedContext = context.WithValue(stream.Context(), auth.AuthCtxKey, reqAuth)
				return handler(srv, wrapped)
			}),
		),
	)

	pb.RegisterFsServer(s, fs)
	go func() {
		err := s.Serve(lis)
		require.NoError(tc.T(), err, "Server exited")
	}()

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	conn, err := grpc.DialContext(tc.Context(), "bufnet", grpc.WithContextDialer(dialer), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(tc.T(), err, "Failed to dial bufnet")

	c := client.NewClientConn(conn)

	return c, fs, func() { c.Close(); s.Stop() }
}

func rebuild(tc util.TestCtx, c *client.Client, project int64, toVersion *int64, dir string, cacheDir *string, expected expectedResponse) {
	if cacheDir == nil {
		newCacheDir := emptyTmpDir(tc.T())
		defer os.RemoveAll(newCacheDir)
		cacheDir = &newCacheDir
	}

	result, err := c.Rebuild(tc.Context(), project, "", toVersion, dir, nil, *cacheDir, nil, true)
	require.NoError(tc.T(), err, "client.Rebuild")

	assert.Equal(tc.T(), expected.version, result.Version, "mismatch rebuild version")
	assert.Equal(tc.T(), expected.count, result.Count, "mismatch rebuild count")
}

func rebuildWithPattern(tc util.TestCtx, c *client.Client, project int64, toVersion *int64, dir string, pattern *files.FilePattern, expectedMatch bool, expected expectedResponse) {
	newCacheDir := emptyTmpDir(tc.T())
	defer os.RemoveAll(newCacheDir)

	result, err := c.Rebuild(tc.Context(), project, "", toVersion, dir, nil, newCacheDir, pattern, true)
	require.NoError(tc.T(), err, "client.Rebuild")

	assert.Equal(tc.T(), expected.version, result.Version, "mismatch rebuild version")
	assert.Equal(tc.T(), expected.count, result.Count, "mismatch rebuild count")
	assert.Equal(tc.T(), expectedMatch, result.PatternMatch, "unexpected file pattern match")
}

func update(tc util.TestCtx, c *client.Client, project int64, dir string, expected expectedResponse) {
	version, count, err := c.Update(tc.Context(), project, dir)
	require.NoError(tc.T(), err, "client.Update")

	assert.Equal(tc.T(), expected.version, version, "mismatch update version")
	assert.Equal(tc.T(), expected.count, count, "mismatch update count")
}

type mockGetServer struct {
	grpc.ServerStream
	ctx     context.Context
	results []*pb.Object
}

func (m *mockGetServer) Context() context.Context {
	return m.ctx
}

func (m *mockGetServer) Send(resp *pb.GetResponse) error {
	m.results = append(m.results, resp.Object)
	return nil
}

type mockGetCompressServer struct {
	grpc.ServerStream
	ctx     context.Context
	results [][]byte
}

func (m *mockGetCompressServer) Context() context.Context {
	return m.ctx
}

func (m *mockGetCompressServer) Send(resp *pb.GetCompressResponse) error {
	m.results = append(m.results, resp.Bytes)
	return nil
}

type mockUpdateServer struct {
	grpc.ServerStream
	ctx      context.Context
	project  int64
	updates  []*pb.Object
	idx      int
	response *pb.UpdateResponse
}

func newMockUpdateServer(ctx context.Context, project int64, updates map[string]expectedObject) *mockUpdateServer {
	var objects []*pb.Object

	for path, object := range updates {
		mode := object.mode
		if mode == 0 {
			mode = 0755
		}

		objects = append(objects, &pb.Object{
			Path:    path,
			Mode:    mode,
			Size:    int64(len(object.content)),
			Deleted: object.deleted,
			Content: []byte(object.content),
		})
	}

	return &mockUpdateServer{
		ctx:     ctx,
		project: project,
		updates: objects,
		idx:     0,
	}
}

func (m *mockUpdateServer) Context() context.Context {
	return m.ctx
}

func (m *mockUpdateServer) SendAndClose(res *pb.UpdateResponse) error {
	m.response = res
	return nil
}

func (m *mockUpdateServer) Recv() (*pb.UpdateRequest, error) {
	if m.idx >= len(m.updates) {
		return nil, io.EOF
	}

	object := m.updates[m.idx]
	m.idx += 1
	return &pb.UpdateRequest{
		Project: m.project,
		Object:  object,
	}, nil
}

type mockGetCacheServer struct {
	grpc.ServerStream
	ctx     context.Context
	results [][]byte
}

func (m *mockGetCacheServer) Context() context.Context {
	return m.ctx
}

func (m *mockGetCacheServer) Send(resp *pb.GetCacheResponse) error {
	m.results = append(m.results, resp.Bytes)
	return nil
}

func buildRequest(project int64, fromVersion, toVersion *int64, prefix bool, paths ...string) *pb.GetRequest {
	path, ignores := paths[0], paths[1:]

	query := &pb.ObjectQuery{
		Path:     path,
		IsPrefix: prefix,
		Ignores:  ignores,
	}

	return &pb.GetRequest{
		Project:     project,
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		Queries:     []*pb.ObjectQuery{query},
	}
}

func exactQuery(project int64, version *int64, paths ...string) *pb.GetRequest {
	return buildRequest(project, nil, version, false, paths...)
}

func prefixQuery(project int64, version *int64, paths ...string) *pb.GetRequest {
	return buildRequest(project, nil, version, true, paths...)
}

func rangeQuery(project int64, fromVersion, toVersion *int64, paths ...string) *pb.GetRequest {
	return buildRequest(project, fromVersion, toVersion, true, paths...)
}

func buildCompressRequest(project int64, fromVersion, toVersion *int64, paths ...string) *pb.GetCompressRequest {
	path, ignores := paths[0], paths[1:]

	query := &pb.ObjectQuery{
		Path:     path,
		IsPrefix: true,
		Ignores:  ignores,
	}

	return &pb.GetCompressRequest{
		Project:     project,
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		Queries:     []*pb.ObjectQuery{query},
	}
}

func verifyStreamResults(t *testing.T, results []*pb.Object, expected map[string]expectedObject) {
	assert.Equal(t, len(expected), len(results), "expected %v objects", len(expected))

	for _, result := range results {
		object, ok := expected[result.Path]
		assert.True(t, ok, "did not expect %v in stream results", result.Path)
		assert.Equal(t, object.content, string(result.Content), "mismatch content for %v", result.Path)
		assert.Equal(t, object.deleted, result.Deleted, "mismatch deleted flag for %v", result.Path)
		if object.mode != 0 {
			assert.Equal(t, object.mode, result.Mode, "mismatch mode for %v", result.Path)
		}
	}
}

func verifyTarResults(t *testing.T, results [][]byte, expected map[string]expectedObject) {
	count := 0

	for _, result := range results {
		s2Reader := s2.NewReader(bytes.NewBuffer(result))
		tarReader := tar.NewReader(s2Reader)

		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			}
			require.NoError(t, err, "failed to read next TAR file")

			count += 1

			expectedMatch, ok := expected[header.Name]
			assert.True(t, ok, "missing %v in expected objects", header.Name)
			if !ok {
				continue
			}

			var buffer bytes.Buffer
			_, err = io.Copy(&buffer, tarReader)
			require.NoError(t, err, "failed to copy content bytes from TAR")

			assert.Equal(t, []byte(expectedMatch.content), buffer.Bytes(), "mismatch content for %v", header.Name)
			if expectedMatch.mode != 0 {
				assert.Equal(t, fs.FileMode(expectedMatch.mode).Perm(), header.Mode, "mismatch file mode for %v", header.Name)
			}
		}
	}

	assert.Equal(t, len(expected), count, "expected %d objects in tar results, got %d", len(expected), count)
}

// Use debugProjects(tc) and debugObjects(tc) within a failing test to log the state of the DB

//lint:ignore U1000 debug utility
func debugProjects(tc util.TestCtx) {
	conn := tc.Connect()
	rows, err := conn.Query(tc.Context(), `
		SELECT id, latest_version, pack_patterns
		FROM dl.projects
	`)
	require.NoError(tc.T(), err, "debug execute project list")

	fmt.Println("\n[DEBUG] Projects")
	fmt.Println("id,\tlatest_version,\tpack_patterns")

	for rows.Next() {
		var id, latestVersion int64
		var packPatterns []string
		err = rows.Scan(&id, &latestVersion, &packPatterns)
		require.NoError(tc.T(), err, "debug scan project")

		fmt.Printf("%d,\t%d,\t\t%v\n", id, latestVersion, packPatterns)
	}

	fmt.Println()
}

//lint:ignore U1000 debug utility
func debugObjects(tc util.TestCtx) {
	conn := tc.Connect()
	rows, err := conn.Query(tc.Context(), `
		SELECT project, start_version, stop_version, path, mode, size, packed, (hash).h1, (hash).h2
		FROM dl.objects
		ORDER BY project, start_version, stop_version, path
	`)
	require.NoError(tc.T(), err, "debug execute object list")

	fmt.Println("\n[DEBUG] Objects")
	fmt.Println("project,\tstart_version,\tstop_version,\tpath,\tmode,\t\tsize,\tpacked,\thash")

	for rows.Next() {
		var project, start_version, mode, size int64
		var stop_version *int64
		var path string
		var packed bool
		var h1, h2 []byte
		err = rows.Scan(&project, &start_version, &stop_version, &path, &mode, &size, &packed, &h1, &h2)
		require.NoError(tc.T(), err, "debug scan object")

		fmt.Printf("%d,\t\t%d,\t\t%s,\t\t%s,\t%s,\t%d,\t%v,\t(%x, %x)\n", project, start_version, formatPtr(stop_version), path, formatMode(mode), size, packed, h1, h2)
	}

	fmt.Println()
}

func formatMode(mode int64) string {
	return fmt.Sprintf("%v", fs.FileMode(mode)&os.ModePerm)
}

func formatPtr(p *int64) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprint(*p)
}
