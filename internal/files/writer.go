package files

import (
	"archive/tar"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/charlievieth/fastwalk"
	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/gobwas/glob"
)

type FileMatcher struct {
	include *glob.Glob
	exclude *glob.Glob
}

func NewFileMatcher(include, exclude string) (*FileMatcher, error) {
	matcher := FileMatcher{}

	if include != "" {
		includeGlob, err := glob.Compile(include)
		if err != nil {
			return nil, fmt.Errorf("error parsing include file match: %w", err)
		}
		matcher.include = &includeGlob
	}

	if exclude != "" {
		excludeGlob, err := glob.Compile(exclude)
		if err != nil {
			return nil, fmt.Errorf("error parsing exclude file match: %w", err)
		}
		matcher.exclude = &excludeGlob
	}

	return &matcher, nil
}

func (f *FileMatcher) Match(filename string) bool {
	result := false

	if f.include != nil {
		result = (*f.include).Match(filename)
	} else {
		result = true
	}

	if result && f.exclude != nil {
		result = !(*f.exclude).Match(filename)
	}

	return result
}

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

type retryableFileOperation[R any] func() (R, error)

// if we encounter an error making a directory, symlink, or opening a file, we try to recover by just removing whatever is currently at the path, and trying again
func retryFileErrors[R any](path string, fn retryableFileOperation[R]) (R, error) {
	result, err := fn()
	if err != nil {
		err = os.RemoveAll(path)
		if err != nil {
			return result, fmt.Errorf("removing existing path error %v: %w", path, err)
		}
		return fn()
	}
	return result, err
}

func writeObject(rootDir string, cacheObjectsDir string, reader *db.TarReader, header *tar.Header, existingDirs map[string]bool, hasReflinkSupport bool) (bool, error) {
	path := filepath.Join(rootDir, header.Name)

	switch header.Typeflag {
	case pb.TarCached:
		content, err := reader.ReadContent()
		if err != nil {
			return false, err
		}
		hashHex := hex.EncodeToString(content)
		if hasReflinkSupport {
			return true, Reflink(filepath.Join(cacheObjectsDir, hashHex, header.Name), path)
		} else {
			return true, Hardlink(filepath.Join(cacheObjectsDir, hashHex, header.Name), path)
		}

	case tar.TypeReg:
		dir := filepath.Dir(path)
		createdDir := false

		if _, exists := existingDirs[dir]; !exists {
			_, err := retryFileErrors(dir, func() (interface{}, error) {
				createdDir = true
				return nil, os.MkdirAll(dir, 0o777)
			})
			if err != nil {
				return false, fmt.Errorf("mkdir -p %v: %w", dir, err)
			}
			existingDirs[dir] = true
		}

		file, err := retryFileErrors(path, func() (*os.File, error) {
			return os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
		})
		if err != nil {
			return false, fmt.Errorf("open file %v: %w", path, err)
		}

		err = PreAllocate(file, header.Size)
		if err != nil {
			return false, fmt.Errorf("failed to pre allocate %v: %w", path, err)
		}
		err = reader.CopyContent(file)
		if err != nil {
			return false, fmt.Errorf("write %v to disk: %w", path, err)
		}

		// If we created the dir while writing this file, we know it is a new file with the correct mode
		if !createdDir {
			info, err := file.Stat()
			if err != nil {
				return false, fmt.Errorf("stat %v: %w", path, err)
			}

			if info.Mode() != os.FileMode(header.Mode) {
				err = file.Chmod(os.FileMode(header.Mode))
				if err != nil {
					return false, fmt.Errorf("chmod %v on disk: %w", path, err)
				}
			}
		}

		file.Close()

	case tar.TypeDir:
		if _, exists := existingDirs[path]; !exists {
			_, err := retryFileErrors(path, func() (interface{}, error) {
				return nil, os.MkdirAll(path, 0o777)
			})
			if err != nil {
				return false, fmt.Errorf("mkdir -p %v: %w", path, err)
			}
			existingDirs[path] = true
		}

	case tar.TypeSymlink:
		return false, makeSymlink(header.Linkname, path)

	case 'D':
		err := os.Remove(path)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		// Temp: account for a handful of projects that have invalid deleted empty directory rows
		if errors.Is(err, fs.ErrExist) {
			break
		}
		if err != nil {
			return false, fmt.Errorf("remove %v from disk: %w", path, err)
		}

	default:
		return false, fmt.Errorf("unhandle TAR type: %v", header.Typeflag)
	}

	return false, nil
}

