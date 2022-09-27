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

	"github.com/gadget-inc/dateilager/internal/pb"

	"github.com/gadget-inc/dateilager/internal/db"
)

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

func writeObject(rootDir string, cacheObjectsDir string, reader *db.TarReader, header *tar.Header) error {
	path := filepath.Join(rootDir, header.Name)

	switch header.Typeflag {
	case pb.TarCached:
		content, err := reader.ReadContent()
		if err != nil {
			return err
		}
		hashHex := hex.EncodeToString(content)
		return makeSymlink(filepath.Join(cacheObjectsDir, hashHex, header.Name), path)
	case tar.TypeReg:
		dir := filepath.Dir(path)
		_, err := retryFileErrors(dir, func() (interface{}, error) {
			return nil, os.MkdirAll(dir, 0777)
		})
		if err != nil {
			return fmt.Errorf("mkdir -p %v: %w", dir, err)
		}

		file, err := retryFileErrors(path, func() (*os.File, error) {
			return os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
		})
		if err != nil {
			return fmt.Errorf("open file %v: %w", path, err)
		}

		err = reader.CopyContent(file)
		file.Close()
		if err != nil {
			return fmt.Errorf("write %v to disk: %w", path, err)
		}
		err = os.Chmod(path, os.FileMode(header.Mode))
		if err != nil {
			return fmt.Errorf("chmod %v on disk: %w", path, err)
		}

	case tar.TypeDir:
		_, err := retryFileErrors(path, func() (interface{}, error) {
			return nil, os.MkdirAll(path, 0777)
		})

		if err != nil {
			return fmt.Errorf("mkdir -p %v: %w", path, err)
		}

	case tar.TypeSymlink:
		return makeSymlink(header.Linkname, path)
	case 'D':
		err := os.Remove(path)
		if errors.Is(err, fs.ErrNotExist) {
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

func makeSymlink(linkname string, path string) error {
	dir := filepath.Dir(path)
	_, err := retryFileErrors(dir, func() (interface{}, error) {
		return nil, os.MkdirAll(dir, 0777)
	})
	if err != nil {
		return fmt.Errorf("mkdir -p %v: %w", dir, err)
	}

	_, err = retryFileErrors(path, func() (interface{}, error) {
		return nil, os.Symlink(linkname, path)
	})
	if err != nil {
		return fmt.Errorf("ln -s %v %v: %w", linkname, path, err)
	}

	return nil
}

func WriteTar(finalDir string, cacheObjectsDir string, reader *db.TarReader, packPath *string) (uint32, error) {
	var count uint32
	dir := finalDir

	if packPath != nil && fileExists(filepath.Join(finalDir, *packPath)) {
		tmpDir, err := os.MkdirTemp("", "dateilager_pack_path_")
		if err != nil {
			return count, fmt.Errorf("cannot create tmp dir for packed tar: %w", err)
		}
		dir = tmpDir
	}

	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("read next TAR header: %w", err)
		}

		err = writeObject(dir, cacheObjectsDir, reader, header)
		if err != nil {
			return count, err
		}

		count += 1
	}

	if packPath != nil && dir != finalDir {
		path := filepath.Join(finalDir, *packPath)
		err := os.RemoveAll(path)
		if err != nil {
			return count, fmt.Errorf("cannot remove existing packed path %v: %w", path, err)
		}

		if fileExists(filepath.Join(dir, *packPath)) {
			err = os.Rename(filepath.Join(dir, *packPath), path)
			if err != nil {
				return count, fmt.Errorf("cannot rename packed path %v to %v: %w", filepath.Join(dir, *packPath), path, err)
			}
		}

		os.RemoveAll(dir)
	}

	return count, nil
}
