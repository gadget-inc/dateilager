package db

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/klauspost/compress/zstd"
)

var (
	ErrEmptyPack = errors.New("empty object stream to pack")
)

type TarWriter struct {
	size       int
	buffer     *bytes.Buffer
	zstdWriter *zstd.Encoder
	tarWriter  *tar.Writer
}

func NewTarWriter() *TarWriter {
	var buffer bytes.Buffer

	zstdWriter, err := zstd.NewWriter(&buffer)
	if err != nil {
		panic("assert not reached: invalid ZSTD writer options")
	}

	return &TarWriter{
		size:       0,
		buffer:     &buffer,
		zstdWriter: zstdWriter,
		tarWriter:  tar.NewWriter(zstdWriter),
	}
}

func (t *TarWriter) BytesAndReset() ([]byte, error) {
	err := t.tarWriter.Close()
	if err != nil {
		return nil, fmt.Errorf("close TarWriter.tarWriter: %w", err)
	}

	err = t.zstdWriter.Close()
	if err != nil {
		return nil, fmt.Errorf("close TarWriter.zstdWriter: %w", err)
	}

	output := t.buffer.Bytes()

	t.size = 0
	t.buffer.Truncate(0)
	t.zstdWriter.Reset(t.buffer)
	t.tarWriter = tar.NewWriter(t.zstdWriter)

	return output, nil
}

func (t *TarWriter) Size() int {
	return t.size
}

func (t *TarWriter) WriteObject(object *pb.Object, writeContent bool) error {
	typeFlag := tar.TypeReg
	if object.Deleted {
		// Custom dateilager type flag to represent deleted files
		typeFlag = 'D'
	}

	size := int64(len(object.Content))
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

	err := t.tarWriter.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("write header to TAR %v: %w", object.Path, err)
	}

	if writeContent {
		_, err = t.tarWriter.Write(object.Content)
		if err != nil {
			return fmt.Errorf("write content to TAR %v: %w", object.Path, err)
		}
	}

	t.size += int(size) + len(object.Path)

	return nil
}

type TarReader struct {
	zstdReader *zstd.Decoder
	tarReader  *tar.Reader
}

func NewTarReader(content []byte) *TarReader {
	zstdReader, err := zstd.NewReader(bytes.NewBuffer(content))
	if err != nil {
		panic("assert not reached: invalid ZSTD reader options")
	}

	return &TarReader{
		zstdReader: zstdReader,
		tarReader:  tar.NewReader(zstdReader),
	}
}

func (t *TarReader) Next() (*tar.Header, error) {
	return t.tarReader.Next()
}

func (t *TarReader) ReadContent() ([]byte, error) {
	var buffer bytes.Buffer
	_, err := io.Copy(&buffer, t.tarReader)
	if err != nil {
		return nil, fmt.Errorf("read content from TarReader: %w", err)
	}

	return buffer.Bytes(), nil
}

func (t *TarReader) CopyContent(buffer io.Writer) error {
	_, err := io.Copy(buffer, t.tarReader)
	return err
}

func (t *TarReader) Close() {
	t.zstdReader.Close()
}

func PackObjects(objects ObjectStream) ([]byte, []byte, error) {
	contentWriter := NewTarWriter()
	namesWriter := NewTarWriter()
	empty := true

	for {
		object, err := objects()
		if err == SKIP {
			continue
		}
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

func updateObjects(before []byte, updates []*pb.Object) ([]byte, []byte, error) {
	reader := NewTarReader(before)

	stream := func() (*pb.Object, error) {
		header, err := reader.Next()
		if err == io.EOF {
			return nil, io.EOF
		}
		if err != nil {
			return nil, err
		}

		update := findUpdate(updates, header.Name)
		if update != nil {
			if update.Deleted {
				return nil, SKIP
			}

			return update, nil
		}

		content, err := reader.ReadContent()
		if err != nil {
			return nil, err
		}

		return &pb.Object{
			Path:    header.Name,
			Mode:    int32(header.Mode),
			Size:    int32(header.Size),
			Deleted: false,
			Content: content,
		}, nil
	}

	return PackObjects(stream)
}

func findUpdate(updates []*pb.Object, path string) *pb.Object {
	for _, object := range updates {
		if path == object.Path {
			return object
		}
	}
	return nil
}
