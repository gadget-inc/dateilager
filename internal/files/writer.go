package files

import (
	"archive/tar"
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

func writeObject(rootDir string, cacheObjectsDir string, reader *db.TarReader, header *tar.Header, existingDirs map[string]bool) (bool, error) {
	path := filepath.Join(rootDir, header.Name)

	switch header.Typeflag {
	case pb.TarCached:
		content, err := reader.ReadContent()
		if err != nil {
			return false, err
		}
		hashHex := hex.EncodeToString(content)
		return true, Hardlink(filepath.Join(cacheObjectsDir, hashHex, header.Name), path)

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
		// Can't hardlink directories, so just create it
		err := os.Mkdir(newpath, info.Mode())
		if err != nil {
			return fmt.Errorf("mkdir %v: %w", newpath, err)
		}
		return nil
	case info.Mode()&fs.ModeSymlink != 0:
		// Can't hardlink symlinks that point to non-existent files, so recreate the symlink pointing to the same target
		target, err := os.Readlink(oldpath)
		if err != nil {
			return fmt.Errorf("readlink %v: %w", oldpath, err)
		}
		err = os.Symlink(target, newpath)
		if err != nil {
			return fmt.Errorf("ln -s %v %v: %w", oldpath, newpath, err)
		}
		return nil
	default:
		// Hardlink the file
		err := os.Link(oldpath, newpath)
		if err != nil {
			return fmt.Errorf("ln %v %v: %w", oldpath, newpath, err)
		}
		return nil
	}
}

func WriteTar(finalDir string, cacheObjectsDir string, reader *db.TarReader, packPath *string, matcher *FileMatcher) (uint32, uint32, bool, error) {
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

		cached, err := writeObject(dir, cacheObjectsDir, reader, header, existingDirs)
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
