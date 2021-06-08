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
		Project:     project,
		FromVersion: nil,
		ToVersion:   version,
		Queries:     []*pb.ObjectQuery{query},
	}
}

func prefixQuery(project int32, version *int64, path string) *pb.GetRequest {
	query := &pb.ObjectQuery{
		Path:     path,
		IsPrefix: true,
	}

	return &pb.GetRequest{
		Project:     project,
		FromVersion: nil,
		ToVersion:   version,
		Queries:     []*pb.ObjectQuery{query},
	}
}

func rangeQuery(project int32, fromVersion, toVersion *int64, path string) *pb.GetRequest {
	query := &pb.ObjectQuery{
		Path:     path,
		IsPrefix: true,
	}

	return &pb.GetRequest{
		Project:     project,
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		Queries:     []*pb.ObjectQuery{query},
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

	fs := tc.FsApi()
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

	if len(stream.results) != 1 {
		t.Errorf("expected 1 result, got: %v", len(stream.results))
	}

	res := stream.results
	if res[0].Path != "/b" {
		t.Errorf("expected /b, got: %v", res[0])
	}

	stream = &mockGetServer{ctx: tc.Context()}
	err = fs.Get(rangeQuery(1, i(1), i(3), ""), stream)
	if err != nil {
		t.Fatalf("fs.Get 1 to 3: %v", err)
	}

	if len(stream.results) != 3 {
		t.Errorf("expected 3 results, got: %v", len(stream.results))
	}

	res = stream.results
	if res[0].Path != "/a" || !res[0].Deleted {
		t.Errorf("expected (/a, deleted), got: %v", res[0])
	}
}

func TestDeleteAll(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 3)
	writeObject(tc, 1, 1, i(2), "/a")
	writeObject(tc, 1, 1, i(3), "/b")
	writeObject(tc, 1, 1, i(3), "/c")

	fs := tc.FsApi()

	stream := &mockGetServer{ctx: tc.Context()}
	err := fs.Get(rangeQuery(1, i(1), i(2), ""), stream)
	if err != nil {
		t.Fatalf("fs.Get 1 to 2: %v", err)
	}

	if len(stream.results) != 1 {
		t.Errorf("expected 1 results, got: %v", len(stream.results))
	}

	res := stream.results
	if !res[0].Deleted {
		t.Errorf("expected deleted /a, got: %v", res[0])
	}

	stream = &mockGetServer{ctx: tc.Context()}
	err = fs.Get(rangeQuery(1, i(1), i(3), ""), stream)
	if err != nil {
		t.Fatalf("fs.Get 1 to 3: %v", err)
	}

	if len(stream.results) != 3 {
		t.Errorf("expected 3 results, got: %v", len(stream.results))
	}

	res = stream.results
	if !res[0].Deleted || !res[1].Deleted || !res[2].Deleted {
		t.Errorf("expected all deleted, got: %v", res)
	}

	stream = &mockGetServer{ctx: tc.Context()}
	err = fs.Get(rangeQuery(1, i(2), i(3), ""), stream)
	if err != nil {
		t.Fatalf("fs.Get 2 to 3: %v", err)
	}

	if len(stream.results) != 2 {
		t.Errorf("expected 2 results, got: %v", len(stream.results))
	}

	res = stream.results
	if !res[0].Deleted || !res[1].Deleted {
		t.Errorf("expected all deleted, got: %v", res)
	}
}

func TestGetExactlyOneVersioned(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	writeProject(tc, 1, 1)
	writeObject(tc, 1, 1, i(2), "/a", "v1")
	writeObject(tc, 1, 2, i(3), "/a", "v2")
	writeObject(tc, 1, 3, nil, "/a", "v3")

	fs := tc.FsApi()

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
