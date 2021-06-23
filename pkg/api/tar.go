package api

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/angelini/dateilager/internal/pb"
)

var (
	ErrEmptyPack = errors.New("empty object stream to pack")
)

type TarWriter struct {
	buffer *bytes.Buffer
	writer *tar.Writer
}

func NewTarWriter() *TarWriter {
	var buffer bytes.Buffer
	return &TarWriter{
		buffer: &buffer,
		writer: tar.NewWriter(&buffer),
	}
}

func (t *TarWriter) BytesAndReset() ([]byte, error) {
	err := t.writer.Close()
	if err != nil {
		return nil, fmt.Errorf("close TarWriter: %w", err)
	}

	output := t.buffer.Bytes()
	t.buffer.Truncate(0)
	t.writer = tar.NewWriter(t.buffer)

	return output, nil
}

func (t *TarWriter) Len() int {
	return t.buffer.Len()
}

func (t *TarWriter) WriteObject(object *pb.Object, writeContent bool) error {
	typeFlag := tar.TypeReg
	if object.Deleted {
		// Custom dateilager type flag to represent deleted files
		typeFlag = 'D'
	}

	size := int64(object.Size)
	if !writeContent {
		size = 0
	}

	header := &tar.Header{
		Name:     object.Path,
		Mode:     int64(object.Mode),
		Size:     size,
		Format:   tar.FormatPAX,
		Typeflag: byte(typeFlag),
	}

	err := t.writer.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("write header to TAR %v: %w", object.Path, err)
	}

	if writeContent {
		_, err = t.writer.Write(object.Content)
		if err != nil {
			return fmt.Errorf("write content to TAR %v: %w", object.Path, err)
		}
	}

	return nil
}

func packObjects(objects objectStream) ([]byte, []byte, error) {
	contentWriter := NewTarWriter()
	namesWriter := NewTarWriter()
	empty := true

	for {
		object, err := objects()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}

		empty = false

		err = contentWriter.WriteObject(object, true)
		if err != nil {
			return nil, nil, err
		}

		err = namesWriter.WriteObject(object, false)
		if err != nil {
			return nil, nil, err
		}
	}

	if empty {
		return nil, nil, ErrEmptyPack
	}

	contentTar, err := contentWriter.BytesAndReset()
	if err != nil {
		return nil, nil, err
	}

	namesTar, err := namesWriter.BytesAndReset()
	if err != nil {
		return nil, nil, err
	}

	return contentTar, namesTar, nil
}
