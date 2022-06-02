package stringutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShortenString(t *testing.T) {
	testCases := []struct {
		input  string
		outLen int
		output string
	}{
		{"gadget.dev", 6, "gadget"},
		{"blah", 34, "blah"},
		{"gadget.dev", 10, "gadget.dev"},
	}
	for i, tc := range testCases {
		res := ShortenString(tc.input, tc.outLen)
		assert.Equal(t, tc.output, res, "case[%d]", i)
	}
}
