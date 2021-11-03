package test

import (
	"context"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/gadget-inc/dateilager/internal/auth"
	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/pb"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/gadget-inc/dateilager/pkg/api"
	"github.com/gadget-inc/dateilager/pkg/client"
	fsdiff "github.com/gadget-inc/fsdiff/pkg/diff"
	fsdiff_pb "github.com/gadget-inc/fsdiff/pkg/pb"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

type Type int

const (
	bufSize          = 1024 * 1024
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

	c := client.NewClientConn(tc.Logger(), conn)

	return c, func() { c.Close(); s.Stop() }
}

func verifyObjects(tc util.TestCtx, objects []*pb.Object, expected map[string]string) {
	if len(expected) != len(objects) {
		tc.Errorf("expected %v objects, got: %v", len(expected), len(objects))
	}

	for _, object := range objects {
		content, ok := expected[object.Path]
		if !ok {
			tc.Fatalf("object path %v not in expected objects", object.Path)
		}

		if string(object.Content) != content {
			tc.Errorf("content mismatch for %v expected '%v', got '%v'", object.Path, content, string(object.Content))
		}
	}
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

func writeDiffFile(tc util.TestCtx, updates map[string]fsdiff_pb.Update_Action) string {
	file, err := os.CreateTemp("", "dateilager_tests_diff_")
	if err != nil {
		tc.Fatalf("create temp file: %v", err)
	}
	fileName := file.Name()

	diff := &fsdiff_pb.Diff{CreatedAt: 0}
	for path, action := range updates {
		diff.Updates = append(diff.Updates, &fsdiff_pb.Update{
			Path:   path,
			Action: action,
		})
	}

	err = fsdiff.WriteDiff(fileName, diff)
	if err != nil {
		tc.Fatalf("write diff file: %v", err)
	}

	return fileName
}

func verifyDir(tc util.TestCtx, dir string, files map[string]expectedFile) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		tc.Fatalf("read directory %v: %v", dir, err)
	}

	if len(dirEntries) != len(files) {
		tc.Errorf("expected %v files in %v, got: %v", len(files), dir, len(dirEntries))
	}

	for name, file := range files {
		path := filepath.Join(dir, name)

		info, err := os.Lstat(path)
		if err != nil {
			tc.Fatalf("lstat file %v: %v", path, err)
		}

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
	defer close()

	objects, err := c.Get(tc.Context(), 1, "", emptyVersionRange)
	if err != nil {
		t.Fatalf("client.GetLatest empty: %v", err)
	}

	if len(objects) != 0 {
		t.Fatalf("object list should be empty: %v", objects)
	}
}

func TestGetLatest(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, i(2), "/a", "a v1")
	writeObject(tc, 1, 1, nil, "/b", "b v1")
	writeObject(tc, 1, 2, nil, "/c", "c v2")

	c, close := createTestClient(tc, tc.FsApi())
	defer close()

	objects, err := c.Get(tc.Context(), 1, "", emptyVersionRange)
	if err != nil {
		t.Fatalf("client.GetLatest with results: %v", err)
	}

	verifyObjects(tc, objects, map[string]string{
		"/b": "b v1",
		"/c": "c v2",
	})

	objects, err = c.Get(tc.Context(), 1, "/c", emptyVersionRange)
	if err != nil {
		t.Fatalf("client.GetLatest with results: %v", err)
	}

	verifyObjects(tc, objects, map[string]string{
		"/c": "c v2",
	})
}

func TestGetVersion(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 3)
	writeObject(tc, 1, 1, i(2), "/a", "a v1")
	writeObject(tc, 1, 1, nil, "/b", "b v1")
	writeObject(tc, 1, 2, nil, "/c", "c v2")
	writeObject(tc, 1, 3, nil, "/d", "d v3")

	c, close := createTestClient(tc, tc.FsApi())
	defer close()

	objects, err := c.Get(tc.Context(), 1, "", toVersion(1))
	if err != nil {
		t.Fatalf("client.GetLatest with results: %v", err)
	}

	verifyObjects(tc, objects, map[string]string{
		"/a": "a v1",
		"/b": "b v1",
	})

	objects, err = c.Get(tc.Context(), 1, "", toVersion(2))
	if err != nil {
		t.Fatalf("client.GetLatest with results: %v", err)
	}

	verifyObjects(tc, objects, map[string]string{
		"/b": "b v1",
		"/c": "c v2",
	})

	objects, err = c.Get(tc.Context(), 1, "/b", toVersion(2))
	if err != nil {
		t.Fatalf("client.GetLatest with results: %v", err)
	}

	verifyObjects(tc, objects, map[string]string{
		"/b": "b v1",
	})
}

