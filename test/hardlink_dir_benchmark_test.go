package test

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/gadget-inc/dateilager/internal/files"
	"github.com/stretchr/testify/require"
)

func TestHardlinkDir(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err, "os.Getwd() failed")

	bigDir := path.Join(wd, "../js/node_modules")
	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)

	copyDir := path.Join(tmpDir, "node_modules")
	err = files.HardlinkDir(bigDir, copyDir)
	require.NoError(t, err, "HardlinkDir failed")

	err = CompareDirectories(bigDir, copyDir)
	require.NoError(t, err, "compareDirectories %s vs %s failed", bigDir, tmpDir)
}

func BenchmarkHardlinkDir(b *testing.B) {
	wd, err := os.Getwd()
	if err != nil {
		b.Error(err)
	}

	bigDir := path.Join(wd, "../js/node_modules")
	tmpDir := emptyTmpDir(b)
	defer os.RemoveAll(tmpDir)

	for n := 0; n < b.N; n++ {
		err := files.HardlinkDir(bigDir, path.Join(tmpDir, "node_modules", fmt.Sprintf("%d", n)))
		if err != nil {
			b.Error(err)
		}
	}
}
