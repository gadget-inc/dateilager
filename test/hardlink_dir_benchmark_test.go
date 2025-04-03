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

//go:bench
func BenchmarkHardlinkDir(b *testing.B) {
	wd, err := os.Getwd()
	if err != nil {
		b.Error(err)
	}

	bigDir := path.Join(wd, "../js/node_modules")
	tmpDir := emptyTmpDir(b)
	defer os.RemoveAll(tmpDir)

	b.ResetTimer()

	b.Run("hardlink", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			copyDir := path.Join(tmpDir, "node_modules", fmt.Sprintf("%d", n))
			err := files.HardlinkDir(bigDir, copyDir)
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
	})
}
