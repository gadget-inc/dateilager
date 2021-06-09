package client

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/angelini/dateilager/internal/pb"
)

func readFileObject(directory, path string) (*pb.Object, error) {
	fullPath := filepath.Join(directory, path)

	file, err := os.Open(fullPath)
	if errors.Is(err, os.ErrNotExist) {
		return &pb.Object{
			Path:     path,
			Mode:     0,
			Size:     0,
			Deleted:  true,
			Contents: nil,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	bytes, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}

	return &pb.Object{
		Path:     path,
		Mode:     int32(info.Mode()),
		Size:     int32(info.Size()),
		Deleted:  false,
		Contents: bytes,
	}, nil
}
