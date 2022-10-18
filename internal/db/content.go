package db

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
	"github.com/klauspost/compress/s2"
	"github.com/minio/sha256-simd"
)

type Hash struct {
	H1 [16]byte
	H2 [16]byte
}

func HashContent(data []byte) Hash {
	sha := sha256.Sum256(data)
	return Hash{
		H1: *(*[16]byte)(sha[0:16]),
		H2: *(*[16]byte)(sha[16:32]),
	}
}

func (h *Hash) Bytes() []byte {
	var hash []byte
	hash = append(hash, h.H1[:]...)
	hash = append(hash, h.H2[:]...)
	return hash
}

func (h *Hash) Hex() string {
	return hex.EncodeToString(h.Bytes())
}

type ContentEncoder struct {
	buffer *bytes.Buffer
	writer *s2.Writer
}

func NewContentEncoder() *ContentEncoder {
	var buffer bytes.Buffer
	writer := s2.NewWriter(&buffer, s2.WriterConcurrency(1))

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

func RandomContents(ctx context.Context, tx pgx.Tx, sample float32) ([]Hash, error) {
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT (hash).h1, (hash).h2
		FROM dl.contents
		TABLESAMPLE SYSTEM(%f)
	`, sample))
	if err != nil {
		return nil, fmt.Errorf("random contents: %w", err)
	}

	var hashes []Hash

	for rows.Next() {
		var hash Hash
		err = rows.Scan(&hash.H1, &hash.H2)
		if err != nil {
			return nil, fmt.Errorf("random contents scan: %w", err)
		}

		hashes = append(hashes, hash)
	}

	return hashes, nil
}
