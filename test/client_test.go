package test

import (
	"context"
	"crypto/rand"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/pb"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/gadget-inc/dateilager/pkg/api"
	"github.com/gadget-inc/dateilager/pkg/client"
	fsdiff_pb "github.com/gadget-inc/fsdiff/pkg/pb"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

type Type int

const (
	bufSize = 1024 * 1024
)

const (
	typeRegular Type = iota
	typeDirectory
	typeSymlink
)

var (
	emptyVersionRange = client.VersionRange{From: nil, To: nil}
)

type expectedFile struct {
	content  string
	fileType Type
}

func toVersion(to int64) client.VersionRange {
	return client.VersionRange{From: nil, To: &to}
}

func fromVersion(from int64) client.VersionRange {
	return client.VersionRange{From: &from, To: nil}
}

func createTestClient(tc util.TestCtx, fs *api.Fs) (*client.Client, db.CloseFunc) {
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
		if err := s.Serve(lis); err != nil {
			tc.Fatalf("Server exited with error: %v", err)
		}
	}()

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	conn, err := grpc.DialContext(tc.Context(), "bufnet", grpc.WithContextDialer(dialer), grpc.WithInsecure())
	if err != nil {
		tc.Fatalf("Failed to dial bufnet: %v", err)
	}

	c := client.NewClientConn(conn)

	return c, func(context.Context) { c.Close(); s.Stop() }
}

// asserts that the given objects contain all the expected paths and file contents
func assertObjects(t *testing.T, objects []*pb.Object, expected map[string]string) {
	contents := make(map[string]string)

	for _, object := range objects {
		contents[object.Path] = string(object.Content)
	}

	// This gives in a much nicer diff in the error message over asserting each object separately.
	assert.EqualValues(t, expected, contents, "unexpected contents for objects")
}

func writeTmpFiles(tc util.TestCtx, files map[string]string) string {
	dir, err := os.MkdirTemp("", "dateilager_tests_")
	if err != nil {
		tc.Fatalf("create temp dir: %v", err)
	}

	for name, content := range files {
		err = os.WriteFile(filepath.Join(dir, name), []byte(content), 0755)
		if err != nil {
			tc.Fatalf("write temp file: %v", err)
		}
	}

	return dir
}

func buildDiff(tc util.TestCtx, updates map[string]fsdiff_pb.Update_Action) *fsdiff_pb.Diff {
	diff := &fsdiff_pb.Diff{}
	for path, action := range updates {
		diff.Updates = append(diff.Updates, &fsdiff_pb.Update{
			Path:   path,
			Action: action,
		})
	}

	return diff
}

func verifyDir(tc util.TestCtx, dir string, files map[string]expectedFile) {
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
	if maybeEmptyDir != nil {
		dirEntries[fmt.Sprintf("%s/", *maybeEmptyDir)] = *maybeEmptyInfo
	}

	if err != nil {
		tc.Fatalf("walk directory %v: %v", dir, err)
	}

	if len(dirEntries) != len(files) {
		tc.Errorf("expected %v files in %v, got: %v", len(files), dir, len(dirEntries))
	}

	for name, file := range files {
		// Not using filepath.Join here as it removes the directory trailing slash
		path := fmt.Sprintf("%s%s", dir, name)
		info := dirEntries[path]

		switch file.fileType {
		case typeDirectory:
			if !info.IsDir() {
				tc.Errorf("%v is not a directory", name)
			}

		case typeSymlink:
			if info.Mode()&fs.ModeSymlink != fs.ModeSymlink {
				tc.Errorf("%v is not a symlink", name)
			}

			target, err := os.Readlink(path)
			if err != nil {
				tc.Fatalf("read link %v: %v", path, err)
			}

			if target != file.content {
				tc.Errorf("symlink target mismatch in %v expected: '%v', got: '%v'", name, file.content, target)
			}

		case typeRegular:
			bytes, err := os.ReadFile(path)
			if err != nil {
				tc.Fatalf("read file %v: %v", path, err)
			}

			if string(bytes) != file.content {
				tc.Errorf("content mismatch in %v expected: '%v', got: '%v'", name, file.content, string(bytes))
			}
		}
	}
}

