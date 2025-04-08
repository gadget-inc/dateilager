package db

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/dgraph-io/ristretto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/puddle/v2"
	"github.com/klauspost/compress/s2"
	"github.com/minio/sha256-simd"
)

const (
	KB              = 1024
	MB              = KB * KB
	DecoderPoolSize = 200
)

type Hash struct {
	H1 [16]byte
	H2 [16]byte
}

type LookupParams struct {
	IsEncoded  bool
	IsOversize bool
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

// Stolen from Go's standard library
// And optimized for the Hash type
const hextable = "0123456789abcdef"

func (h *Hash) Hex() string {
	buffer := make([]byte, 64)
	idx := 0

	for _, v := range h.H1 {
		buffer[idx] = hextable[v>>4]
		buffer[idx+1] = hextable[v&0x0f]
		idx += 2
	}

	for _, v := range h.H2 {
		buffer[idx] = hextable[v>>4]
		buffer[idx+1] = hextable[v&0x0f]
		idx += 2
	}

	return string(buffer)
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

func (c *ContentEncoder) Encode(content DecodedContent) (EncodedContent, error) {
	_, err := c.writer.Write(content)
	if err != nil {
		return nil, err
	}

	err = c.writer.Close()
	if err != nil {
		return nil, err
	}

	tmpOutput := c.buffer.Bytes()

	c.buffer.Reset()
	c.writer.Reset(c.buffer)

	if tmpOutput == nil {
		return []byte(""), nil
	}

	output := make([]byte, len(tmpOutput))
	copy(output, tmpOutput)
	return output, nil
}

func (c *ContentEncoder) Close() error {
	return c.writer.Close()
}

type ContentDecoder struct {
	buffer *bytes.Buffer
	reader *s2.Reader
}

func NewContentDecoder() *ContentDecoder {
	var buffer bytes.Buffer
	reader := s2.NewReader(nil)

	return &ContentDecoder{
		buffer: &buffer,
		reader: reader,
	}
}

func (c *ContentDecoder) Decode(encoded EncodedContent) (DecodedContent, error) {
	c.buffer.Reset()
	c.reader.Reset(bytes.NewReader(encoded))

	_, err := io.Copy(c.buffer, c.reader)
	if err != nil {
		return nil, err
	}

	output := make([]byte, c.buffer.Len())
	copy(output, c.buffer.Bytes())
	return output, nil
}

type ContentLookup struct {
	cache    *ristretto.Cache
	decoders *puddle.Pool[*ContentDecoder]
}

func NewContentLookup() (*ContentLookup, error) {
	cacheSize := int64(1_000)
	cacheSizeEnv := os.Getenv("DL_CACHE_SIZE")
	if cacheSizeEnv != "" {
		size, err := strconv.ParseInt(cacheSizeEnv, 10, 64)
		if err == nil {
			cacheSize = size
		}
	}

	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 100_000,
		MaxCost:     cacheSize * MB,
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}

	constructor := func(context.Context) (*ContentDecoder, error) {
		return NewContentDecoder(), nil
	}

	decoders, err := puddle.NewPool(&puddle.Config[*ContentDecoder]{Constructor: constructor, MaxSize: DecoderPoolSize})
	if err != nil {
		return nil, err
	}

	return &ContentLookup{
		cache:    cache,
		decoders: decoders,
	}, nil
}

func (cl *ContentLookup) Lookup(ctx context.Context, tx pgx.Tx, hashesToLookup map[Hash]LookupParams) (map[Hash]DecodedContent, error) {
	var notFound []Hash
	contents := make(map[Hash]DecodedContent, len(hashesToLookup))

	decoder, err := cl.decoders.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot acquire content decoder: %w", err)
	}
	defer decoder.Release()

	for hash, params := range hashesToLookup {
		if params.IsOversize {
			continue
		}
		value, found := cl.cache.Get(hash.Hex())
		if found {
			if params.IsEncoded {
				decoded, err := decoder.Value().Decode(value.(EncodedContent))
				if err != nil {
					return nil, fmt.Errorf("cannot decode value from cache %v: %w", hash.Hex(), err)
				}
				contents[hash] = decoded
			} else {
				contents[hash] = value.(DecodedContent)
			}
		} else {
			notFound = append(notFound, hash)
		}
	}

	if len(notFound) > 0 {
		rows, err := tx.Query(ctx, `
			SELECT (hash).h1, (hash).h2, bytes
			FROM dl.contents
			WHERE hash = ANY($1::hash[])
		`, notFound)
		if err != nil {
			return nil, fmt.Errorf("lookup missing hash contents: %w", err)
		}

		for rows.Next() {
			var hash Hash
			var value []byte

			err = rows.Scan(&hash.H1, &hash.H2, &value)
			if err != nil {
				return nil, fmt.Errorf("content lookup scan: %w", err)
			}

			// This is a content addressable cache, any cached value will never be updated
			cl.cache.Set(hash.Hex(), value, int64(len(value)))

			if hashesToLookup[hash].IsEncoded {
				decoded, err := decoder.Value().Decode(value)
				if err != nil {
					return nil, fmt.Errorf("cannot decode value from content table %v: %w", hash.Hex(), err)
				}
				contents[hash] = decoded
			} else {
				contents[hash] = value
			}
		}

		err = rows.Err()
		if err != nil {
			return nil, fmt.Errorf("failed to iterate rows: %w", err)
		}
	}

	return contents, nil
}

func RandomContents(ctx context.Context, conn DbConnector, sample float32) ([]Hash, error) {
	rows, err := conn.Query(ctx, fmt.Sprintf(`
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

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return hashes, nil
}
