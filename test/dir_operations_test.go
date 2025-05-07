package test

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"testing"

	"github.com/gadget-inc/dateilager/internal/files"
	"github.com/stretchr/testify/require"
)

type dirOperation func(src, dst string) error

func TestDirOperations(t *testing.T) {
	operations := map[string]dirOperation{
		"Hardlink": files.Hardlink,
	}

	// Only add Reflink to operations if reflinks are supported
	tmpDir := emptyTmpDir(t)
	if files.HasReflinkSupport(tmpDir) {
		operations["Reflink"] = files.Reflink
	}
	os.RemoveAll(tmpDir)

	for name, op := range operations {
		t.Run(name, func(t *testing.T) {
			t.Run("Basic", func(t *testing.T) {
				tmpDir := emptyTmpDir(t)
				defer os.RemoveAll(tmpDir)

				// Create source directory with a file
				srcDir := path.Join(tmpDir, "src")
				err := os.MkdirAll(srcDir, 0o755)
				require.NoError(t, err, "failed to create source directory")
				err = os.WriteFile(path.Join(srcDir, "test.txt"), []byte("test content"), 0o644)
				require.NoError(t, err, "failed to create test file")

				// Create destination directory
				dstDir := path.Join(tmpDir, "dst")
				err = op(srcDir, dstDir)
				require.NoError(t, err, "%s failed", name)

				// Verify the directories match
				err = CompareDirectories(srcDir, dstDir)
				require.NoError(t, err, "compareDirectories failed")

				// For hardlinks, verify that the files are actually hardlink'd
				if name == "Hardlink" {
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
				err := os.MkdirAll(path.Join(srcDir, "a/b/c"), 0o755)
				require.NoError(t, err, "failed to create nested directories")

				// Create some files in the nested structure
				files := map[string]string{
					"a/file1.txt":     "content1",
					"a/b/file2.txt":   "content2",
					"a/b/c/file3.txt": "content3",
				}

				for file, content := range files {
					err := os.WriteFile(path.Join(srcDir, file), []byte(content), 0o644)
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
				if name == "Hardlink" {
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
				err := os.MkdirAll(srcDir, 0o755)
				require.NoError(t, err, "failed to create source directory")
				err = os.WriteFile(path.Join(srcDir, "test.txt"), []byte("test content"), 0o644)
				require.NoError(t, err, "failed to create test file")

				// Create destination directory with existing content
				dstDir := path.Join(tmpDir, "dst")
				err = os.MkdirAll(dstDir, 0o755)
				require.NoError(t, err, "failed to create destination directory")
				err = os.WriteFile(path.Join(dstDir, "existing.txt"), []byte("existing content"), 0o644)
				require.NoError(t, err, "failed to create existing file")

				// Attempt to copy
				err = op(srcDir, dstDir)
				require.NoError(t, err, "%s failed", name)

				// Verify the directories match
				err = CompareDirectories(srcDir, dstDir)
				require.NoError(t, err, "compareDirectories failed")
			})

			t.Run("WithSymlink", func(t *testing.T) {
				tmpDir := emptyTmpDir(t)
				defer os.RemoveAll(tmpDir)

				// Create source directory with a symlink
				srcDir := path.Join(tmpDir, "src")
				err := os.MkdirAll(srcDir, 0o755)
				require.NoError(t, err, "failed to create source directory")

				err = os.WriteFile(path.Join(srcDir, "test.txt"), []byte("test content"), 0o644)
				require.NoError(t, err, "failed to create test file")

				err = os.Symlink(path.Join(srcDir, "test.txt"), path.Join(srcDir, "link.txt"))
				require.NoError(t, err, "failed to create symlink")

				err = os.Symlink(path.Join(srcDir, "link.txt"), path.Join(srcDir, "link2.txt"))
				require.NoError(t, err, "failed to create symlink")

				err = os.Symlink(path.Join(srcDir, "does_not_exist.txt"), path.Join(srcDir, "link3.txt"))
				require.NoError(t, err, "failed to create symlink")

				// Create destination directory
				dstDir := path.Join(tmpDir, "dst")
				err = op(srcDir, dstDir)
				require.NoError(t, err, "%s failed", name)

				// Verify the directories match
				err = CompareDirectories(srcDir, dstDir)
				require.NoError(t, err, "compareDirectories failed")
			})

			t.Run("NodeModules", func(t *testing.T) {
				stagingDir := cachedStagingDir(t)                                   // tmp/dateilager_cached
				cachedNodeModules := path.Join(stagingDir, "dl_cache/node_modules") // tmp/dateilager_cached/dl_cache/node_modules
				tmpDir := emptyTmpDir(t)                                            // tmp/test/dateilager_test_<random>
				targetNodeModules := path.Join(tmpDir, "node_modules")              // tmp/test/dateilager_test_<random>/node_modules
				defer os.RemoveAll(tmpDir)

				err := op(cachedNodeModules, targetNodeModules)
				require.NoError(t, err, "%s failed", name)

				err = CompareDirectories(cachedNodeModules, targetNodeModules)
				require.NoError(t, err, "compareDirectories %s vs %s failed", cachedNodeModules, tmpDir)
			})
		})
	}
}

func BenchmarkDirOperations(b *testing.B) {
	operations := map[string]dirOperation{
		"Hardlink": files.Hardlink,
	}

	// Only add Reflink to operations if reflinks are supported
	tmpDir := emptyTmpDir(b)
	if files.HasReflinkSupport(tmpDir) {
		operations["Reflink"] = files.Reflink
	}
	os.RemoveAll(tmpDir)

	stagingDir := cachedStagingDir(b)                                   // tmp/dateilager_cached
	cachedNodeModules := path.Join(stagingDir, "dl_cache/node_modules") // tmp/dateilager_cached/dl_cache/node_modules

	for name, op := range operations {
		b.Run(name, func(b *testing.B) {
			b.Run("Normal", func(b *testing.B) {
				tmpDir := emptyBenchDir(b) // tmp/bench/dateilager_bench_<random>
				defer os.RemoveAll(tmpDir)

				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					targetNodeModules := path.Join(tmpDir, fmt.Sprintf("app/%d/node_modules", n)) // tmp/bench/dateilager_bench_<random>/app/<n>/node_modules
					err := op(cachedNodeModules, targetNodeModules)
					b.StopTimer()
					require.NoError(b, err, "%s failed", name)

					err = CompareDirectories(cachedNodeModules, targetNodeModules)
					require.NoError(b, err, "compareDirectories %s vs %s failed", cachedNodeModules, tmpDir)
					b.StartTimer()
				}
			})

			b.Run("Overlay", func(b *testing.B) {
				if os.Getenv("DL_OVERLAY_BENCH") != "true" {
					b.Skip("DL_OVERLAY_BENCH is not set")
				}

				tmpDir := emptyBenchDir(b)                               // tmp/bench/dateilager_bench_<random>
				upperDir := path.Join(tmpDir, "upper")                   // tmp/bench/dateilager_bench_<random>/upper
				workDir := path.Join(tmpDir, "work")                     // tmp/bench/dateilager_bench_<random>/work
				targetPath := path.Join(tmpDir, "gadget")                // tmp/bench/dateilager_bench_<random>/gadget
				cacheDir := path.Join(targetPath, "dl_cache")            // tmp/bench/dateilager_bench_<random>/gadget/dl_cache
				cachedNodeModules := path.Join(cacheDir, "node_modules") // tmp/bench/dateilager_bench_<random>/gadget/dl_cache/node_modules
				defer os.RemoveAll(tmpDir)

				mkdirAll(b, upperDir, 0o777)
				defer os.RemoveAll(upperDir)

				mkdirAll(b, workDir, 0o777)
				defer os.RemoveAll(workDir)

				err := os.MkdirAll(targetPath, 0o777)
				require.NoError(b, err, "failed to create target path")
				defer os.RemoveAll(targetPath)

				err = exec.Command("sudo", "mount", "-t", "overlay", "overlay", "-n", "--options", fmt.Sprintf("redirect_dir=on,volatile,lowerdir=%s,upperdir=%s,workdir=%s", stagingDir, upperDir, workDir), targetPath).Run()
				require.NoError(b, err, "mount failed")

				defer func() {
					err := exec.Command("sudo", "umount", targetPath).Run()
					require.NoError(b, err, "umount failed")
				}()

				mkdirAll(b, cacheDir, 0o755)

				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					targetNodeModules := path.Join(targetPath, fmt.Sprintf("app/%d/node_modules", n)) // tmp/bench/dateilager_bench_<random>/gadget/app/<n>/node_modules
					err := op(cachedNodeModules, targetNodeModules)
					b.StopTimer()
					require.NoError(b, err, "%s failed", name)

					err = CompareDirectories(cachedNodeModules, targetNodeModules)
					require.NoError(b, err, "compareDirectories %s vs %s failed", cachedNodeModules, tmpDir)
					b.StartTimer()
				}
			})

			b.Run("LVM", func(b *testing.B) {
				if os.Getenv("DL_LVM_BENCH") != "true" {
					b.Skip("DL_LVM_BENCH is not set")
				}

				device := os.Getenv("DL_LVM_DEVICE")
				if device == "" {
					b.Skip("DL_LVM_DEVICE is not set")
				}

				size := os.Getenv("DL_LVM_SIZE")
				if size == "" {
					b.Skip("DL_LVM_SIZE is not set")
				}

				snapshotSize := os.Getenv("DL_LVM_SNAPSHOT_SIZE")
				if snapshotSize == "" {
					b.Skip("DL_LVM_SNAPSHOT_SIZE is not set")
				}

				format := os.Getenv("DL_LVM_FORMAT")
				if format == "" {
					format = "ext4"
				}

				execCommand(b, "sudo", "pvcreate", device)
				defer execCommand(b, "sudo", "pvremove", device)

				execCommand(b, "sudo", "vgcreate", "vg_dateilager_cached", device)
				defer execCommand(b, "sudo", "vgremove", "-y", "vg_dateilager_cached")

				execCommand(b, "sudo", "lvcreate", "--name", "thinpool", "--size", size, "--type", "thin-pool", "vg_dateilager_cached")
				execCommand(b, "sudo", "lvcreate", "--name", "base", "--virtualsize", size, "--thinpool", "vg_dateilager_cached/thinpool")
				execCommand(b, "sudo", "mkfs."+format, "/dev/vg_dateilager_cached/base")

				func() {
					execCommand(b, "sudo", "mkdir", "-p", "/mnt/dateilager_cached_base")
					defer execCommand(b, "sudo", "rmdir", "/mnt/dateilager_cached_base")

					execCommand(b, "sudo", "mount", "/dev/vg_dateilager_cached/base", "/mnt/dateilager_cached_base")
					defer execCommand(b, "sudo", "umount", "/mnt/dateilager_cached_base")

					execCommand(b, "sudo", "cp", "-a", stagingDir+"/.", "/mnt/dateilager_cached_base")
				}()

				execCommand(b, "sudo", "lvcreate", "--snapshot", "--name", "pod-0", "--size", snapshotSize, "vg_dateilager_cached/base")

				tmpDir := emptyBenchDir(b)                               // tmp/bench/dateilager_bench_<random>
				targetDir := path.Join(tmpDir, "gadget")                 // tmp/bench/dateilager_bench_<random>/gadget
				cacheDir := path.Join(targetDir, "dl_cache")             // tmp/bench/dateilager_bench_<random>/gadget/dl_cache
				cachedNodeModules := path.Join(cacheDir, "node_modules") // tmp/bench/dateilager_bench_<random>/gadget/dl_cache/node_modules
				defer os.RemoveAll(tmpDir)

				err := os.MkdirAll(targetDir, 0o777)
				require.NoError(b, err, "failed to create target path")
				defer os.RemoveAll(targetDir)

				execCommand(b, "sudo", "mount", "/dev/vg_dateilager_cached/pod-0", targetDir)
				defer execCommand(b, "sudo", "umount", targetDir)

				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					targetNodeModules := path.Join(targetDir, fmt.Sprintf("app/%d/node_modules", n)) // tmp/bench/dateilager_bench_<random>/gadget/app/<n>/node_modules
					err := op(cachedNodeModules, targetNodeModules)
					b.StopTimer()
					require.NoError(b, err, "%s failed", name)

					err = CompareDirectories(cachedNodeModules, targetNodeModules)
					require.NoError(b, err, "compareDirectories %s vs %s failed", cachedNodeModules, targetDir)
					b.StartTimer()
				}
			})
		})
	}
}
