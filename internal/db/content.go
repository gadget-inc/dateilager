package db

import (
	"bytes"
	"io"

	"github.com/klauspost/compress/s2"
	"github.com/minio/sha256-simd"
)

func HashContent(data []byte) ([]byte, []byte) {
	sha := sha256.Sum256(data)
	return sha[0:16], sha[16:]
}

type ContentEncoder struct {
	buffer *bytes.Buffer
	writer *s2.Writer
}

func NewContentEncoder() *ContentEncoder {
	var buffer bytes.Buffer
	writer := s2.NewWriter(&buffer)

	return &ContentEncoder{
		buffer: &buffer,
		writer: writer,
	}
}

func (c *ContentEncoder) Encode(content []byte) ([]byte, error) {
	_, err := c.writer.Write(content)
	if err != nil {
		return nil, err
	}

	err = c.writer.Close()
	if err != nil {
		return nil, err
	}

	output := c.buffer.Bytes()

	c.buffer.Truncate(0)
	c.writer.Reset(c.buffer)

	if output == nil {
		output = []byte("")
	}

	return output, nil
}

type ContentDecoder struct {
	reader *s2.Reader
}

func NewContentDecoder() *ContentDecoder {
	reader := s2.NewReader(nil)

	return &ContentDecoder{
		reader: reader,
	}
}

func (c *ContentDecoder) Decoder(encoded []byte) ([]byte, error) {
	c.reader.Reset(bytes.NewBuffer(encoded))
	output, err := io.ReadAll(c.reader)
	if err != nil {
		return nil, err
	}

	return output, nil
}
