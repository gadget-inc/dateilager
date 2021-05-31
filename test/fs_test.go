package test

import (
	"context"
	"testing"

	util "github.com/angelini/dateilager/internal/testutil"
	"github.com/angelini/dateilager/pkg/api"
	"github.com/angelini/dateilager/pkg/pb"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

var (
	log, _ = zap.NewDevelopment()
)

func writeProject(tc util.TestCtx, id int32, latest_version int64) {
	conn := tc.Connect()
	_, err := conn.Exec(tc.Context(), `
		INSERT INTO dl.projects (id, latest_version)
		VALUES ($1, $2)
		`, id, latest_version)
	if err != nil {
		tc.Fatalf("inserting project: %v", err)
	}
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

func TestGetEmpty(t *testing.T) {
	tc := util.NewTestCtx(t)
	defer tc.Close()

	fs := api.Fs{
		Log:    log,
		DbConn: tc.Connector(),
	}

	writeProject(tc, 1, 1)

	stream := &mockGetServer{ctx: tc.Context()}
	err := fs.Get(&pb.GetRequest{Project: 1, Version: nil}, stream)
	if err != nil {
		t.Fatalf("fs.Get: %v", err)
	}

	if len(stream.results) != 0 {
		t.Fatalf("stream results should be empty: %v", stream.results)
	}
}
