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

func writeObject(rootDir string, cacheObjectsDir string, reader *db.TarReader, header *tar.Header, existingDirs map[string]bool) error {
	path := filepath.Join(rootDir, header.Name)

	switch header.Typeflag {
	case pb.TarCached:
		content, err := reader.ReadContent()
		if err != nil {
			return err
		}
		hashHex := hex.EncodeToString(content)
		return HardlinkDir(filepath.Join(cacheObjectsDir, hashHex, header.Name), path)

	case tar.TypeReg:
		dir := filepath.Dir(path)
		createdDir := false

		if _, exists := existingDirs[dir]; !exists {
			_, err := retryFileErrors(dir, func() (interface{}, error) {
				createdDir = true
				return nil, os.MkdirAll(dir, 0777)
			})
			if err != nil {
				return fmt.Errorf("mkdir -p %v: %w", dir, err)
			}
			existingDirs[dir] = true
		}

		file, err := retryFileErrors(path, func() (*os.File, error) {
			return os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
		})
		if err != nil {
			return fmt.Errorf("open file %v: %w", path, err)
		}

		err = PreAllocate(file, header.Size)
		if err != nil {
			return fmt.Errorf("failed to pre allocate %v: %w", path, err)
		}
		err = reader.CopyContent(file)
		if err != nil {
			return fmt.Errorf("write %v to disk: %w", path, err)
		}

		// If we created the dir while writing this file, we know it is a new file with the correct mode
		if !createdDir {
			info, err := file.Stat()
			if err != nil {
				return fmt.Errorf("stat %v: %w", path, err)
			}

			if info.Mode() != os.FileMode(header.Mode) {
				err = file.Chmod(os.FileMode(header.Mode))
				if err != nil {
					return fmt.Errorf("chmod %v on disk: %w", path, err)
				}
			}
		}

		file.Close()

	case tar.TypeDir:
		if _, exists := existingDirs[path]; !exists {
			_, err := retryFileErrors(path, func() (interface{}, error) {
				return nil, os.MkdirAll(path, 0777)
			})
			if err != nil {
				return fmt.Errorf("mkdir -p %v: %w", path, err)
			}
			existingDirs[path] = true
		}

	case tar.TypeSymlink:
		return makeSymlink(header.Linkname, path)

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
			return fmt.Errorf("remove %v from disk: %w", path, err)
		}

	default:
		return fmt.Errorf("unhandle TAR type: %v", header.Typeflag)
	}

	return nil
}

func makeSymlink(oldname, newname string) error {
	dir := filepath.Dir(newname)
	_, err := retryFileErrors(dir, func() (interface{}, error) {
		return nil, os.MkdirAll(dir, 0777)
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

func HardlinkDir(olddir, newdir string) error {
	if fileExists(newdir) {
		err := os.RemoveAll(newdir)
		if err != nil {
			return fmt.Errorf("cannot remove existing cached path %v: %w", newdir, err)
		}
	}

	rootInfo, err := os.Lstat(olddir)
	if err != nil {
		return fmt.Errorf("cannot stat olddir %v: %w", olddir, err)
	}

	err = os.MkdirAll(newdir, rootInfo.Mode())
	if err != nil {
		return fmt.Errorf("cannot create new root dir %v: %w", olddir, err)
	}

	return filepath.WalkDir(olddir, func(oldpath string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("failed to walk dir: %v, %w", oldpath, err)
		}
		if oldpath == olddir {
			return nil
		}

		newpath := filepath.Join(newdir, strings.TrimPrefix(oldpath, olddir))

		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return fmt.Errorf("cannot access directory info %v: %w", oldpath, err)
			}
			err = os.Mkdir(newpath, info.Mode())
			if err != nil {
				return fmt.Errorf("cannot create dir %v: %w", newpath, err)
			}
			return nil
		}

		err = os.Link(oldpath, newpath)
		if err != nil {
			return fmt.Errorf("ln %v %v: %w", oldpath, newpath, err)
		}

		return nil
	})
}

func WriteTar(finalDir string, cacheObjectsDir string, reader *db.TarReader, packPath *string, matcher *FileMatcher) (uint32, bool, error) {
	var count uint32
	dir := finalDir

	fileMatch := true

	existingDirs := make(map[string]bool)

	if packPath != nil && fileExists(filepath.Join(finalDir, *packPath)) {
		tmpDir, err := os.MkdirTemp(filepath.Join(finalDir, ".dl"), "dateilager_pack_path_")
		if err != nil {
			return count, false, fmt.Errorf("cannot create tmp dir for packed tar: %w", err)
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
			return count, false, fmt.Errorf("read next TAR header: %w", err)
		}

		if matcher != nil && !matcher.Match(header.Name) {
			fileMatch = false
		}

		err = writeObject(dir, cacheObjectsDir, reader, header, existingDirs)
		if err != nil {
			return count, false, err
		}

		count += 1
	}

	if packPath != nil && dir != finalDir {
		path := filepath.Join(finalDir, *packPath)
		err := os.RemoveAll(path)
		if err != nil {
			return count, false, fmt.Errorf("cannot remove existing packed path %v: %w", path, err)
		}

		if fileExists(filepath.Join(dir, *packPath)) {
			err = os.Rename(filepath.Join(dir, *packPath), path)
			if err != nil {
				return count, false, fmt.Errorf("cannot rename packed path %v to %v: %w", filepath.Join(dir, *packPath), path, err)
			}
		}
	}

	return count, fileMatch, nil
}
