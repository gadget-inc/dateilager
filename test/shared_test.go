package test

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/environment"
	"github.com/gadget-inc/dateilager/internal/exec"
	"github.com/gadget-inc/dateilager/internal/files"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/lvm"
	"github.com/gadget-inc/dateilager/internal/pb"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/gadget-inc/dateilager/pkg/api"
	"github.com/gadget-inc/dateilager/pkg/client"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/klauspost/compress/s2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func init() {
	encoding := os.Getenv("LOG_ENCODING")
	if encoding == "" {
		encoding = "console"
	}

	levelStr := os.Getenv("LOG_LEVEL")
	if levelStr == "" {
		levelStr = "info"
	}

	level, err := zapcore.ParseLevel(levelStr)
	if err != nil {
		panic(fmt.Sprintf("failed to parse log level: %v", err))
	}

	err = logger.Init(environment.Dev, encoding, zap.NewAtomicLevelAt(level))
	if err != nil {
		panic(fmt.Sprintf("failed to init logger: %v", err))
	}
}

type Type int

const (
	bufSize       = 1024 * 1024
	symlinkMode   = 0o755 | int64(fs.ModeSymlink)
	directoryMode = 0o755 | int64(fs.ModeDir)
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
	uid      int
	gid      int
}

type expectedResponse struct {
	version int64
	count   uint32
}

var emptyVersionRange = client.VersionRange{From: nil, To: nil}

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

func countObjectsByProject(tc util.TestCtx, project int64) int {
	conn := tc.Connect()

	var count int
	err := conn.QueryRow(tc.Context(), `
		SELECT count(*)
		FROM dl.objects
		WHERE project = $1
	`, project).Scan(&count)
	require.NoError(tc.T(), err, "count objects")

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

	writeObjectFull(tc, project, start, stop, path, content, 0o755)
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
	mode := fs.FileMode(0o755)
	mode |= fs.ModeDir

	writeObjectFull(tc, project, start, stop, path, "", mode)
}

