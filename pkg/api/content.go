package api

import (
	"bytes"
	"io"

	"github.com/klauspost/compress/zstd"
)

type ContentEncoder struct {
	buffer *bytes.Buffer
	writer *zstd.Encoder
}

func NewContentEncoder() *ContentEncoder {
	var buffer bytes.Buffer
	writer, err := zstd.NewWriter(&buffer, zstd.WithEncoderLevel(zstd.SpeedFastest))
	if err != nil {
		panic("assert not reached: invalid ZSTD writer options")
	}

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
	buffer *bytes.Buffer
	reader *zstd.Decoder
}

func NewContentDecoder() *ContentDecoder {
	var buffer bytes.Buffer
	reader, err := zstd.NewReader(nil)
	if err != nil {
		panic("assert not reached: invalid ZSTD reader options")
	}

	return &ContentDecoder{
		buffer: &buffer,
		reader: reader,
	}
}

func (c *ContentDecoder) Decoder(encoded []byte) ([]byte, error) {
	c.buffer.Truncate(0)
	c.reader.Reset(bytes.NewBuffer(encoded))

	_, err := io.Copy(c.buffer, c.reader)
	if err != nil {
		return nil, err
	}

	return c.buffer.Bytes(), nil
}
