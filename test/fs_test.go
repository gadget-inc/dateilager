package test

import (
	"context"
	"testing"

	"github.com/angelini/dateilager/internal/pb"
	util "github.com/angelini/dateilager/internal/testutil"
	"google.golang.org/grpc"
)

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

func buildRequest(project int32, fromVersion, toVersion *int64, path string, prefix, content bool) *pb.GetRequest {
	query := &pb.ObjectQuery{
		Path:        path,
		IsPrefix:    prefix,
		WithContent: content,
	}

	return &pb.GetRequest{
		Project:     project,
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		Queries:     []*pb.ObjectQuery{query},
	}
}

func exactQuery(project int32, version *int64, path string) *pb.GetRequest {
	return buildRequest(project, nil, version, path, false, true)
}

func prefixQuery(project int32, version *int64, path string) *pb.GetRequest {
	return buildRequest(project, nil, version, path, true, true)
}

func noContentQuery(project int32, version *int64, path string) *pb.GetRequest {
	return buildRequest(project, nil, version, path, true, false)
}

func rangeQuery(project int32, fromVersion, toVersion *int64, path string) *pb.GetRequest {
	return buildRequest(project, fromVersion, toVersion, path, true, true)
}

func buildCompressRequest(project int32, fromVersion, toVersion *int64, path string) *pb.GetCompressRequest {
	query := &pb.ObjectQuery{
		Path:        path,
		IsPrefix:    true,
		WithContent: true,
	}

	return &pb.GetCompressRequest{
		Project:     project,
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		Queries:     []*pb.ObjectQuery{query},
	}
}

type expectedObject struct {
	deleted bool
	content string
}

func verifyStreamResults(tc util.TestCtx, results []*pb.Object, expected map[string]expectedObject) {
	if len(results) != len(expected) {
		tc.Errorf("expected %v objects, got: %v", len(expected), len(results))
	}

	for _, result := range results {
		object, ok := expected[result.Path]
		if !ok {
			tc.Fatalf("missing %v in stream results", result.Path)
		}

		if string(result.Content) != object.content {
			tc.Errorf("mismatch content for %v expected '%v', got '%v'", result.Path, object.content, string(result.Content))
		}

		if result.Deleted != object.deleted {
			tc.Errorf("mismatch deleted flag for %v expected %v, got %v", result.Path, object.deleted, result.Deleted)
		}
	}
}

func TestGetEmpty(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 1)

	fs := tc.FsApi()
	stream := &mockGetServer{ctx: tc.Context()}

	err := fs.Get(&pb.GetRequest{Project: 1}, stream)
	if err != nil {
		t.Fatalf("fs.Get: %v", err)
	}

	if len(stream.results) != 0 {
		t.Fatalf("stream results should be empty: %v", stream.results)
	}
}

func TestGetExactlyOne(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a")
	writeObject(tc, 1, 1, nil, "/b")

	fs := tc.FsApi()
	stream := &mockGetServer{ctx: tc.Context()}

	err := fs.Get(exactQuery(1, nil, "/a"), stream)
	if err != nil {
		t.Fatalf("fs.Get: %v", err)
	}

	verifyStreamResults(tc, stream.results, map[string]expectedObject{
		"/a": {
			content: "",
			deleted: false,
		},
	})
}

func TestGetPrefix(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a/a")
	writeObject(tc, 1, 1, nil, "/a/b")
	writeObject(tc, 1, 1, nil, "/b/a")
	writeObject(tc, 1, 1, nil, "/b/b")

	fs := tc.FsApi()
	stream := &mockGetServer{ctx: tc.Context()}

	err := fs.Get(prefixQuery(1, nil, "/a"), stream)
	if err != nil {
		t.Fatalf("fs.Get: %v", err)
	}

	verifyStreamResults(tc, stream.results, map[string]expectedObject{
		"/a/a": {
			content: "",
			deleted: false,
		},
		"/a/b": {
			content: "",
			deleted: false,
		},
	})
}

func TestGetRange(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 4)
	writeObject(tc, 1, 1, i(3), "/a")
	writeObject(tc, 1, 2, i(3), "/b", "b v2")
	writeObject(tc, 1, 3, nil, "/b", "b v3")
	writeObject(tc, 1, 3, nil, "/c")
	writeObject(tc, 1, 4, nil, "/d")

	fs := tc.FsApi()

	stream := &mockGetServer{ctx: tc.Context()}
	err := fs.Get(rangeQuery(1, i(1), i(2), ""), stream)
	if err != nil {
		t.Fatalf("fs.Get 1 to 2: %v", err)
	}

	verifyStreamResults(tc, stream.results, map[string]expectedObject{
		"/b": {
			content: "b v2",
			deleted: false,
		},
	})

	stream = &mockGetServer{ctx: tc.Context()}
	err = fs.Get(rangeQuery(1, i(1), i(3), ""), stream)
	if err != nil {
		t.Fatalf("fs.Get 1 to 3: %v", err)
	}

	verifyStreamResults(tc, stream.results, map[string]expectedObject{
		"/a": {
			content: "",
			deleted: true,
		},
		"/b": {
			content: "b v3",
			deleted: false,
		},
		"/c": {
			content: "",
			deleted: false,
		},
	})
}

