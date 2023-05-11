package buffers

import (
	"bytes"
	"sync"
)

var bufferPool = &sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

func Get() *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

func GetWith(bs []byte) *bytes.Buffer {
	buf := Get()
	buf.Write(bs)
	return buf
}

func Put(buf *bytes.Buffer) {
	bufferPool.Put(buf)
}