func TestGetLatestEmpty(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)

	c, close := createTestClient(tc, tc.FsApi())
	defer close(tc.Context())

	objects, err := c.Get(tc.Context(), 1, "", nil, emptyVersionRange)
	if err != nil {
		t.Fatalf("client.GetLatest empty: %v", err)
	}

	if len(objects) != 0 {
		t.Fatalf("object list should be empty: %v", objects)
	}
}

func TestGet(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 3)
	writeObject(tc, 1, 1, i(2), "/a", "a v1")
	writeObject(tc, 1, 1, nil, "/b", "b v1")
	writeObject(tc, 1, 2, nil, "/c", "c v2")
	writeObject(tc, 1, 3, nil, "/d", "d v3")

	c, close := createTestClient(tc, tc.FsApi())
	defer close(tc.Context())

	testCases := []struct {
		name     string
		project  int64
		prefix   string
		ignores  []string
		vrange   client.VersionRange
		expected map[string]string
	}{
		{
			name:    "get version 1",
			project: 1,
			vrange:  toVersion(1),
			expected: map[string]string{
				"/a": "a v1",
				"/b": "b v1",
			},
		},
		{
			name:    "get version 2",
			project: 1,
			vrange:  toVersion(2),
			expected: map[string]string{
				"/b": "b v1",
				"/c": "c v2",
			},
		},
		{
			name:    "get version with prefix",
			project: 1,
			prefix:  "/b",
			vrange:  toVersion(2),
			expected: map[string]string{
				"/b": "b v1",
			},
		},
		{
			name:    "get latest version",
			project: 1,
			vrange:  emptyVersionRange,
			expected: map[string]string{
				"/b": "b v1",
				"/c": "c v2",
				"/d": "d v3",
			},
		},
		{
			name:    "get latest version with prefix",
			project: 1,
			prefix:  "/c",
			vrange:  emptyVersionRange,
			expected: map[string]string{
				"/c": "c v2",
			},
		},
		{
			name:    "get latest version with ignores",
			project: 1,
			prefix:  "",
			ignores: []string{"/b"},
			vrange:  emptyVersionRange,
			expected: map[string]string{
				"/c": "c v2",
				"/d": "d v3",
			},
		},
		{
			name:    "get latest version with ignores and deleted files",
			project: 1,
			prefix:  "",
			ignores: []string{"/a"},
			vrange:  fromVersion(1), // makes sure the query includes deleted files
			expected: map[string]string{
				"/c": "c v2",
				"/d": "d v3",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			objects, err := c.Get(tc.Context(), testCase.project, testCase.prefix, testCase.ignores, testCase.vrange)
			require.NoError(t, err, "client.Get")

			assertObjects(t, objects, testCase.expected)
		})
	}
}

func TestGetVersionMissingProject(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	c, close := createTestClient(tc, tc.FsApi())
	defer close(tc.Context())

	objects, err := c.Get(tc.Context(), 1, "", nil, toVersion(1))
	if err == nil {
		t.Fatalf("client.GetLatest didn't error accessing objects: %v", objects)
	}
}

func TestRebuild(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a", "a v1")
	writeObject(tc, 1, 1, nil, "/b", "b v1")
	writeObject(tc, 1, 1, nil, "/c", "c v1")

	c, close := createTestClient(tc, tc.FsApi())
	defer close(tc.Context())

	tmpDir := writeTmpFiles(tc, map[string]string{})
	defer os.RemoveAll(tmpDir)

	version, count, err := c.Rebuild(tc.Context(), 1, "", emptyVersionRange, tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
	}
	if version != 1 {
		t.Errorf("expected rebuild version to be 1, got: %v", version)
	}
	if count != 3 {
		t.Errorf("expected rebuild count to be 3, got: %v", version)
	}

	verifyDir(tc, tmpDir, map[string]expectedFile{
		"/a": {content: "a v1"},
		"/b": {content: "b v1"},
		"/c": {content: "c v1"},
	})
}