func TestGetDeleteAll(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, i(2), "/a")
	writeObject(tc, 1, 1, i(3), "/b")
	writeObject(tc, 1, 1, i(3), "/c")

	fs := tc.FsApi()

	stream := &mockGetServer{ctx: tc.Context()}
	err := fs.Get(rangeQuery(1, i(1), i(2), ""), stream)
	if err != nil {
		t.Fatalf("fs.Get 1 to 2: %v", err)
	}

	verifyStreamResults(tc, stream.results, map[string]expectedObject{
		"/a": {
			content: "",
			deleted: true,
		},
	})

	stream = &mockGetServer{ctx: tc.Context()}
	err = fs.Get(rangeQuery(1, i(1), i(3), ""), stream)
	if err != nil {
		t.Fatalf("fs.Get 1 to 3: %v", err)
	}

	verifyStreamResults(tc, stream.results, map[string]expectedObject{
		"/a": {
			content: "",
			deleted: true,
		},
		"/b": {
			content: "",
			deleted: true,
		},
		"/c": {
			content: "",
			deleted: true,
		},
	})

	stream = &mockGetServer{ctx: tc.Context()}
	err = fs.Get(rangeQuery(1, i(2), i(3), ""), stream)
	if err != nil {
		t.Fatalf("fs.Get 2 to 3: %v", err)
	}

	verifyStreamResults(tc, stream.results, map[string]expectedObject{
		"/b": {
			content: "",
			deleted: true,
		},
		"/c": {
			content: "",
			deleted: true,
		},
	})
}

func TestGetExactlyOneVersioned(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 3)
	writeObject(tc, 1, 1, i(2), "/a", "v1")
	writeObject(tc, 1, 2, i(3), "/a", "v2")
	writeObject(tc, 1, 3, nil, "/a", "v3")

	fs := tc.FsApi()

	stream := &mockGetServer{ctx: tc.Context()}
	err := fs.Get(exactQuery(1, i(1), "/a"), stream)
	if err != nil {
		t.Fatalf("fs.Get version 1: %v", err)
	}

	verifyStreamResults(tc, stream.results, map[string]expectedObject{
		"/a": {
			content: "v1",
			deleted: false,
		},
	})

	stream = &mockGetServer{ctx: tc.Context()}
	err = fs.Get(exactQuery(1, i(2), "/a"), stream)
	if err != nil {
		t.Fatalf("fs.Get version 2: %v", err)
	}

	verifyStreamResults(tc, stream.results, map[string]expectedObject{
		"/a": {
			content: "v2",
			deleted: false,
		},
	})

	stream = &mockGetServer{ctx: tc.Context()}
	err = fs.Get(exactQuery(1, i(3), "/a"), stream)
	if err != nil {
		t.Fatalf("fs.Get version 3: %v", err)
	}

	verifyStreamResults(tc, stream.results, map[string]expectedObject{
		"/a": {
			content: "v3",
			deleted: false,
		},
	})
}

func TestGetWithoutContent(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 3)
	writeObject(tc, 1, 1, nil, "/a", "a v1")
	writeObject(tc, 1, 2, nil, "/b", "b v2")
	writeObject(tc, 1, 3, nil, "/c", "c v3")

	fs := tc.FsApi()

	stream := &mockGetServer{ctx: tc.Context()}
	err := fs.Get(noContentQuery(1, nil, ""), stream)
	if err != nil {
		t.Fatalf("fs.Get: %v", err)
	}

	verifyStreamResults(tc, stream.results, map[string]expectedObject{
		"/a": {
			content: "",
			deleted: false,
		},
		"/b": {
			content: "",
			deleted: false,
		},
		"/c": {
			content: "",
			deleted: false,
		},
	})
}

func TestCompress(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 3)
	writeObject(tc, 1, 1, i(2), "/a", "v1")
	writeObject(tc, 1, 2, i(3), "/a", "v2")
	writeObject(tc, 1, 3, nil, "/a", "v3")

	fs := tc.FsApi()

	stream := &mockGetCompressServer{ctx: tc.Context()}
	err := fs.GetCompress(buildCompressRequest(1, nil, nil, ""), stream)
	if err != nil {
		t.Fatalf("fs.Get version 1: %v", err)
	}

	// FIXME: It should only return 1 TAR
	if len(stream.results) != 2 {
		t.Errorf("expected 2 TAR files, got: %v", len(stream.results))
	}
}

func TestPack(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, nil, "/a/c", "a/c v1")
	writeObject(tc, 1, 1, nil, "/a/d", "a/d v1")
	writeObject(tc, 1, 2, nil, "/a/e", "a/e v2")
	writeObject(tc, 1, 2, nil, "/b", "b v2")

	fs := tc.FsApi()

	request := pb.PackRequest{
		Project: 1,
		Path:    "/a/",
	}
	response, err := fs.Pack(tc.Context(), &request)
	if err != nil {
		t.Fatalf("fs.Get: %v", err)
	}

	if response.Version != 3 {
		t.Errorf("expected version 3, got: %v", response.Version)
	}

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(prefixQuery(1, nil, "/a"), stream)
	if err != nil {
		t.Fatalf("fs.Get: %v", err)
	}

	verifyStreamResults(tc, stream.results, map[string]expectedObject{
		"/a/c": {
			content: "a/c v1",
			deleted: false,
		},
		"/a/d": {
			content: "a/d v1",
			deleted: false,
		},
		"/a/e": {
			content: "a/e v2",
			deleted: false,
		},
	})
}
