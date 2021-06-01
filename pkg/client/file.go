package client

import (
	"io/ioutil"
	"os"
	"strings"

	"github.com/angelini/dateilager/internal/pb"
)

func readFileObject(path, prefix string) (*pb.Object, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return &pb.Object{
		Path:     strings.TrimPrefix(path, prefix),
		Mode:     int32(info.Mode()),
		Size:     int32(info.Size()),
		Contents: bytes,
	}, nil
}
