package test

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/gadget-inc/dateilager/internal/files"
	"github.com/stretchr/testify/require"
)

func TestReflink(t *testing.T) {
	// Skip the test if reflinks are not supported
	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)
	if !files.HasReflinkSupport(tmpDir) {
		t.Skip("Reflinks are not supported on this filesystem")
	}

	wd, err := os.Getwd()
	require.NoError(t, err, "os.Getwd() failed")

	bigDir := path.Join(wd, "../js/node_modules")
	tmpDir = emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	copyDir := path.Join(tmpDir, "node_modules")
	err = files.Reflink(bigDir, copyDir)
	require.NoError(t, err, "Reflink failed")

	err = CompareDirectories(bigDir, copyDir)
	require.NoError(t, err, "compareDirectories %s vs %s failed", bigDir, tmpDir)
}

//go:bench
func BenchmarkReflink(b *testing.B) {
	// Skip the benchmark if reflinks are not supported
	tmpDir := emptyTmpDir(b)
	defer os.RemoveAll(tmpDir)
	if !files.HasReflinkSupport(tmpDir) {
		b.Skip("Reflinks are not supported on this filesystem")
	}

	wd, err := os.Getwd()
	if err != nil {
		b.Error(err)
	}

	bigDir := path.Join(wd, "../js/node_modules")
	tmpDir = emptyTmpDir(b)
	defer os.RemoveAll(tmpDir)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		copyDir := path.Join(tmpDir, "node_modules", fmt.Sprintf("%d", n))
		err := files.Reflink(bigDir, copyDir)
		b.StopTimer()
		if err != nil {
			b.Error(err)
		}

		err = CompareDirectories(bigDir, copyDir)
		if err != nil {
			b.Error(err)
		}
		b.StartTimer()
	}
}