func TestRebuildWithOverwritesAndDeletes(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, i(2), "/a", "a v1 - long buffer of content")
	writeObject(tc, 1, 1, i(2), "/b", "b v1")
	writeObject(tc, 1, 1, nil, "/c", "c v1")
	writeObject(tc, 1, 1, i(2), "/e", "e v1")
	writeObject(tc, 1, 2, nil, "/a", "a v2")
	writeObject(tc, 1, 2, nil, "/d", "d v2")
	writeSymlink(tc, 1, 2, nil, "/e", "a")

	c, close := createTestClient(tc, tc.FsApi())
	defer close(tc.Context())

	tmpDir := writeTmpFiles(tc, map[string]string{
		"/a": "a v1 - long buffer of content",
		"/b": "b v1",
		"/c": "c v1",
		"/e": "e v1",
	})
	defer os.RemoveAll(tmpDir)

	version, count, err := c.Rebuild(tc.Context(), 1, "", fromVersion(1), tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild with overwrites and deletes: %v", err)
	}
	if version != 2 {
		t.Errorf("expected rebuild version to be 2, got: %v", version)
	}
	if count != 4 {
		t.Errorf("expected rebuild count to be 4, got: %v", count)
	}

	verifyDir(tc, tmpDir, map[string]expectedFile{
		"/a": {content: "a v2"},
		"/c": {content: "c v1"},
		"/d": {content: "d v2"},
		"/e": {content: "a v2"},
	})
}

func TestRebuildWithEmptyDirAndSymlink(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, nil, "/a", "a v1")
	writeObject(tc, 1, 1, nil, "/d/e", "e v1")
	writeEmptyDir(tc, 1, 1, nil, "/b/")
	writeSymlink(tc, 1, 2, nil, "/c", "/a")
	writeSymlink(tc, 1, 2, nil, "/f/g/h", "/d/e")

	c, close := createTestClient(tc, tc.FsApi())
	defer close(tc.Context())

	tmpDir := writeTmpFiles(tc, map[string]string{})
	defer os.RemoveAll(tmpDir)

	version, count, err := c.Rebuild(tc.Context(), 1, "", emptyVersionRange, tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
	}
	if version != 2 {
		t.Errorf("expected rebuild version to be 2, got: %v", version)
	}
	if count != 5 {
		t.Errorf("expected rebuild count to be 5, got: %v", count)
	}

	verifyDir(tc, tmpDir, map[string]expectedFile{
		"/a":     {content: "a v1"},
		"/d/e":   {content: "e v1"},
		"/b/":    {content: "", fileType: typeDirectory},
		"/c":     {content: "/a", fileType: typeSymlink},
		"/f/g/h": {content: "/d/e", fileType: typeSymlink},
	})
}

func TestRebuildWithUpdatedEmptyDirectories(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeEmptyDir(tc, 1, 1, nil, "/a/")
	writeEmptyDir(tc, 1, 1, nil, "/b/")

	c, close := createTestClient(tc, tc.FsApi())
	defer close(tc.Context())

	tmpDir := writeTmpFiles(tc, map[string]string{})
	defer os.RemoveAll(tmpDir)

	version, count, err := c.Rebuild(tc.Context(), 1, "", emptyVersionRange, tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
	}
	if version != 1 {
		t.Errorf("expected rebuild version to be 1, got: %v", version)
	}
	if count != 2 {
		t.Errorf("expected rebuild count to be 2, got: %v", count)
	}

	verifyDir(tc, tmpDir, map[string]expectedFile{
		"/a/": {content: "", fileType: typeDirectory},
		"/b/": {content: "", fileType: typeDirectory},
	})

	err = os.WriteFile(filepath.Join(tmpDir, "/a/c"), []byte("a/c v2"), 0755)
	if err != nil {
		tc.Fatalf("write file %v: %v", filepath.Join(tmpDir, "/a/c"), err)
	}

	diff := buildDiff(tc, map[string]fsdiff_pb.Update_Action{
		"/a/c": fsdiff_pb.Update_ADD,
	})

	version, count, err = c.Update(tc.Context(), 1, diff, tmpDir)
	if err != nil {
		t.Fatalf("client.UpdateObjects: %v", err)
	}
	if version != 2 {
		t.Errorf("expected update version to be 2, got: %v", version)
	}
	if count != 1 {
		t.Errorf("expected update count to be 1, got: %v", count)
	}

	version, count, err = c.Rebuild(tc.Context(), 1, "", fromVersion(1), tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
	}
	if version != 2 {
		t.Errorf("expected rebuild version to be 2, got: %v", version)
	}
	if count != 1 {
		t.Errorf("expected rebuild count to be 1, got: %v", count)
	}

	verifyDir(tc, tmpDir, map[string]expectedFile{
		"/a/c": {content: "a/c v2"},
		"/b/":  {content: "", fileType: typeDirectory},
	})
}

