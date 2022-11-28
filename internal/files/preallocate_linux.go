//go:build linux

package files

import (
	"os"

	"golang.org/x/sys/unix"
)

func PreAllocate(file *os.File, size int64) error {
	if size > 0 {
		return unix.Fallocate(int(file.Fd()), 0, 0, size)
	}
	return nil
}
