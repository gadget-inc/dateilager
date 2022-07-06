package pb

import (
	"archive/tar"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func tarTypeFromMode(mode fs.FileMode) byte {
	if mode.IsDir() {
		return tar.TypeDir
	}

	if mode&fs.ModeSymlink == fs.ModeSymlink {
		return tar.TypeSymlink
	}

	return tar.TypeReg
}

func ObjectFromFilePath(directory, path string) (*Object, error) {
	fullPath := filepath.Join(directory, path)

	info, err := os.Lstat(fullPath)
	// If the file has been deleted since the diffs were generated,
	// send a deleted object update instead of trying to read it from disk.
	if errors.Is(err, fs.ErrNotExist) {
		return &Object{
			Path:    path,
			Deleted: true,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	var content []byte
	tarType := tarTypeFromMode(info.Mode())

	switch tarType {
	case tar.TypeReg:
		content, err = os.ReadFile(fullPath)
		if err != nil {
			return nil, err
		}
	case tar.TypeDir:
		content = []byte("")
	case tar.TypeSymlink:
		target, err := os.Readlink(fullPath)
		if err != nil {
			return nil, err
		}
		if target == "" {
			return nil, fmt.Errorf("empty link target for %v", fullPath)
		}
		content = []byte(target)
	}

	return &Object{
		Path:    path,
		Mode:    int64(info.Mode()),
		Size:    int64(len(content)),
		Deleted: false,
		Content: content,
	}, nil
}

func ObjectFromTarHeader(header *tar.Header, content []byte) *Object {
	mode := header.Mode
	size := header.Size

	switch header.Typeflag {
	case tar.TypeDir:
		mode |= int64(fs.ModeDir)
		size = 0
	case tar.TypeSymlink:
		mode |= int64(fs.ModeSymlink)
		content = []byte(header.Linkname)
		size = int64(len(content))
	}

	return &Object{
		Path:    header.Name,
		Mode:    mode,
		Size:    size,
		Deleted: false,
		Content: content,
	}
}

func (o *Object) FileMode() fs.FileMode {
	return fs.FileMode(o.Mode)
}

func (o *Object) TarType() byte {
	if o.Deleted {
		// A custom DateiLager typeflag to represent deleted objects
		return 'D'
	}

	return tarTypeFromMode(o.FileMode())
}