func TestRebuildWithManyObjects(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	expectedFiles := make(map[string]expectedFile)

	writeProject(tc, 1, 1)
	for i := 0; i < 500; i++ {
		bytes := make([]byte, 50000)
		_, err := rand.Read(bytes)
		if err != nil {
			t.Fatal("could not generate random bytes")
		}
		writeObject(tc, 1, 1, nil, fmt.Sprintf("/%d", i), string(bytes))
		expectedFiles[fmt.Sprintf("/%d", i)] = expectedFile{content: string(bytes)}
	}

	c, close := createTestClient(tc, tc.FsApi())
	defer close(tc.Context())

	tmpDir := writeTmpFiles(tc, map[string]string{})
	defer os.RemoveAll(tmpDir)

	version, count, err := c.Rebuild(tc.Context(), 1, "", emptyVersionRange, tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
	}
	if version != 1 {
		t.Errorf("expected rebuild version to be 1, got: %v", version)
	}
	if count != 500 {
		t.Errorf("expected rebuild count to be 500, got: %v", count)
	}

	verifyDir(tc, tmpDir, expectedFiles)
}

func TestUpdateObjects(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a", "a v1")
	writeObject(tc, 1, 1, nil, "/b", "b v1")
	writeObject(tc, 1, 1, nil, "/c", "c v1")

	tmpDir := writeTmpFiles(tc, map[string]string{
		"/a": "a v2",
		"/c": "c v2",
		"/d": "d v2",
	})
	defer os.RemoveAll(tmpDir)

	diff := buildDiff(tc, map[string]fsdiff_pb.Update_Action{
		"/a": fsdiff_pb.Update_CHANGE,
		"/c": fsdiff_pb.Update_CHANGE,
		"/d": fsdiff_pb.Update_ADD,
	})

	c, close := createTestClient(tc, tc.FsApi())
	defer close(tc.Context())

	version, count, err := c.Update(tc.Context(), 1, diff, tmpDir)
	if err != nil {
		t.Fatalf("client.UpdateObjects: %v", err)
	}
	if version != 2 {
		t.Errorf("expected update version to be 2, got: %v", version)
	}
	if count != 3 {
		t.Errorf("expected update count to be 3, got: %v", count)
	}

	objects, err := c.Get(tc.Context(), 1, "", nil, emptyVersionRange)
	if err != nil {
		t.Fatalf("client.GetLatest after update: %v", err)
	}

	assertObjects(t, objects, map[string]string{
		"/a": "a v2",
		"/b": "b v1",
		"/c": "c v2",
		"/d": "d v2",
	})
}

func TestUpdateWithManyObjects(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)

	fixtureFiles := make(map[string]string)
	updates := make(map[string]fsdiff_pb.Update_Action)

	for i := 0; i < 500; i++ {
		bytes := make([]byte, 50000)
		_, err := rand.Read(bytes)
		if err != nil {
			t.Fatal("could not generate random bytes")
		}

		path := fmt.Sprintf("/%d", i)
		fixtureFiles[path] = string(bytes)
		updates[path] = fsdiff_pb.Update_ADD
	}

	tmpDir := writeTmpFiles(tc, fixtureFiles)
	defer os.RemoveAll(tmpDir)

	diff := buildDiff(tc, updates)

	c, close := createTestClient(tc, tc.FsApi())
	defer close(tc.Context())

	version, count, err := c.Update(tc.Context(), 1, diff, tmpDir)
	if err != nil {
		t.Fatalf("client.UpdateObjects: %v", err)
	}
	if version != 2 {
		t.Errorf("expected update version to be 2, got: %v", version)
	}
	if count != 500 {
		t.Errorf("expected update count to be 500, got: %v", count)
	}

	objects, err := c.Get(tc.Context(), 1, "", nil, emptyVersionRange)
	if err != nil {
		t.Fatalf("client.GetLatest after update: %v", err)
	}

	assertObjects(t, objects, fixtureFiles)
}