func makeSymlink(oldname, newname string) error {
	dir := filepath.Dir(newname)
	_, err := retryFileErrors(dir, func() (interface{}, error) {
		return nil, os.MkdirAll(dir, 0o777)
	})
	if err != nil {
		return fmt.Errorf("mkdir -p %v: %w", dir, err)
	}

	_, err = retryFileErrors(newname, func() (interface{}, error) {
		return nil, os.Symlink(oldname, newname)
	})
	if err != nil {
		return fmt.Errorf("ln -s %v %v: %w", oldname, newname, err)
	}

	return nil
}

func Hardlink(oldpath, newpath string) error {
	if fileExists(newpath) {
		err := os.RemoveAll(newpath)
		if err != nil {
			return fmt.Errorf("cannot remove existing path %v: %w", newpath, err)
		}
	}

	info, err := os.Lstat(oldpath)
	if err != nil {
		return fmt.Errorf("cannot stat oldpath %v: %w", oldpath, err)
	}

	if !info.IsDir() {
		return hardlink(info, oldpath, newpath)
	}

	err = os.MkdirAll(newpath, info.Mode())
	if err != nil {
		return fmt.Errorf("cannot create root newpath %v: %w", newpath, err)
	}

	fastwalkConf := fastwalk.DefaultConfig.Copy()
	fastwalkConf.Sort = fastwalk.SortDirsFirst

	return fastwalk.Walk(fastwalkConf, oldpath, func(oldsubpath string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("failed to walk dir %v: %w", oldsubpath, err)
		}

		newsubpath := filepath.Join(newpath, strings.TrimPrefix(oldsubpath, oldpath))

		// The new "root" already exists so don't recreate it
		if newsubpath == newpath {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("unable to get info %v: %w", oldsubpath, err)
		}

		return hardlink(info, oldsubpath, newsubpath)
	})
}

func hardlink(info os.FileInfo, oldpath, newpath string) error {
	switch {
	case info.IsDir():
		// Can't hardlink directories, so recreate it
		err := os.Mkdir(newpath, info.Mode())
		if err != nil {
			return fmt.Errorf("mkdir %v: %w", newpath, err)
		}
		return nil
	case info.Mode()&os.ModeSymlink != 0:
		// Can't hardlink symlinks, so recreate it
		target, err := os.Readlink(oldpath)
		if err != nil {
			return fmt.Errorf("readlink %v: %w", oldpath, err)
		}
		err = os.Symlink(target, newpath)
		if err != nil {
			return fmt.Errorf("symlink %v %v: %w", target, newpath, err)
		}
		return nil
	case info.Mode().IsRegular():
		// Hardlink the file
		err := os.Link(oldpath, newpath)
		if err != nil {
			return fmt.Errorf("ln %v %v: %w", oldpath, newpath, err)
		}
		return nil
	default:
		// Not a directory, symlink, or regular file, so just copy it
		return copyFile(oldpath, newpath, info.Mode())
	}
}

func Reflink(oldpath, newpath string) error {
	if fileExists(newpath) {
		err := os.RemoveAll(newpath)
		if err != nil {
			return fmt.Errorf("cannot remove existing path %v: %w", newpath, err)
		}
	}

	info, err := os.Lstat(oldpath)
	if err != nil {
		return fmt.Errorf("cannot stat oldpath %v: %w", oldpath, err)
	}

	if !info.IsDir() {
		return reflink(info, oldpath, newpath)
	}

	err = os.MkdirAll(newpath, info.Mode())
	if err != nil {
		return fmt.Errorf("cannot create root newpath %v: %w", newpath, err)
	}

	fastwalkConf := fastwalk.DefaultConfig.Copy()
	fastwalkConf.Sort = fastwalk.SortDirsFirst

	return fastwalk.Walk(fastwalkConf, oldpath, func(oldsubpath string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("failed to walk dir %v: %w", oldsubpath, err)
		}

		newsubpath := filepath.Join(newpath, strings.TrimPrefix(oldsubpath, oldpath))

		// The new "root" already exists so don't recreate it
		if newsubpath == newpath {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("unable to get info %v: %w", oldsubpath, err)
		}

		return reflink(info, oldsubpath, newsubpath)
	})
}

