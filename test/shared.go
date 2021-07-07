package test

import (
	"io/fs"

	"github.com/gadget-inc/dateilager/internal/db"
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

func writeObjectFull(tc util.TestCtx, project int32, start int64, stop *int64, path, content string, mode fs.FileMode) {
	conn := tc.Connect()

	contentBytes := []byte(content)
	h1, h2 := db.HashContent(contentBytes)

	_, err := conn.Exec(tc.Context(), `
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		VALUES ($1, $2, $3, $4, ($5, $6), $7, $8, $9)
	`, project, start, stop, path, h1, h2, mode, len(contentBytes), false)
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

func writeObject(tc util.TestCtx, project int32, start int64, stop *int64, path string, contents ...string) {
	var content string
	if len(contents) == 0 {
		content = ""
	} else {
		content = contents[0]
	}

	writeObjectFull(tc, project, start, stop, path, content, 0755)
}

func writeEmptyDir(tc util.TestCtx, project int32, start int64, stop *int64, path string) {
	mode := fs.FileMode(0755)
	mode |= fs.ModeDir

	writeObjectFull(tc, project, start, stop, path, "", mode)
}

func writeSymlink(tc util.TestCtx, project int32, start int64, stop *int64, path, target string) {
	mode := fs.FileMode(0755)
	mode |= fs.ModeSymlink

	writeObjectFull(tc, project, start, stop, path, target, mode)
}