func TestUpdateObjectsWithMissingFile(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a", "a v1")
	writeObject(tc, 1, 1, nil, "/b", "b v1")
	writeObject(tc, 1, 1, nil, "/c", "c v1")

	tmpDir := writeTmpFiles(tc, map[string]string{
		"/a": "a v2",
		"/c": "c v2",
		"/d": "d v2",
	})
	defer os.RemoveAll(tmpDir)

	diff := buildDiff(tc, map[string]fsdiff_pb.Update_Action{
		"/a": fsdiff_pb.Update_CHANGE,
		"/c": fsdiff_pb.Update_CHANGE,
		"/d": fsdiff_pb.Update_ADD,
	})

	// Remove "/c" even though it was marked as changed by the diff
	os.Remove(filepath.Join(tmpDir, "c"))
	// Remove "/d" even though it was marked as added by the diff
	os.Remove(filepath.Join(tmpDir, "d"))

	c, close := createTestClient(tc, tc.FsApi())
	defer close(tc.Context())

	version, count, err := c.Update(tc.Context(), 1, diff, tmpDir)
	if err != nil {
		t.Fatalf("client.UpdateObjects: %v", err)
	}
	if version != 2 {
		t.Errorf("expected update version to be 2, got: %v", version)
	}
	if count != 3 {
		t.Errorf("expected update count to be 3, got: %v", count)
	}

	objects, err := c.Get(tc.Context(), 1, "", nil, emptyVersionRange)
	if err != nil {
		t.Fatalf("client.GetLatest after update: %v", err)
	}

	assertObjects(t, objects, map[string]string{
		"/a": "a v2",
		"/b": "b v1",
	})
}

func TestUpdateAndRebuild(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a", "a v1")
	writeObject(tc, 1, 1, nil, "/b", "b v1")
	writeObject(tc, 1, 1, nil, "/c", "c v1")

	c, close := createTestClient(tc, tc.FsApi())
	defer close(tc.Context())

	tmpDir := writeTmpFiles(tc, map[string]string{})
	defer os.RemoveAll(tmpDir)

	version, count, err := c.Rebuild(tc.Context(), 1, "", emptyVersionRange, tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
	}
	if version != 1 {
		t.Errorf("expected rebuild version to be 1, got: %v", version)
	}
	if count != 3 {
		t.Errorf("expected rebuild count to be 3, got: %v", version)
	}

	verifyDir(tc, tmpDir, map[string]expectedFile{
		"/a": {content: "a v1"},
		"/b": {content: "b v1"},
		"/c": {content: "c v1"},
	})

	os.WriteFile(filepath.Join(tmpDir, "a"), []byte("a v2"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "c"), []byte("c v2"), 0755)

	diff := buildDiff(tc, map[string]fsdiff_pb.Update_Action{
		"/a": fsdiff_pb.Update_CHANGE,
		"/c": fsdiff_pb.Update_CHANGE,
	})

	version, count, err = c.Update(tc.Context(), 1, diff, tmpDir)
	if err != nil {
		t.Fatalf("client.UpdateObjects: %v", err)
	}
	if version != 2 {
		t.Errorf("expected update version to be 2, got: %v", version)
	}
	if count != 2 {
		t.Errorf("expected update count to be 2, got: %v", count)
	}

	version, count, err = c.Rebuild(tc.Context(), 1, "", client.VersionRange{From: i(1), To: i(2)}, tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
	}
	if version != 2 {
		t.Errorf("expected rebuild version to be 2, got: %v", version)
	}
	if count != 2 {
		t.Errorf("expected rebuild count to be 2, got: %v", version)
	}

	verifyDir(tc, tmpDir, map[string]expectedFile{
		"/a": {content: "a v2"},
		"/b": {content: "b v1"},
		"/c": {content: "c v2"},
	})
}

