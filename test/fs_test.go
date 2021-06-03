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

func exactQuery(project int32, version *int64, path string) *pb.GetRequest {
	query := &pb.ObjectQuery{
		Path:     path,
		IsPrefix: false,
	}

	return &pb.GetRequest{
		Project: project,
		Version: version,
		Queries: []*pb.ObjectQuery{query},
	}
}

func prefixQuery(project int32, version *int64, path string) *pb.GetRequest {
	query := &pb.ObjectQuery{
		Path:     path,
		IsPrefix: true,
	}

	return &pb.GetRequest{
		Project: project,
		Version: version,
		Queries: []*pb.ObjectQuery{query},
	}
}

func TestGetEmpty(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 1)

	fs := tc.FsApi(log)
	stream := &mockGetServer{ctx: tc.Context()}

	err := fs.Get(&pb.GetRequest{Project: 1, Version: nil}, stream)
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

	fs := tc.FsApi(log)
	stream := &mockGetServer{ctx: tc.Context()}

	err := fs.Get(exactQuery(1, nil, "/a"), stream)
	if err != nil {
		t.Fatalf("fs.Get: %v", err)
	}

	if len(stream.results) != 1 {
		t.Errorf("expected exactly 1 result, got: %v", len(stream.results))
	}

	result := stream.results[0]
	if result.Path != "/a" {
		t.Errorf("expected Path /a, got: %v", result.Path)
	}
}

func TestGetPrefix(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, nil, "/a/a")
	writeObject(tc, 1, 1, nil, "/a/b")
	writeObject(tc, 1, 1, nil, "/b/a")
	writeObject(tc, 1, 1, nil, "/b/b")

	fs := tc.FsApi(log)
	stream := &mockGetServer{ctx: tc.Context()}

	err := fs.Get(prefixQuery(1, nil, "/a"), stream)
	if err != nil {
		t.Fatalf("fs.Get: %v", err)
	}

	if len(stream.results) != 2 {
		t.Errorf("expected 2 results, got: %v", len(stream.results))
	}

	res := stream.results
	if res[0].Path != "/a/a" || res[1].Path != "/a/b" {
		t.Errorf("expected Paths (/a/a, /a/b), got: (%v, %v)", res[0].Path, res[1].Path)
	}
}

func TestGetExactlyOneVersioned(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, i(2), "/a", "v1")
	writeObject(tc, 1, 2, i(3), "/a", "v2")
	writeObject(tc, 1, 3, nil, "/a", "v3")

	fs := tc.FsApi(log)
	stream := &mockGetServer{ctx: tc.Context()}

	err := fs.Get(exactQuery(1, i(1), "/a"), stream)
	if err != nil {
		t.Fatalf("fs.Get version 1: %v", err)
	}

	contents := string(stream.results[0].Contents)
	if contents != "v1" {
		t.Errorf("expected Contents v1, got: %v", contents)
	}

	stream = &mockGetServer{ctx: tc.Context()}
	err = fs.Get(exactQuery(1, i(2), "/a"), stream)
	if err != nil {
		t.Fatalf("fs.Get version 2: %v", err)
	}

	contents = string(stream.results[0].Contents)
	if contents != "v2" {
		t.Errorf("expected Contents v2, got: %v", contents)
	}

	stream = &mockGetServer{ctx: tc.Context()}
	err = fs.Get(exactQuery(1, i(3), "/a"), stream)
	if err != nil {
		t.Fatalf("fs.Get version 3: %v", err)
	}

	contents = string(stream.results[0].Contents)
	if contents != "v3" {
		t.Errorf("expected Contents v3, got: %v", contents)
	}
}
