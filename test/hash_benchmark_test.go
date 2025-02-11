package test

import (
	"crypto/rand"
	"testing"

	"github.com/gadget-inc/dateilager/internal/db"
	"github.com/stretchr/testify/assert"
)

//go:bench
func BenchmarkHex(b *testing.B) {
	data := make([]byte, 64)
	_, err := rand.Read(data)
	assert.NoError(b, err, "failed to create random bytes")

	for n := 0; n < b.N; n++ {
		hash := db.HashContent(data)
		_ = hash.Hex()
	}
}