func TestUpdateAndRebuildWithIdenticalObjects(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a", "a v1")
	writeObject(tc, 1, 1, nil, "/b", "b v1")
	writeObject(tc, 1, 1, nil, "/c", "c v1")

	c, close := createTestClient(tc, tc.FsApi())
	defer close(tc.Context())

	tmpDir := writeTmpFiles(tc, map[string]string{})
	defer os.RemoveAll(tmpDir)

	version, count, err := c.Rebuild(tc.Context(), 1, "", emptyVersionRange, tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
	}
	if version != 1 {
		t.Errorf("expected rebuild version to be 1, got: %v", version)
	}
	if count != 3 {
		t.Errorf("expected rebuild count to be 3, got: %v", version)
	}

	verifyDir(tc, tmpDir, map[string]expectedFile{
		"/a": {content: "a v1"},
		"/b": {content: "b v1"},
		"/c": {content: "c v1"},
	})

	currentTime := time.Now().Local()
	err = os.Chtimes(filepath.Join(tmpDir, "a"), currentTime, currentTime)
	if err != nil {
		t.Fatalf("touch file %v: %v", filepath.Join(tmpDir, "a"), err)
	}

	err = os.Chtimes(filepath.Join(tmpDir, "b"), currentTime, currentTime)
	if err != nil {
		t.Fatalf("touch file %v: %v", filepath.Join(tmpDir, "b"), err)
	}

	os.WriteFile(filepath.Join(tmpDir, "c"), []byte("c v2"), 0755)

	diff := buildDiff(tc, map[string]fsdiff_pb.Update_Action{
		"/a": fsdiff_pb.Update_CHANGE,
		"/b": fsdiff_pb.Update_CHANGE,
		"/c": fsdiff_pb.Update_CHANGE,
	})

	version, count, err = c.Update(tc.Context(), 1, diff, tmpDir)
	if err != nil {
		t.Fatalf("client.UpdateObjects: %v", err)
	}
	if version != 2 {
		t.Errorf("expected update version to be 2, got: %v", version)
	}
	if count != 3 {
		t.Errorf("expected update count to be 3, got: %v", count)
	}

	version, count, err = c.Rebuild(tc.Context(), 1, "", client.VersionRange{From: i(1), To: i(2)}, tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
	}
	if version != 2 {
		t.Errorf("expected rebuild version to be 2, got: %v", version)
	}

	// Only one file should be updated since /a and /b were identical but with a new mod times
	if count != 1 {
		t.Errorf("expected rebuild count to be 1, got: %v", version)
	}
}

func TestUpdateAndRebuildWithPacked(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writePackedObjects(tc, 1, 1, nil, "/a/", map[string]expectedObject{
		"/a/c": {content: "a/c v1"},
		"/a/d": {content: "a/d v1"},
	})
	writeObject(tc, 1, 1, nil, "/b", "b v1")

	c, close := createTestClient(tc, tc.FsApi())
	defer close(tc.Context())

	tmpDir := writeTmpFiles(tc, map[string]string{})
	defer os.RemoveAll(tmpDir)

	version, count, err := c.Rebuild(tc.Context(), 1, "", emptyVersionRange, tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
	}
	if version != 1 {
		t.Errorf("expected rebuild version to be 1, got: %v", version)
	}
	if count != 3 {
		t.Errorf("expected rebuild count to be 3, got: %v", version)
	}

	verifyDir(tc, tmpDir, map[string]expectedFile{
		"/a/c": {content: "a/c v1"},
		"/a/d": {content: "a/d v1"},
		"/b":   {content: "b v1"},
	})

	os.WriteFile(filepath.Join(tmpDir, "a/c"), []byte("a/c v2"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "b"), []byte("b v2"), 0755)

	diff := buildDiff(tc, map[string]fsdiff_pb.Update_Action{
		"/a/c": fsdiff_pb.Update_CHANGE,
		"/b":   fsdiff_pb.Update_CHANGE,
	})

	version, count, err = c.Update(tc.Context(), 1, diff, tmpDir)
	if err != nil {
		t.Fatalf("client.UpdateObjects: %v", err)
	}
	if version != 2 {
		t.Errorf("expected update version to be 2, got: %v", version)
	}
	if count != 2 {
		t.Errorf("expected update count to be 2, got: %v", count)
	}

	version, count, err = c.Rebuild(tc.Context(), 1, "", client.VersionRange{From: i(1), To: i(2)}, tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
	}
	if version != 2 {
		t.Errorf("expected rebuild version to be 2, got: %v", version)
	}
	if count != 2 {
		t.Errorf("expected rebuild count to be 2, got: %v", version)
	}

	verifyDir(tc, tmpDir, map[string]expectedFile{
		"/a/c": {content: "a/c v2"},
		"/a/d": {content: "a/d v1"},
		"/b":   {content: "b v2"},
	})
}

