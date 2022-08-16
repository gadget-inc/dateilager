package files

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gadget-inc/dateilager/internal/db"
)

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func removePathIfSymlink(path string) error {
	stat, err := os.Lstat(path)
	if err != nil {
		return nil
	}

	if stat.Mode()&os.ModeSymlink == os.ModeSymlink {
		err = os.Remove(path)
		return err
	}

	return nil
}

func removePathIfNotDirectory(path string) error {
	stat, err := os.Lstat(path)
	if err != nil {
		return nil
	}

	if stat.Mode()&os.ModeDir != os.ModeDir {
		err = os.Remove(path)
		return err
	}

	return nil
}

func writeObject(rootDir string, reader *db.TarReader, header *tar.Header) error {
	path := filepath.Join(rootDir, header.Name)

	switch header.Typeflag {
	case tar.TypeReg:
		err := os.MkdirAll(filepath.Dir(path), 0777)
		if err != nil {
			return fmt.Errorf("mkdir -p %v: %w", filepath.Dir(path), err)
		}

		// os.OpenFile returns an error if it's called on a symlink
		// remove any potential symlinks before writing the regular file
		err = removePathIfSymlink(path)
		if err != nil {
			return fmt.Errorf("remove path if symlink %v: %w", path, err)
		}

		file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
		if err != nil {
			return fmt.Errorf("open file %v: %w", path, err)
		}

		err = reader.CopyContent(file)
		file.Close()
		if err != nil {
			return fmt.Errorf("write %v to disk: %w", path, err)
		}

	case tar.TypeDir:
		err := removePathIfNotDirectory(path)
		if err != nil {
			return fmt.Errorf("remove path if not dir %v: %w", path, err)
		}

		err = os.MkdirAll(path, os.FileMode(header.Mode))
		if err != nil {
			return fmt.Errorf("mkdir -p %v: %w", path, err)
		}

	case tar.TypeSymlink:
		err := os.MkdirAll(filepath.Dir(path), 0755)
		if err != nil {
			return fmt.Errorf("mkdir -p %v: %w", filepath.Dir(path), err)
		}

		// Remove existing link
		if _, err = os.Lstat(path); err == nil {
			err = os.Remove(path)
			if err != nil {
				return fmt.Errorf("rm %v before symlinking %v: %w", path, header.Linkname, err)
			}
		}

		err = os.Symlink(header.Linkname, path)
		if err != nil {
			return fmt.Errorf("ln -s %v %v: %w", header.Linkname, path, err)
		}

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

func WriteTar(finalDir string, reader *db.TarReader, packPath *string) (uint32, error) {
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

		err = writeObject(dir, reader, header)
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
