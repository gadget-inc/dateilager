package test

import (
	"os"
	"path"
	"testing"

	"github.com/gadget-inc/dateilager/internal/files"
	"github.com/stretchr/testify/require"
)

type dirOperation func(src, dst string) error

func TestDirOperations(t *testing.T) {
	operations := map[string]dirOperation{
		"HardlinkDir": files.HardlinkDir,
	}

	// Only add ReflinkDir to operations if reflinks are supported
	tmpDir := emptyTmpDir(t)
	defer os.RemoveAll(tmpDir)
	if files.HasReflinkSupport(tmpDir) {
		operations["ReflinkDir"] = files.ReflinkDir
	}

	for name, op := range operations {
		t.Run(name, func(t *testing.T) {
			t.Run("Basic", func(t *testing.T) {
				tmpDir := emptyTmpDir(t)
				defer os.RemoveAll(tmpDir)

				// Create source directory with a file
				srcDir := path.Join(tmpDir, "src")
				err := os.MkdirAll(srcDir, 0755)
				require.NoError(t, err, "failed to create source directory")
				err = os.WriteFile(path.Join(srcDir, "test.txt"), []byte("test content"), 0644)
				require.NoError(t, err, "failed to create test file")

				// Create destination directory
				dstDir := path.Join(tmpDir, "dst")
				err = op(srcDir, dstDir)
				require.NoError(t, err, "%s failed", name)

				// Verify the directories match
				err = CompareDirectories(srcDir, dstDir)
				require.NoError(t, err, "compareDirectories failed")

				// For hardlinks, verify that the files are actually hardlink'd
				if name == "HardlinkDir" {
					srcInfo, err := os.Stat(path.Join(srcDir, "test.txt"))
					require.NoError(t, err, "failed to stat source file")
					dstInfo, err := os.Stat(path.Join(dstDir, "test.txt"))
					require.NoError(t, err, "failed to stat destination file")
					require.Equal(t, srcInfo.Sys(), dstInfo.Sys(), "files should be hardlink'd")
				}
			})

			t.Run("WithNestedDirs", func(t *testing.T) {
				tmpDir := emptyTmpDir(t)
				defer os.RemoveAll(tmpDir)

				// Create a nested directory structure
				srcDir := path.Join(tmpDir, "src")
				err := os.MkdirAll(path.Join(srcDir, "a/b/c"), 0755)
				require.NoError(t, err, "failed to create nested directories")

				// Create some files in the nested structure
				files := map[string]string{
					"a/file1.txt":     "content1",
					"a/b/file2.txt":   "content2",
					"a/b/c/file3.txt": "content3",
				}

				for file, content := range files {
					err := os.WriteFile(path.Join(srcDir, file), []byte(content), 0644)
					require.NoError(t, err, "failed to create file %s", file)
				}

				// Create destination directory
				dstDir := path.Join(tmpDir, "dst")
				err = op(srcDir, dstDir)
				require.NoError(t, err, "%s failed", name)

				// Verify the directories match
				err = CompareDirectories(srcDir, dstDir)
				require.NoError(t, err, "compareDirectories failed")

				// For hardlinks, verify that all files are hardlink'd
				if name == "HardlinkDir" {
					for file := range files {
						srcInfo, err := os.Stat(path.Join(srcDir, file))
						require.NoError(t, err, "failed to stat source file %s", file)
						dstInfo, err := os.Stat(path.Join(dstDir, file))
						require.NoError(t, err, "failed to stat destination file %s", file)
						require.Equal(t, srcInfo.Sys(), dstInfo.Sys(), "files should be hardlink'd: %s", file)
					}
				}
			})

			t.Run("WithExistingDest", func(t *testing.T) {
				tmpDir := emptyTmpDir(t)
				defer os.RemoveAll(tmpDir)

				// Create source directory with a file
				srcDir := path.Join(tmpDir, "src")
				err := os.MkdirAll(srcDir, 0755)
				require.NoError(t, err, "failed to create source directory")
				err = os.WriteFile(path.Join(srcDir, "test.txt"), []byte("test content"), 0644)
				require.NoError(t, err, "failed to create test file")

				// Create destination directory with existing content
				dstDir := path.Join(tmpDir, "dst")
				err = os.MkdirAll(dstDir, 0755)
				require.NoError(t, err, "failed to create destination directory")
				err = os.WriteFile(path.Join(dstDir, "existing.txt"), []byte("existing content"), 0644)
				require.NoError(t, err, "failed to create existing file")

				// Attempt to copy
				err = op(srcDir, dstDir)
				require.NoError(t, err, "%s failed", name)

				// Verify the directories match
				err = CompareDirectories(srcDir, dstDir)
				require.NoError(t, err, "compareDirectories failed")
			})
		})
	}
}