func TestUpdateAndRebuildWithIdenticalPackedObjects(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1, "/a/")
	writePackedObjects(tc, 1, 1, nil, "/a/", map[string]expectedObject{
		"/a/c": {content: "a/c v1"},
		"/a/d": {content: "a/d v1"},
	})
	writeObject(tc, 1, 1, nil, "/b", "b v1")

	c, close := createTestClient(tc, tc.FsApi())
	defer close(tc.Context())

	tmpDir := writeTmpFiles(tc, map[string]string{})
	defer os.RemoveAll(tmpDir)

	version, count, err := c.Rebuild(tc.Context(), 1, "", emptyVersionRange, tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
	}
	if version != 1 {
		t.Errorf("expected rebuild version to be 1, got: %v", version)
	}
	if count != 3 {
		t.Errorf("expected rebuild count to be 3, got: %v", version)
	}

	verifyDir(tc, tmpDir, map[string]expectedFile{
		"/a/c": {content: "a/c v1"},
		"/a/d": {content: "a/d v1"},
		"/b":   {content: "b v1"},
	})

	currentTime := time.Now().Local()
	err = os.Chtimes(filepath.Join(tmpDir, "a/c"), currentTime, currentTime)
	if err != nil {
		t.Fatalf("touch file %v: %v", filepath.Join(tmpDir, "a/c"), err)
	}

	err = os.Chtimes(filepath.Join(tmpDir, "a/d"), currentTime, currentTime)
	if err != nil {
		t.Fatalf("touch file %v: %v", filepath.Join(tmpDir, "a/d"), err)
	}

	os.WriteFile(filepath.Join(tmpDir, "b"), []byte("b v2"), 0755)

	diff := buildDiff(tc, map[string]fsdiff_pb.Update_Action{
		"/a/c": fsdiff_pb.Update_CHANGE,
		"/a/d": fsdiff_pb.Update_CHANGE,
		"/b":   fsdiff_pb.Update_CHANGE,
	})

	version, count, err = c.Update(tc.Context(), 1, diff, tmpDir)
	if err != nil {
		t.Fatalf("client.UpdateObjects: %v", err)
	}
	if version != 2 {
		t.Errorf("expected update version to be 2, got: %v", version)
	}
	if count != 3 {
		t.Errorf("expected update count to be 3, got: %v", count)
	}

	version, count, err = c.Rebuild(tc.Context(), 1, "", client.VersionRange{From: i(1), To: i(2)}, tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
	}
	if version != 2 {
		t.Errorf("expected rebuild version to be 2, got: %v", version)
	}

	// Only one file should be updated since /a and /b were identical but with a new mod times
	if count != 1 {
		t.Errorf("expected rebuild count to be 1, got: %v", version)
	}
}

func TestDeleteProject(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Admin, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, i(2), "/a", "a v1")
	writeObject(tc, 1, 1, nil, "/b", "b v1")
	writeObject(tc, 1, 2, nil, "/c", "c v2")

	c, close := createTestClient(tc, tc.FsApi())
	defer close(tc.Context())

	objects, err := c.Get(tc.Context(), 1, "", nil, emptyVersionRange)
	if err != nil {
		t.Fatalf("client.GetLatest with results: %v", err)
	}

	assertObjects(t, objects, map[string]string{
		"/b": "b v1",
		"/c": "c v2",
	})

	err = c.DeleteProject(tc.Context(), 1)
	if err != nil {
		t.Fatalf("client.DeleteProject with results: %v", err)
	}

	objects, err = c.Get(tc.Context(), 1, "", nil, toVersion(1))
	if err == nil {
		t.Fatalf("client.GetLatest didn't error accessing objects: %v", objects)
	}
}