func TestRebuild(t *testing.T) {
	tc := util.NewTestCtx(t, auth.Project, 1)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a", "a v1")
	writeObject(tc, 1, 1, nil, "/b", "b v1")
	writeObject(tc, 1, 1, nil, "/c", "c v1")

	c, close := createTestClient(tc, tc.FsApi())
	defer close()

	tmpDir := writeTmpFiles(tc, map[string]string{})
	defer os.RemoveAll(tmpDir)

	err := c.Rebuild(tc.Context(), 1, "", emptyVersionRange, tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
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
	writeObject(tc, 1, 1, i(2), "/a", "a v1")
	writeObject(tc, 1, 1, i(2), "/b", "b v1")
	writeObject(tc, 1, 1, nil, "/c", "c v1")
	writeObject(tc, 1, 2, nil, "/a", "a v2")
	writeObject(tc, 1, 2, nil, "/d", "d v2")

	c, close := createTestClient(tc, tc.FsApi())
	defer close()

	tmpDir := writeTmpFiles(tc, map[string]string{
		"/a": "a v1",
		"/b": "b v1",
		"/c": "c v1",
	})
	defer os.RemoveAll(tmpDir)

	err := c.Rebuild(tc.Context(), 1, "", fromVersion(1), tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild with overwrites and deletes: %v", err)
	}

	verifyDir(tc, tmpDir, map[string]expectedFile{
		"/a": {content: "a v2"},
		"/c": {content: "c v1"},
		"/d": {content: "d v2"},
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
	defer close()

	tmpDir := writeTmpFiles(tc, map[string]string{})
	defer os.RemoveAll(tmpDir)

	err := c.Rebuild(tc.Context(), 1, "", emptyVersionRange, tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
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
	defer close()

	tmpDir := writeTmpFiles(tc, map[string]string{})
	defer os.RemoveAll(tmpDir)

	err := c.Rebuild(tc.Context(), 1, "", emptyVersionRange, tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
	}

	verifyDir(tc, tmpDir, map[string]expectedFile{
		"/a/": {content: "", fileType: typeDirectory},
		"/b/": {content: "", fileType: typeDirectory},
	})

	err = os.WriteFile(filepath.Join(tmpDir, "/a/c"), []byte("a/c v2"), 0755)
	if err != nil {
		tc.Fatalf("write file %v: %v", filepath.Join(tmpDir, "/a/c"), err)
	}

	diffPath := writeDiffFile(tc, map[string]fsdiff_pb.Update_Action{
		"/a/c": fsdiff_pb.Update_ADD,
	})

	version, count, err := c.Update(tc.Context(), 1, diffPath, tmpDir)
	if err != nil {
		t.Fatalf("client.UpdateObjects: %v", err)
	}

	if version != 2 {
		t.Errorf("expected version to increment to 2, got: %v", version)
	}

	if count != 1 {
		t.Errorf("expected count to be 1, got: %v", count)
	}

	err = c.Rebuild(tc.Context(), 1, "", fromVersion(1), tmpDir)
	if err != nil {
		t.Fatalf("client.Rebuild: %v", err)
	}

	verifyDir(tc, tmpDir, map[string]expectedFile{
		"/a/c": {content: "a/c v2"},
		"/b/":  {content: "", fileType: typeDirectory},
	})
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

	diffPath := writeDiffFile(tc, map[string]fsdiff_pb.Update_Action{
		"/a": fsdiff_pb.Update_CHANGE,
		"/c": fsdiff_pb.Update_CHANGE,
		"/d": fsdiff_pb.Update_ADD,
	})
	defer os.Remove(diffPath)

	c, close := createTestClient(tc, tc.FsApi())
	defer close()

	version, count, err := c.Update(tc.Context(), 1, diffPath, tmpDir)
	if err != nil {
		t.Fatalf("client.UpdateObjects: %v", err)
	}

	if version != 2 {
		t.Errorf("expected version to increment to 2, got: %v", version)
	}

	if count != 3 {
		t.Errorf("expected count to be 3, got: %v", count)
	}

	objects, err := c.Get(tc.Context(), 1, "", emptyVersionRange)
	if err != nil {
		t.Fatalf("client.GetLatest after update: %v", err)
	}

	verifyObjects(tc, objects, map[string]string{
		"/a": "a v2",
		"/b": "b v1",
		"/c": "c v2",
		"/d": "d v2",
	})
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

	diffPath := writeDiffFile(tc, map[string]fsdiff_pb.Update_Action{
		"/a": fsdiff_pb.Update_CHANGE,
		"/c": fsdiff_pb.Update_CHANGE,
		"/d": fsdiff_pb.Update_ADD,
	})
	defer os.Remove(diffPath)

	// Remove "/c" even though it was marked as changed by the diff
	os.Remove(filepath.Join(tmpDir, "c"))
	// Remove "/d" even though it was marked as added by the diff
	os.Remove(filepath.Join(tmpDir, "d"))

	c, close := createTestClient(tc, tc.FsApi())
	defer close()

	version, count, err := c.Update(tc.Context(), 1, diffPath, tmpDir)
	if err != nil {
		t.Fatalf("client.UpdateObjects: %v", err)
	}

	if version != 2 {
		t.Errorf("expected version to increment to 2, got: %v", version)
	}

	if count != 3 {
		t.Errorf("expected count to be 3, got: %v", count)
	}

	objects, err := c.Get(tc.Context(), 1, "", emptyVersionRange)
	if err != nil {
		t.Fatalf("client.GetLatest after update: %v", err)
	}

	verifyObjects(tc, objects, map[string]string{
		"/a": "a v2",
		"/b": "b v1",
	})
}
