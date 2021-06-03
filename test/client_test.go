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

const bufSize = 1024 * 1024

func createTestServer(tc util.TestCtx, fs *api.Fs) (*client.Client, api.CloseFunc) {
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

func writeTmpFiles(tc util.TestCtx, files map[string]string) string {
	dir, err := ioutil.TempDir("", "dateilager_tests")
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

func TestGetLatestEmpty(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 1)

	c, close := createTestServer(tc, tc.FsApi())
	defer close()

	objects, err := c.GetLatest(tc.Context(), 1, "")
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

	c, close := createTestServer(tc, tc.FsApi())
	defer close()

	objects, err := c.GetLatest(tc.Context(), 1, "")
	if err != nil {
		t.Fatalf("client.GetLatest with results: %v", err)
	}

	objectB := objects[0]
	if objectB.Path != "/b" || string(objectB.Contents) != "b v1" {
		t.Errorf("expected object (/b, 'b v1'), got: %v", objectB)
	}

	objectC := objects[1]
	if objectC.Path != "/c" || string(objectC.Contents) != "c v2" {
		t.Errorf("expected object (/c, 'c v2'), got: %v", objectC)
	}

	objects, err = c.GetLatest(tc.Context(), 1, "/c")
	if err != nil {
		t.Fatalf("client.GetLatest with results: %v", err)
	}

	objectC = objects[0]
	if objectC.Path != "/c" || string(objectC.Contents) != "c v2" {
		t.Errorf("expected object (/c, 'c v2'), got: %v", objectC)
	}
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

	c, close := createTestServer(tc, tc.FsApi())
	defer close()

	version, err := c.UpdateObjects(tc.Context(), 1, []string{"/a", "/c"}, tmpDir)
	if err != nil {
		t.Fatalf("client.UpdateObjects: %v", err)
	}

	if version != 2 {
		t.Fatalf("expected version to increment to 2, got: %v", version)
	}

	objects, err := c.GetLatest(tc.Context(), 1, "")
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
