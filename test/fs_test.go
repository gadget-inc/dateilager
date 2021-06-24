package test

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/angelini/dateilager/internal/pb"
	util "github.com/angelini/dateilager/internal/testutil"
	"github.com/klauspost/compress/zstd"
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

func verifyTarResults(tc util.TestCtx, results [][]byte, expected map[string]expectedObject) {
	count := 0

	for _, result := range results {
		zstdReader, err := zstd.NewReader(bytes.NewBuffer(result))
		if err != nil {
			tc.Fatalf("failed to create zstdReader %v", err)
		}
		defer zstdReader.Close()

		tarReader := tar.NewReader(zstdReader)

		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				tc.Fatalf("failed to read next TAR file %v", err)
			}

			expectedMatch, ok := expected[header.Name]
			if !ok {
				tc.Errorf("missing %v in TAR", header.Name)
			}

			count += 1

			var buffer bytes.Buffer
			_, err = io.Copy(&buffer, tarReader)
			if err != nil {
				tc.Fatalf("failed to copy content bytes from TAR %v", err)
			}

			if !bytes.Equal([]byte(expectedMatch.content), buffer.Bytes()) {
				tc.Errorf("mismatch content for %v expected '%v', got '%v'", header.Name, expectedMatch.content, buffer.String())
			}
		}
	}

	if count != len(expected) {
		tc.Errorf("expected %v objects, got: %v", len(expected), count)
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
		"/a": {content: ""},
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
		"/a/a": {content: ""},
		"/a/b": {content: ""},
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
		"/b": {content: "b v3"},
		"/c": {content: ""},
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
		"/a": {content: "v1"},
	})

	stream = &mockGetServer{ctx: tc.Context()}
	err = fs.Get(exactQuery(1, i(2), "/a"), stream)
	if err != nil {
		t.Fatalf("fs.Get version 2: %v", err)
	}

	verifyStreamResults(tc, stream.results, map[string]expectedObject{
		"/a": {content: "v2"},
	})

	stream = &mockGetServer{ctx: tc.Context()}
	err = fs.Get(exactQuery(1, i(3), "/a"), stream)
	if err != nil {
		t.Fatalf("fs.Get version 3: %v", err)
	}

	verifyStreamResults(tc, stream.results, map[string]expectedObject{
		"/a": {content: "v3"},
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
		"/a": {content: ""},
		"/b": {content: ""},
		"/c": {content: ""},
	})
}

func TestGetCompress(t *testing.T) {
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
		t.Fatalf("fs.GetCompress: %v", err)
	}

	if len(stream.results) != 1 {
		t.Errorf("expected 1 TAR files, got: %v", len(stream.results))
	}

	verifyTarResults(tc, stream.results, map[string]expectedObject{
		"/a": {content: "v3"},
	})
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
		t.Fatalf("fs.Pack: %v", err)
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
		"/a/c": {content: "a/c v1"},
		"/a/d": {content: "a/d v1"},
		"/a/e": {content: "a/e v2"},
	})
}

func TestGetPackedObjectsWithoutContent(t *testing.T) {
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
		t.Fatalf("fs.Pack: %v", err)
	}

	if response.Version != 3 {
		t.Errorf("expected version 3, got: %v", response.Version)
	}

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(noContentQuery(1, nil, "/a"), stream)
	if err != nil {
		t.Fatalf("fs.Get: %v", err)
	}

	verifyStreamResults(tc, stream.results, map[string]expectedObject{
		"/a/c": {content: ""},
		"/a/d": {content: ""},
		"/a/e": {content: ""},
	})
}

func TestGetObjectWithinPack(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a/b", "a/b v1")
	writeObject(tc, 1, 1, nil, "/a/c", "a/c v1")

	fs := tc.FsApi()

	request := pb.PackRequest{
		Project: 1,
		Path:    "/a/",
	}
	response, err := fs.Pack(tc.Context(), &request)
	if err != nil {
		t.Fatalf("fs.Pack: %v", err)
	}

	if response.Version != 2 {
		t.Errorf("expected version 2, got: %v", response.Version)
	}

	stream := &mockGetServer{ctx: tc.Context()}
	err = fs.Get(exactQuery(1, nil, "/a/b"), stream)
	if err != nil {
		t.Fatalf("fs.Get: %v", err)
	}

	verifyStreamResults(tc, stream.results, map[string]expectedObject{
		"/a/b": {content: "a/b v1"},
	})
}

func TestGetCompressReturnsPackedObjectsWithoutRepacking(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 2)
	writeObject(tc, 1, 1, nil, "/a/c", "a/c v1")
	writeObject(tc, 1, 1, nil, "/a/d", "a/d v1")
	writeObject(tc, 1, 2, nil, "/b", "b v2")

	fs := tc.FsApi()

	request := pb.PackRequest{
		Project: 1,
		Path:    "/a/",
	}
	response, err := fs.Pack(tc.Context(), &request)
	if err != nil {
		t.Fatalf("fs.Pack: %v", err)
	}

	if response.Version != 3 {
		t.Errorf("expected version 3, got: %v", response.Version)
	}

	stream := &mockGetCompressServer{ctx: tc.Context()}
	err = fs.GetCompress(buildCompressRequest(1, nil, nil, ""), stream)
	if err != nil {
		t.Fatalf("fs.GetCompress: %v", err)
	}

	if len(stream.results) != 2 {
		t.Errorf("expected 2 TAR files, got: %v", len(stream.results))
	}

	verifyTarResults(tc, stream.results, map[string]expectedObject{
		"/a/c": {content: "a/c v1"},
		"/a/d": {content: "a/d v1"},
		"/b":   {content: "b v2"},
	})
}
