package test

import (
	util "github.com/angelini/dateilager/internal/testutil"
	"github.com/angelini/dateilager/pkg/api"
	"go.uber.org/zap"
)

var (
	log, _ = zap.NewDevelopment()
)

func i(i int64) *int64 {
	return &i
}

func writeProject(tc util.TestCtx, id int32, latest_version int64) {
	conn := tc.Connect()
	_, err := conn.Exec(tc.Context(), `
		INSERT INTO dl.projects (id, latest_version)
		VALUES ($1, $2)
	`, id, latest_version)
	if err != nil {
		tc.Fatalf("insert project: %v", err)
	}
}

func writeObject(tc util.TestCtx, project int32, start int64, stop *int64, path string, contents ...string) {
	conn := tc.Connect()

	var content string
	if len(contents) == 0 {
		content = ""
	} else {
		content = contents[0]
	}
	h1, h2 := api.HashContents([]byte(content))

	_, err := conn.Exec(tc.Context(), `
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size)
		VALUES ($1, $2, $3, $4, ($5, $6), $7, $8)
	`, project, start, stop, path, h1, h2, 0, 0)
	if err != nil {
		tc.Fatalf("insert object: %v", err)
	}

	_, err = conn.Exec(tc.Context(), `
		INSERT INTO dl.contents (hash, bytes)
		VALUES (($1, $2), $3)
		ON CONFLICT
		   DO NOTHING
	`, h1, h2, content)
	if err != nil {
		tc.Fatalf("insert contents: %v", err)
	}
}
