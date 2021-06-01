package client

import (
	"os"

	"github.com/angelini/dateilager/pkg/pb"
)

func readFileObject(path string) (*pb.Object, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	var bytes []byte
	_, err = file.Read(bytes)
	if err != nil {
		return nil, err
	}

	return &pb.Object{
		Path: path,
		Mode: int32(info.Mode()),
		Size: int32(info.Size()),
	}, nil
}