func reflink(info os.FileInfo, oldpath, newpath string) error {
	switch {
	case info.IsDir():
		// Can't reflink directories, so recreate it
		err := os.Mkdir(newpath, info.Mode())
		if err != nil {
			return fmt.Errorf("mkdir %v: %w", newpath, err)
		}
		return nil
	case info.Mode()&os.ModeSymlink != 0:
		// Can't reflink symlinks, so recreate it
		target, err := os.Readlink(oldpath)
		if err != nil {
			return fmt.Errorf("readlink %v: %w", oldpath, err)
		}
		err = os.Symlink(target, newpath)
		if err != nil {
			return fmt.Errorf("symlink %v %v: %w", target, newpath, err)
		}
		return nil
	case info.Mode().IsRegular():
		// Reflink the file
		err := reflinkFile(oldpath, newpath, info.Mode())
		if err != nil {
			return fmt.Errorf("reflink %v %v: %w", oldpath, newpath, err)
		}
		return nil
	default:
		// Not a directory, symlink, or regular file, so just copy it
		return copyFile(oldpath, newpath, info.Mode())
	}
}

// copyFile copies a file from src to dst, preserving metadata
func copyFile(src, dst string, perm fs.FileMode) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file %v: %w", src, err)
	}
	defer sourceFile.Close()

	destFile, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("open destination file %v: %w", dst, err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("copy %v to %v: %w", src, dst, err)
	}
	return nil
}

func WriteTar(finalDir string, cacheObjectsDir string, reader *db.TarReader, packPath *string, matcher *FileMatcher, hasReflinkSupport bool) (uint32, uint32, bool, error) {
	var count uint32
	var cachedCount uint32
	dir := finalDir

	fileMatch := true

	existingDirs := make(map[string]bool)

	if packPath != nil && fileExists(filepath.Join(finalDir, *packPath)) {
		tmpDir, err := os.MkdirTemp(filepath.Join(finalDir, ".dl"), "dateilager_pack_path_")
		if err != nil {
			return count, cachedCount, false, fmt.Errorf("cannot create tmp dir for packed tar: %w", err)
		}
		defer os.RemoveAll(tmpDir)
		dir = tmpDir
	}

	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, cachedCount, false, fmt.Errorf("read next TAR header: %w", err)
		}

		if matcher != nil && !matcher.Match(header.Name) {
			fileMatch = false
		}

		cached, err := writeObject(dir, cacheObjectsDir, reader, header, existingDirs, hasReflinkSupport)
		if err != nil {
			return count, cachedCount, false, err
		}

		count += 1
		if cached {
			cachedCount += 1
		}
	}

	if packPath != nil && dir != finalDir {
		path := filepath.Join(finalDir, *packPath)
		err := os.RemoveAll(path)
		if err != nil {
			return count, cachedCount, false, fmt.Errorf("cannot remove existing packed path %v: %w", path, err)
		}

		if fileExists(filepath.Join(dir, *packPath)) {
			err = os.Rename(filepath.Join(dir, *packPath), path)
			if err != nil {
				return count, cachedCount, false, fmt.Errorf("cannot rename packed path %v to %v: %w", filepath.Join(dir, *packPath), path, err)
			}
		}
	}

	return count, cachedCount, fileMatch, nil
}

// HasReflinkSupport checks if the given directory supports reflinks.
// It attempts to create a reflink in the directory and returns true if successful.
func HasReflinkSupport(dir string) bool {
	useReflinks := os.Getenv("DL_USE_REFLINKS")
	if useReflinks == "always" {
		return true
	}
	if useReflinks != "1" && useReflinks != "true" {
		return false
	}

	srcFile := filepath.Join(dir, "reflink_test_src")
	dstFile := filepath.Join(dir, "reflink_test_dst")
	defer os.Remove(srcFile)
	defer os.Remove(dstFile)

	// Create a test file
	err := os.WriteFile(srcFile, []byte("test"), 0o644)
	if err != nil {
		return false
	}

	// Try to create a reflink
	err = reflinkFile(srcFile, dstFile, 0o644)
	if err != nil {
		return false
	}

	// Ensure the files are the same
	srcData, err := os.ReadFile(srcFile)
	if err != nil {
		return false
	}
	dstData, err := os.ReadFile(dstFile)
	if err != nil {
		return false
	}

	return bytes.Equal(srcData, dstData)
}
