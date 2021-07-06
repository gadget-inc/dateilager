package test

import (
	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/pb"
	util "github.com/gadget-inc/dateilager/internal/testutil"
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

	contentBytes := []byte(content)
	h1, h2 := db.HashContent(contentBytes)

	_, err := conn.Exec(tc.Context(), `
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, permission, type, size, packed)
		VALUES ($1, $2, $3, $4, ($5, $6), $7, $8, $9, $10)
	`, project, start, stop, path, h1, h2, 0666, pb.Object_REGULAR, len(contentBytes), false)
	if err != nil {
		tc.Fatalf("insert object: %v", err)
	}

	contentEncoder := db.NewContentEncoder()
	encoded, err := contentEncoder.Encode(contentBytes)
	if err != nil {
		tc.Fatalf("encode content: %v", err)
	}

	_, err = conn.Exec(tc.Context(), `
		INSERT INTO dl.contents (hash, bytes)
		VALUES (($1, $2), $3)
		ON CONFLICT
		   DO NOTHING
	`, h1, h2, encoded)
	if err != nil {
		tc.Fatalf("insert contents: %v", err)
	}
}
