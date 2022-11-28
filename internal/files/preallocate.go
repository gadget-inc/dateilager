//go:build !linux

package files

import (
	"os"
)

func PreAllocate(file *os.File, size int64) error {
	return nil
}
