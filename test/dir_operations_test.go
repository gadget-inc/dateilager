package test

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/gadget-inc/dateilager/internal/files"
	"github.com/gadget-inc/dateilager/pkg/cached"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"k8s.io/utils/mount"
)

var (
	mounter           = mount.New("")
	randomFileContent = make([]byte, 1024*64)
)

type dirOperation func(src, dst string) error

func init() {
	_, err := rand.Read(randomFileContent)
	if err != nil {
		panic(err)
	}
}

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
				preparedDir := preparedCachedDir(t)                                  // tmp/dateilager_cached
				cachedNodeModules := path.Join(preparedDir, "dl_cache/node_modules") // tmp/dateilager_cached/dl_cache/node_modules
				tmpDir := emptyTmpDir(t)                                             // tmp/test/dateilager_test_<random>
				targetNodeModules := path.Join(tmpDir, "node_modules")               // tmp/test/dateilager_test_<random>/node_modules
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

	preparedDir := preparedCachedDir(b)                                  // tmp/dateilager_cached
	cachedNodeModules := path.Join(preparedDir, "dl_cache/node_modules") // tmp/dateilager_cached/dl_cache/node_modules

	for name, op := range operations {
		b.Run(name, func(b *testing.B) {
			b.Run("Normal", func(b *testing.B) {
				tmpDir := emptyBenchDir(b) // tmp/bench/dateilager_bench_<random>
				defer os.RemoveAll(tmpDir)

				b.ResetTimer()
				for n := 0; b.Loop(); n++ {
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

				execRun(b, "mount", "-t", "overlay", "overlay", "-n", "--options", fmt.Sprintf("redirect_dir=on,volatile,lowerdir=%s,upperdir=%s,workdir=%s", preparedDir, upperDir, workDir), targetPath)
				defer execRun(b, "umount", targetPath)

				mkdirAll(b, cacheDir, 0o755)

				b.ResetTimer()
				for n := 0; b.Loop(); n++ {
					targetNodeModules := path.Join(targetPath, fmt.Sprintf("app/%d/node_modules", n)) // tmp/bench/dateilager_bench_<random>/gadget/app/<n>/node_modules
					err := op(cachedNodeModules, targetNodeModules)
					b.StopTimer()
					require.NoError(b, err, "%s failed", name)

					err = CompareDirectories(cachedNodeModules, targetNodeModules)
					require.NoError(b, err, "compareDirectories %s vs %s failed", cachedNodeModules, tmpDir)
					b.StartTimer()
				}
			})

			for _, baseLVFormat := range []string{cached.XFS, cached.EXT4} {
				b.Run(baseLVFormat, func(b *testing.B) {
					if baseLVFormat == cached.EXT4 && name == "Reflink" {
						b.Skip("ext4 doesn't support reflinks")
					}

					b.Run("lvm", func(b *testing.B) {
						basePV := os.Getenv("DL_BASE_PV")
						if basePV == "" {
							b.Skip("DL_BASE_PV is not set")
						}

						thinpoolPVGlobs := os.Getenv("DL_THINPOOL_PV_GLOBS")
						if thinpoolPVGlobs == "" {
							b.Skip("DL_THINPOOL_PV_GLOBS is not set")
						}

						var thinpoolPVs []string
						for pv := range strings.SplitSeq(thinpoolPVGlobs, ",") {
							devices, err := filepath.Glob(pv)
							require.NoError(b, err)
							thinpoolPVs = append(thinpoolPVs, devices...)
						}

						ensurePV(b, basePV)
						defer removePV(b, basePV)

						vg := "vg_dl_cached_bench"
						baseLV := vg + "/base"
						baseLVDevice := "/dev/" + baseLV
						readThroughBasePV := os.Getenv("DL_READ_THROUGH_BASE_PV")
						thinpoolLV := vg + "/thinpool"
						thinpoolCacheLVSize := os.Getenv("DL_THINPOOL_CACHE_LV_SIZE_KIB")
						thinpoolCachePV := "/dev/ram0"

						if thinpoolCacheLVSize != "" {
							defer removePV(b, thinpoolCachePV)
							defer execRun(b, "modprobe", "-r", "brd")
						}

						ensureVG(b, vg, basePV)
						defer removeVG(b, vg)

						ensureLV(b, baseLV, cached.LVCreateBaseArgs(vg, basePV)...)
						defer removeLV(b, baseLV)

						formatOptions := cached.FormatOptions(baseLVDevice, baseLVFormat)
						mountOptions := cached.MountOptions(baseLVFormat)

						execRun(b, "mkfs."+baseLVFormat, formatOptions...)

						tmpDir := emptyBenchDir(b) // tmp/bench/dateilager_bench_<random>
						defer os.RemoveAll(tmpDir)

						func() {
							baseLVMountPoint := path.Join(tmpDir, "mnt", baseLV)
							require.NoError(b, os.MkdirAll(baseLVMountPoint, 0o775))
							defer os.RemoveAll(baseLVMountPoint)

							require.NoError(b, mounter.Mount(baseLVDevice, baseLVMountPoint, baseLVFormat, mountOptions))
							execRun(b, "cp", "-a", preparedDir+"/.", baseLVMountPoint)

							require.NoError(b, mounter.Unmount(baseLVMountPoint))
							execRun(b, "lvchange", "--permission", "r", baseLV)
							execRun(b, "lvchange", "--activate", "n", baseLV)
						}()

						if readThroughBasePV != "" {
							ensurePV(b, readThroughBasePV)
							execRun(b, "vgextend", vg, readThroughBasePV)
							execRun(b, "lvconvert", cached.LVConvertReadThroughBasePVArgs(readThroughBasePV, baseLV)...)
						}

						execRun(b, "vgextend", append([]string{"--config=devices/allow_mixed_block_sizes=1", vg}, thinpoolPVs...)...)

						ensureLV(b, thinpoolLV, cached.LVCreateThinpoolArgs(vg, thinpoolPVs)...)
						defer removeLV(b, thinpoolLV)

						if thinpoolCacheLVSize != "" {
							execRun(b, "modprobe", "brd", "rd_nr=1", "rd_size="+thinpoolCacheLVSize)
							ensurePV(b, thinpoolCachePV)
							execRun(b, "vgextend", vg, thinpoolCachePV)
							execRun(b, "lvconvert", cached.LVConvertThinpoolCacheArgs(thinpoolCachePV, thinpoolLV)...)
						}

						execRun(b, "lvchange", "--activate", "n", baseLV)

						volumeID := "csi-0" // e.g. csi-8987671b2736a86f94ac1054cfda8012690a6c3a11b6cf40d1fcb64550c44935
						lv := vg + "/" + volumeID
						lvDevice := "/dev/" + lv

						ensureLV(b, lv, cached.LVCreateThinSnapshotArgs(baseLV, thinpoolLV, volumeID)...)
						defer removeLV(b, lv)

						targetDir := path.Join(tmpDir, "gadget")                 // tmp/bench/dateilager_bench_<random>/gadget
						cacheDir := path.Join(targetDir, "dl_cache")             // tmp/bench/dateilager_bench_<random>/gadget/dl_cache
						cachedNodeModules := path.Join(cacheDir, "node_modules") // tmp/bench/dateilager_bench_<random>/gadget/dl_cache/node_modules

						require.NoError(b, os.MkdirAll(targetDir, 0o775))
						defer os.RemoveAll(targetDir)

						require.NoError(b, mounter.Mount(lvDevice, targetDir, baseLVFormat, mountOptions))
						defer func() {
							require.NoError(b, mounter.Unmount(targetDir))
						}()

						b.ResetTimer()
						for n := 0; b.Loop(); n++ {
							nodeModulesDir := path.Join(targetDir, fmt.Sprintf("app/%d/node_modules", n)) // tmp/bench/dateilager_bench_<random>/gadget/app/<n>/node_modules
							srcDir := path.Join(targetDir, fmt.Sprintf("app/%d/src", n))                  // tmp/bench/dateilager_bench_<random>/gadget/app/<n>/src

							err := op(cachedNodeModules, nodeModulesDir)
							err = errors.Join(err, writeFilesOfVaryingSizes(b, srcDir, 1000))

							b.StopTimer()
							require.NoError(b, err)

							err = CompareDirectories(cachedNodeModules, nodeModulesDir)
							require.NoError(b, err, "compareDirectories %s vs %s failed", cachedNodeModules, targetDir)
							b.StartTimer()
						}
					})
				})
			}
		})
	}
}

func writeFilesOfVaryingSizes(t testing.TB, dir string, count int) error {
	err := os.MkdirAll(dir, 0o775)
	if err != nil {
		return err
	}

	variation := min(50, count)
	minFileSize := 64
	maxFileSize := len(randomFileContent)

	g, ctx := errgroup.WithContext(t.Context())
	workers := parallelWorkerCount()
	jobs := make(chan int, workers)

	for range workers {
		g.Go(func() error {
			for i := range jobs {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				name := "file" + strconv.Itoa(i)
				size := minFileSize + (i%variation)*(maxFileSize/variation)
				start := (i * variation) % maxFileSize
				end := min(start+size, maxFileSize)
				if err := os.WriteFile(path.Join(dir, name), randomFileContent[start:end], 0o644); err != nil {
					return err
				}
			}
			return nil
		})
	}

	for i := range count {
		jobs <- i
	}
	close(jobs)

	return g.Wait()
}

func parallelWorkerCount() int {
	envCount := os.Getenv("DL_WRITE_WORKERS")
	if envCount != "" {
		count, err := strconv.Atoi(envCount)
		if err == nil {
			return count
		}
	}

	return min(max(runtime.NumCPU()/2, 2), 8)
}
