package test

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/pb"
	util "github.com/gadget-inc/dateilager/internal/testutil"
	"github.com/stretchr/testify/require"
)

func i(i int64) *int64 {
	return &i
}

type expectedObject struct {
	content string
	deleted bool
	mode    int64
}

func writeProject(tc util.TestCtx, id int32, latestVersion int64, packPatterns ...string) {
	conn := tc.Connect()
	_, err := conn.Exec(tc.Context(), `
		INSERT INTO dl.projects (id, latest_version, pack_patterns)
		VALUES ($1, $2, $3)
	`, id, latestVersion, packPatterns)
	require.NoError(tc.T(), err, "insert project")
}

func writeObjectFull(tc util.TestCtx, project int64, start int64, stop *int64, path, content string, mode fs.FileMode) {
	conn := tc.Connect()

	contentBytes := []byte(content)
	h1, h2 := db.HashContent(contentBytes)

	_, err := conn.Exec(tc.Context(), `
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		VALUES ($1, $2, $3, $4, ($5, $6), $7, $8, $9)
	`, project, start, stop, path, h1, h2, mode, len(contentBytes), false)
	require.NoError(tc.T(), err, "insert object")

	contentEncoder := db.NewContentEncoder()
	encoded, err := contentEncoder.Encode(contentBytes)
	require.NoError(tc.T(), err, "encode content")

	_, err = conn.Exec(tc.Context(), `
		INSERT INTO dl.contents (hash, bytes)
		VALUES (($1, $2), $3)
		ON CONFLICT
		   DO NOTHING
	`, h1, h2, encoded)
	require.NoError(tc.T(), err, "insert contents")
}

func writeObject(tc util.TestCtx, project int64, start int64, stop *int64, path string, contents ...string) {
	var content string
	if len(contents) == 0 {
		content = ""
	} else {
		content = contents[0]
	}

	writeObjectFull(tc, project, start, stop, path, content, 0755)
}

func writeEmptyDir(tc util.TestCtx, project int64, start int64, stop *int64, path string) {
	mode := fs.FileMode(0755)
	mode |= fs.ModeDir

	writeObjectFull(tc, project, start, stop, path, "", mode)
}

func writeSymlink(tc util.TestCtx, project int64, start int64, stop *int64, path, target string) {
	mode := fs.FileMode(0755)
	mode |= fs.ModeSymlink

	writeObjectFull(tc, project, start, stop, path, target, mode)
}

func writePackedObjects(tc util.TestCtx, project int64, start int64, stop *int64, path string, objects map[string]expectedObject) {
	conn := tc.Connect()

	contentsTar, namesTar := packObjects(tc, objects)
	h1, h2 := db.HashContent(contentsTar)

	_, err := conn.Exec(tc.Context(), `
		INSERT INTO dl.objects (project, start_version, stop_version, path, hash, mode, size, packed)
		VALUES ($1, $2, $3, $4, ($5, $6), $7, $8, $9)
	`, project, start, stop, path, h1, h2, 0755, len(contentsTar), true)
	require.NoError(tc.T(), err, "insert object")

	_, err = conn.Exec(tc.Context(), `
		INSERT INTO dl.contents (hash, bytes, names_tar)
		VALUES (($1, $2), $3, $4)
		ON CONFLICT
		DO NOTHING
	`, h1, h2, contentsTar, namesTar)
	require.NoError(tc.T(), err, "insert contents")
}

func packObjects(tc util.TestCtx, objects map[string]expectedObject) ([]byte, []byte) {
	contentWriter := db.NewTarWriter()
	namesWriter := db.NewTarWriter()

	for path, info := range objects {
		mode := info.mode
		if mode == 0 {
			mode = 0755
		}

		object := &pb.Object{
			Path:    path,
			Mode:    mode,
			Size:    int64(len(info.content)),
			Content: []byte(info.content),
			Deleted: info.deleted,
		}

		err := contentWriter.WriteObject(object, true)
		require.NoError(tc.T(), err, "write content to TAR")

		err = namesWriter.WriteObject(object, false)
		require.NoError(tc.T(), err, "write name to TAR")
	}

	contentTar, err := contentWriter.BytesAndReset()
	require.NoError(tc.T(), err, "write content TAR to bytes")

	namesTar, err := namesWriter.BytesAndReset()
	require.NoError(tc.T(), err, "write names TAR to bytes")

	return contentTar, namesTar
}

// Use debugProjects(tc) and debugObjects(tc) within a failing test to log the state of the DB

//lint:ignore U1000 debug utility
func debugProjects(tc util.TestCtx) {
	conn := tc.Connect()
	rows, err := conn.Query(tc.Context(), `
		SELECT id, latest_version, pack_patterns
		FROM dl.projects
	`)
	require.NoError(tc.T(), err, "debug execute project list")

	fmt.Println("\n[DEBUG] Projects")
	fmt.Println("id,\tlatest_version,\tpack_patterns")

	for rows.Next() {
		var id, latestVersion int64
		var packPatterns []string
		err = rows.Scan(&id, &latestVersion, &packPatterns)
		require.NoError(tc.T(), err, "debug scan project")

		fmt.Printf("%d,\t%d,\t\t%v\n", id, latestVersion, packPatterns)
	}

	fmt.Println()
}

//lint:ignore U1000 debug utility
func debugObjects(tc util.TestCtx) {
	conn := tc.Connect()
	rows, err := conn.Query(tc.Context(), `
		SELECT project, start_version, stop_version, path, mode, size, packed
		FROM dl.objects
	`)
	require.NoError(tc.T(), err, "debug execute object list")

	fmt.Println("\n[DEBUG] Objects")
	fmt.Println("project,\tstart_version,\tstop_version,\tpath,\tmode,\t\tsize,\tpacked")

	for rows.Next() {
		var project, start_version, mode, size int64
		var stop_version *int64
		var path string
		var packed bool
		err = rows.Scan(&project, &start_version, &stop_version, &path, &mode, &size, &packed)
		require.NoError(tc.T(), err, "debug scan object")

		fmt.Printf("%d,\t\t%d,\t\t%s,\t\t%s,\t%s,\t%d,\t%v\n", project, start_version, formatPtr(stop_version), path, formatMode(mode), size, packed)
	}

	fmt.Println()
}

func formatMode(mode int64) string {
	return fmt.Sprintf("%v", fs.FileMode(mode)&os.ModePerm)
}

func formatPtr(p *int64) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprint(*p)
}
