package test

import (
	"context"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/angelini/dateilager/internal/pb"
	util "github.com/angelini/dateilager/internal/testutil"
	"github.com/angelini/dateilager/pkg/api"
	"github.com/angelini/dateilager/pkg/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

const (
	bufSize = 1024 * 1024
)

var (
	emptyVersionRange = client.VersionRange{From: nil, To: nil}
)

func toVersion(to int64) client.VersionRange {
	return client.VersionRange{From: nil, To: &to}
}

func fromVersion(from int64) client.VersionRange {
	return client.VersionRange{From: &from, To: nil}
}

func createTestClient(tc util.TestCtx, fs *api.Fs) (*client.Client, api.CloseFunc) {
	lis := bufconn.Listen(bufSize)
	s := grpc.NewServer()

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

	c := client.NewClientConn(tc.Context(), tc.Logger(), conn)

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

		if string(object.Contents) != content {
			tc.Errorf("content mismatch for %v expected '%v', got '%v'", object.Path, content, string(object.Contents))
		}
	}
}

func writeTmpFiles(tc util.TestCtx, files map[string]string) string {
	dir, err := ioutil.TempDir("", "dateilager_tests_")
	if err != nil {
		tc.Fatalf("create temp dir: %v", err)
	}

	for name, content := range files {
		err = ioutil.WriteFile(filepath.Join(dir, name), []byte(content), 0666)
		if err != nil {
			tc.Fatalf("write temp file: %v", err)
		}
	}

	return dir
}

func verifyDir(tc util.TestCtx, dir string, files map[string]string) {
	dirFileInfos, err := ioutil.ReadDir(dir)
	if err != nil {
		tc.Fatalf("read directory %v: %v", dir, err)
	}

	if len(dirFileInfos) != len(files) {
		tc.Errorf("expected %v files in %v, got: %v", len(files), dir, len(dirFileInfos))
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		bytes, err := ioutil.ReadFile(path)
		if err != nil {
			tc.Fatalf("read file %v: %v", path, err)
		}

		if string(bytes) != content {
			tc.Errorf("content mismatch in %v expected: '%v', got: '%v'", name, content, string(bytes))
		}
	}
}

func TestGetLatestEmpty(t *testing.T) {
	tc := util.NewTestCtx(t)
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
	tc := util.NewTestCtx(t)
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
	tc := util.NewTestCtx(t)
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
	tc := util.NewTestCtx(t)
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

	verifyDir(tc, tmpDir, map[string]string{
		"/a": "a v1",
		"/b": "b v1",
		"/c": "c v1",
	})
}

func TestRebuildWithOverwritesAndDeletes(t *testing.T) {
	tc := util.NewTestCtx(t)
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

	verifyDir(tc, tmpDir, map[string]string{
		"/a": "a v2",
		"/c": "c v1",
		"/d": "d v2",
	})
}

func TestUpdateObjects(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a", "a v1")
	writeObject(tc, 1, 1, nil, "/b", "b v1")
	writeObject(tc, 1, 1, nil, "/c", "c v1")

	tmpDir := writeTmpFiles(tc, map[string]string{
		"/a": "a v2",
		"/c": "c v2",
	})
	defer os.RemoveAll(tmpDir)

	c, close := createTestClient(tc, tc.FsApi())
	defer close()

	version, err := c.Update(tc.Context(), 1, []string{"/a", "/c"}, tmpDir)
	if err != nil {
		t.Fatalf("client.UpdateObjects: %v", err)
	}

	if version != 2 {
		t.Fatalf("expected version to increment to 2, got: %v", version)
	}

	objects, err := c.Get(tc.Context(), 1, "", emptyVersionRange)
	if err != nil {
		t.Fatalf("client.GetLatest after update: %v", err)
	}

	objectA := objects[0]
	if objectA.Path != "/a" || string(objectA.Contents) != "a v2" {
		t.Errorf("expected object (/a, 'a v2'), got: %v", objectA)
	}

	objectB := objects[1]
	if objectB.Path != "/b" || string(objectB.Contents) != "b v1" {
		t.Errorf("expected object (/b, 'b v1'), got: %v", objectB)
	}

	objectC := objects[2]
	if objectC.Path != "/c" || string(objectC.Contents) != "c v2" {
		t.Errorf("expected object (/c, 'c v2'), got: %v", objectC)
	}
}