func writeSymlink(tc util.TestCtx, project int64, start int64, stop *int64, path, target string) {
	mode := fs.FileMode(0o755)
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
	`, project, start, stop, path, hash.H1, hash.H2, 0, len(contentsTar), true)
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

func writePackedFiles(tc util.TestCtx, project int64, start int64, stop *int64, path string, extraFiles ...map[string]expectedObject) string {
	objects := map[string]expectedObject{
		filepath.Join(path, "1"): {content: fmt.Sprintf("%s v%d", filepath.Join(path, "1"), start)},
		filepath.Join(path, "2"): {content: fmt.Sprintf("%s v%d", filepath.Join(path, "2"), start)},
	}

	for _, extraFiles := range extraFiles {
		for k, v := range extraFiles {
			objects[filepath.Join(path, k)] = v
		}
	}

	hash := writePackedObjects(tc, project, start, stop, path, objects)
	return hash.Hex()
}

func packObjects(tc util.TestCtx, objects map[string]expectedObject) []byte {
	contentWriter := db.NewTarWriter()
	defer contentWriter.Close()

	var keys []string
	for key := range objects {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	// iterate the objects for packing in a deterministic order
	for _, key := range keys {
		info := objects[key]
		mode := info.mode
		if mode == 0 {
			mode = 0o755
		}

		object := db.NewUncachedTarObject(key, mode, int64(len(info.content)), info.deleted, []byte(info.content))

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
	err := os.MkdirAll(filepath.Dir(fullPath), 0o755)
	require.NoError(t, err, "mkdir %v", filepath.Dir(fullPath))
	err = os.WriteFile(fullPath, []byte(content), 0o755)
	require.NoError(t, err, "write file %v", path)
}

func mkdirAll(t testing.TB, path string, mode os.FileMode) {
	if err := os.MkdirAll(path, mode); err != nil {
		t.Fatalf("failed to create directory %s: %v", path, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat directory %s: %v", path, err)
	}

	if info.Mode()&os.ModePerm != mode {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatalf("failed to change permissions of directory %s: %v", path, err)
		}
	}
}

var (
	cachedWorkspaceDir     string
	cachedWorkspaceDirOnce sync.Once
)

// workspaceDir returns the root directory of the git repository.
func workspaceDir(t testing.TB) string {
	cachedWorkspaceDirOnce.Do(func() {
		cachedWorkspaceDir = execOutput(t, "git", "rev-parse", "--show-toplevel")
	})
	return cachedWorkspaceDir
}

// emptyTmpDir returns a temporary directory for testing. (e.g. tmp/test/dateilager_test_<random>)
func emptyTmpDir(t testing.TB) string {
	wd := workspaceDir(t)
	mkdirAll(t, path.Join(wd, "tmp/test"), 0o777)
	dir, err := os.MkdirTemp(path.Join(wd, "tmp/test"), "dateilager_test_")
	require.NoError(t, err, "failed to create tmp dir")
	return dir
}

// emptyBenchDir returns a temporary directory for benchmarking. (e.g. tmp/bench/dateilager_bench_<random>)
func emptyBenchDir(b *testing.B) string {
	wd := workspaceDir(b)
	mkdirAll(b, path.Join(wd, "tmp/bench"), 0o755)
	dir, err := os.MkdirTemp(path.Join(wd, "tmp/bench"), "dateilager_bench_")
	require.NoError(b, err, "failed to create tmp dir")
	return dir
}

// cachedStagingDir returns a directory similar to the staging directory used by the cached csi driver.
//
// The directory contains a dl_cache directory with a large node_modules directory inside it.
// The layout is as follows:
//
//	tmp/dateilager_cached/
//	├── dl_cache/
//	│   ├── package.json
//	│   └── package-lock.json
//	│   └── node_modules/
//	│       ├── react/
//	│       ├── react-dom/
//	│       └── ... (many more)
func cachedStagingDir(t testing.TB) string {
	wd := workspaceDir(t)
	stagingDir := path.Join(wd, "tmp/dateilager_cached")
	mkdirAll(t, path.Join(stagingDir, "dl_cache"), 0o755)

	err := os.WriteFile(path.Join(stagingDir, "dl_cache/package.json"), []byte(`{
  "name": "bigdir",
  "version": "1.0.0",
  "description": "A big directory",
  "main": "index.js",
  "scripts": {
    "test": "echo \"Error: no test specified\" && exit 1"
  },
  "dependencies": {
    "@swc/cli": "*",
    "@swc/core": "*",
    "@swc/wasm": "*",
    "@swc/jest": "*",
    "@swc/helpers": "*",
    "@gadgetinc/react": "*",
    "@mdxeditor/editor": "*",
    "@radix-ui/react-accordion": "*",
    "@radix-ui/react-avatar": "*",
    "@radix-ui/react-checkbox": "*",
    "@radix-ui/react-dialog": "*",
    "@radix-ui/react-dropdown-menu": "*",
    "@radix-ui/react-label": "*",
    "@radix-ui/react-popover": "*",
    "@radix-ui/react-progress": "*",
    "@radix-ui/react-radio-group": "*",
    "@radix-ui/react-scroll-area": "*",
    "@radix-ui/react-select": "*",
    "@radix-ui/react-separator": "*",
    "@radix-ui/react-slot": "*",
    "@radix-ui/react-tabs": "*",
    "@radix-ui/react-tooltip": "*",
    "@react-router/dev": "*",
    "@react-router/fs-routes": "*",
    "@react-router/node": "*",
    "@react-router/serve": "*",
    "@types/node": "*",
    "@types/react": "*",
    "@types/react-dom": "*",
    "autoprefixer": "*",
    "class-variance-authority": "*",
    "clsx": "*",
    "cmdk": "*",
    "date-fns": "*",
    "ggt": "*",
    "isbot": "*",
    "lucide-react": "*",
    "postcss": "*",
    "prettier": "*",
    "prettier-plugin-organize-imports": "*",
    "prettier-plugin-packagejson": "*",
    "react": "*",
    "react-day-picker": "*",
    "react-dom": "*",
    "react-router": "*",
    "sonner": "*",
    "tailwind-merge": "*",
    "tailwindcss": "*",
    "tailwindcss-animate": "*",
    "typescript": "*",
    "vite": "*"
  }
}`), 0o644)
	require.NoError(t, err, "failed to write package.json")

	cmd := exec.Command(t.Context(), "npm", "install")
	cmd.SetDir(path.Join(stagingDir, "dl_cache"))
	require.NoError(t, cmd.Run(), "npm install failed")

	return stagingDir
}

func writeTmpFiles(t *testing.T, version int64, files map[string]string) string {
	dir := emptyTmpDir(t)

	for name, content := range files {
		writeFile(t, dir, name, content)
	}

	err := client.WriteVersionFile(dir, version)
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

	if version != -1 {
		fileVersion, err := client.ReadVersionFile(dir)
		require.NoError(t, err, "read version file")

		assert.Equal(t, version, fileVersion, "expected file version %v", version)
		assert.Equal(t, len(files), len(dirEntries), "expected %v files in %v", len(files), dir)
	}

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

		if file.uid != 0 {
			assert.Equal(t, file.uid, int(info.Sys().(*syscall.Stat_t).Uid), "uid mismatch in %v", name)
		}

		if file.gid != 0 {
			assert.Equal(t, file.gid, int(info.Sys().(*syscall.Stat_t).Gid), "gid mismatch in %v", name)
		}
	}
}

func createTestGRPCServer(tc util.TestCtx) (*bufconn.Listener, *grpc.Server, func() *grpc.ClientConn) {
	reqAuth := tc.Auth()
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

	lis := bufconn.Listen(bufSize)
	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	getConn := func() *grpc.ClientConn {
		//nolint:staticcheck, nolintlint // Using DialContext until we're ready to migrate to NewClient
		conn, err := grpc.DialContext(tc.Context(), "bufnet", grpc.WithContextDialer(dialer), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(tc.T(), err, "Failed to dial bufnet")
		return conn
	}

	return lis, s, getConn
}

func createTestClient(tc util.TestCtx) (*client.Client, *api.Fs, func()) {
	lis, s, getConn := createTestGRPCServer(tc)

	fs := tc.FsApi()
	pb.RegisterFsServer(s, fs)

	go func() {
		err := s.Serve(lis)
		require.NoError(tc.T(), err, "Server exited")
	}()

	c := client.NewClientConn(getConn())

	return c, fs, func() { c.Close(); s.Stop() }
}

func rebuild(tc util.TestCtx, c *client.Client, project int64, toVersion *int64, dir string, cacheDir *string, expected expectedResponse, subpaths []string) {
	if cacheDir == nil {
		newCacheDir := emptyTmpDir(tc.T())
		defer os.RemoveAll(newCacheDir)
		cacheDir = &newCacheDir
	}

	result, err := c.Rebuild(tc.Context(), project, "", toVersion, dir, nil, subpaths, *cacheDir, nil, true)
	require.NoError(tc.T(), err, "client.Rebuild")

	assert.Equal(tc.T(), expected.version, result.Version, "mismatch rebuild version")
	assert.Equal(tc.T(), expected.count, result.Count, "mismatch rebuild count")
}

func rebuildWithMatcher(tc util.TestCtx, c *client.Client, project int64, toVersion *int64, dir string, matcher *files.FileMatcher, expectedMatch bool, expected expectedResponse) {
	newCacheDir := emptyTmpDir(tc.T())
	defer os.RemoveAll(newCacheDir)

	result, err := c.Rebuild(tc.Context(), project, "", toVersion, dir, nil, nil, newCacheDir, matcher, true)
	require.NoError(tc.T(), err, "client.Rebuild")

	assert.Equal(tc.T(), expected.version, result.Version, "mismatch rebuild version")
	assert.Equal(tc.T(), expected.count, result.Count, "mismatch rebuild count")
	assert.Equal(tc.T(), expectedMatch, result.FileMatch, "unexpected file match")
}

func update(tc util.TestCtx, c *client.Client, project int64, dir string, expected expectedResponse, subpaths []string) {
	version, count, err := c.Update(tc.Context(), project, dir, subpaths)
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
			mode = 0o755
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

func buildCompressRequest(project int64, fromVersion, toVersion *int64, subpaths []string, paths ...string) *pb.GetCompressRequest {
	path, ignores := paths[0], paths[1:]

	if subpaths == nil {
		subpaths = []string{}
	}

	query := &pb.ObjectQuery{
		Path:     path,
		IsPrefix: true,
		Ignores:  ignores,
		Subpaths: subpaths,
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
	require.NoError(tc.T(), rows.Err(), "iterate rows")

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
	require.NoError(tc.T(), rows.Err(), "iterate rows")

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

// CompareDirectories compares the contents and permissions of two directories recursively.
func CompareDirectories(dir1, dir2 string) error {
	files1 := make(map[string]os.FileInfo)
	err := filepath.Walk(dir1, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(dir1, path)
		if err != nil {
			return err
		}
		files1[relPath] = info
		return nil
	})
	if err != nil {
		return err
	}

	err = filepath.Walk(dir2, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(dir2, path)
		if err != nil {
			return err
		}
		if origInfo, ok := files1[relPath]; ok {
			if info.IsDir() && origInfo.IsDir() {
				// Only compare directories for existence, not content
				delete(files1, relPath)
				return nil
			}
			if !compareFileInfo(origInfo, info) {
				return fmt.Errorf("file permissions differ for %s: %o vs %o", relPath, origInfo.Mode()&os.ModePerm, info.Mode()&os.ModePerm)
			}
			if equal, err := compareFileContents(info, filepath.Join(dir1, relPath), filepath.Join(dir2, relPath)); err != nil {
				return fmt.Errorf("error comparing contents of %s: %v", relPath, err)
			} else if !equal {
				return fmt.Errorf("contents differ for %s", relPath)
			}
			delete(files1, relPath)
		} else {
			return fmt.Errorf("extra file found in directory 2: %s", path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for file := range files1 {
		err = fmt.Errorf("file missing in directory 2: %s", file)
	}
	return err
}

// compareFileInfo compares the permissions and other metadata of two files.
func compareFileInfo(info1, info2 os.FileInfo) bool {
	return (info1.Mode() & os.ModePerm) == (info2.Mode() & os.ModePerm)
}

// compareFileContents compares the contents of two files.
func compareFileContents(info os.FileInfo, file1, file2 string) (bool, error) {
	if info.Mode()&os.ModeSymlink != 0 {
		f1Target, err := os.Readlink(file1)
		if err != nil {
			return false, err
		}
		f2Target, err := os.Readlink(file2)
		if err != nil {
			return false, err
		}
		return f1Target == f2Target, nil
	}

	f1, err := os.Open(file1)
	if err != nil {
		return false, err
	}
	defer f1.Close()
	f2, err := os.Open(file2)
	if err != nil {
		return false, err
	}
	defer f2.Close()

	hash1, hash2 := sha256.New(), sha256.New()
	if _, err := io.Copy(hash1, f1); err != nil {
		return false, err
	}
	if _, err := io.Copy(hash2, f2); err != nil {
		return false, err
	}

	return bytes.Equal(hash1.Sum(nil), hash2.Sum(nil)), nil
}

// exec executes a command
func execRun(t testing.TB, command string, args ...string) {
	err := exec.Run(t.Context(), command, args...)
	require.NoError(t, err)
}

// execOutput executes a command and returns the output
func execOutput(t testing.TB, command string, args ...string) string {
	out, err := exec.Output(t.Context(), command, args...)
	require.NoError(t, err)
	return out
}

func ensurePV(t testing.TB, pv string) {
	err := lvm.EnsurePV(t.Context(), pv)
	require.NoError(t, err)
}

func removePV(t testing.TB, pv string) {
	err := lvm.RemovePV(t.Context(), pv)
	require.NoError(t, err)
}

func ensureVG(t testing.TB, vgName string, devices ...string) {
	err := lvm.EnsureVG(t.Context(), vgName, devices...)
	require.NoError(t, err)
}

func removeVG(t testing.TB, vgName string) {
	err := lvm.RemoveVG(t.Context(), vgName)
	require.NoError(t, err)
}

func ensureLV(t testing.TB, lvName string, lvCreateArgs ...string) {
	err := lvm.EnsureLV(t.Context(), lvName, lvCreateArgs...)
	require.NoError(t, err)
}

func removeLV(t testing.TB, lvName string) {
	err := lvm.RemoveLV(t.Context(), lvName)
	require.NoError(t, err)
}
