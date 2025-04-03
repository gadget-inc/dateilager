//go:build darwin

package files

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// reflinkFile performs the actual reflink action using the clonefile syscall
// without handling any fallback mechanism. On Darwin, this uses the clonefile syscall
// which efficiently creates a copy-on-write clone of the entire source file.
//
// This operation requires both files to be on the same filesystem that supports
// reflinks (like APFS).
func reflinkFile(source, target string) error {
	err := unix.Clonefile(source, target, unix.CLONE_NOFOLLOW)
	if err != nil {
		if err == unix.ENOTSUP || err == unix.EXDEV {
			return fmt.Errorf("reflink not supported: %w", err)
		}
		return err
	}

	return nil
}
