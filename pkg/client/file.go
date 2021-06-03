package client

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/angelini/dateilager/internal/pb"
)

func readFileObject(path, prefix string) (*pb.Object, bool, error) {
	fullPath := filepath.Join(prefix, path)

	file, err := os.Open(fullPath)
	if errors.Is(err, os.ErrNotExist) {
		return &pb.Object{
			Path:     path,
			Mode:     0,
			Size:     0,
			Contents: nil,
		}, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}

	bytes, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return nil, false, err
	}

	return &pb.Object{
		Path:     path,
		Mode:     int32(info.Mode()),
		Size:     int32(info.Size()),
		Contents: bytes,
	}, false, nil
}
