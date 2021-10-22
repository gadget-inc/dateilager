package db

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/klauspost/compress/s2"
)

var (
	ErrEmptyPack = errors.New("empty object stream to pack")
)

type TarWriter struct {
	size      int
	buffer    *bytes.Buffer
	s2Writer  *s2.Writer
	tarWriter *tar.Writer
}

func NewTarWriter() *TarWriter {
	var buffer bytes.Buffer
	s2Writer := s2.NewWriter(&buffer)

	return &TarWriter{
		size:      0,
		buffer:    &buffer,
		s2Writer:  s2Writer,
		tarWriter: tar.NewWriter(s2Writer),
	}
}

func (t *TarWriter) BytesAndReset() ([]byte, error) {
	err := t.tarWriter.Close()
	if err != nil {
		return nil, fmt.Errorf("close TarWriter.tarWriter: %w", err)
	}

	err = t.s2Writer.Close()
	if err != nil {
		return nil, fmt.Errorf("close TarWriter.s2Writer: %w", err)
	}

	output := t.buffer.Bytes()

	t.size = 0
	t.buffer.Truncate(0)
	t.s2Writer.Reset(t.buffer)
	t.tarWriter = tar.NewWriter(t.s2Writer)

	return output, nil
}

func (t *TarWriter) Size() int {
	return t.size
}

func (t *TarWriter) WriteObject(object *pb.Object, writeContent bool) error {
	typeFlag := object.TarType()

	size := int64(len(object.Content))
	if !writeContent || typeFlag == tar.TypeDir || typeFlag == tar.TypeSymlink {
		size = 0
	}

	header := &tar.Header{
		Name:     object.Path,
		Mode:     int64(object.FileMode().Perm()),
		Typeflag: typeFlag,
		Size:     size,
		Format:   tar.FormatPAX,
	}

	if typeFlag == tar.TypeSymlink {
		header.Linkname = string(object.Content)
	}

	err := t.tarWriter.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("write header to TAR %v: %w", object.Path, err)
	}

	if size > 0 {
		_, err = t.tarWriter.Write(object.Content)
		if err != nil {
			return fmt.Errorf("write content to TAR %v: %w", object.Path, err)
		}
	}

	t.size += int(size) + len(object.Path)

	return nil
}

type TarReader struct {
	s2Reader  *s2.Reader
	tarReader *tar.Reader
}

func NewTarReader(content []byte) *TarReader {
	s2Reader := s2.NewReader(bytes.NewBuffer(content))

	return &TarReader{
		s2Reader:  s2Reader,
		tarReader: tar.NewReader(s2Reader),
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
	seenPaths := make(map[string]bool)
	idxHint := 0

	reader := NewTarReader(before)
	readerObjectsRemaining := true

	stream := func() (*pb.Object, error) {
		// Yield unseen updates as new objects if we've finished walking the original pack
		if !readerObjectsRemaining {
			for idx, object := range updates[idxHint:] {
				if _, ok := seenPaths[object.Path]; !ok {
					seenPaths[object.Path] = true
					idxHint = idx

					if object.Deleted {
						return nil, SKIP
					}
					return object, nil
				}
			}
			return nil, io.EOF
		}

		header, err := reader.Next()
		if err == io.EOF {
			readerObjectsRemaining = false
			return nil, SKIP
		}
		if err != nil {
			return nil, err
		}

		seenPaths[header.Name] = true

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

		return pb.ObjectFromTarHeader(header, content), nil
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
