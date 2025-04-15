//go:build linux

package files

import (
	"errors"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

// COPY_FILE_CLONE is a flag for copy_file_range that indicates we want to create a reflink
const COPY_FILE_CLONE = 0x1

// reflinkFile performs the actual reflink action using copy_file_range with COPY_FILE_CLONE
// without handling any fallback mechanism. On Linux, this uses the copy_file_range syscall
// with COPY_FILE_CLONE flag which efficiently creates a copy-on-write clone of the entire source file.
//
// This operation requires both files to be on the same filesystem that supports
// reflinks (like btrfs or xfs with reflink=1 mount option).
func reflinkFile(source, target string, perm fs.FileMode) error {
	s, err := os.Open(source)
	if err != nil {
		return err
	}
	defer s.Close()

	d, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer d.Close()

	// Get file size for the copy
	info, err := s.Stat()
	if err != nil {
		return err
	}
	size := info.Size()

	// Use copy_file_range with COPY_FILE_CLONE flag
	_, err = unix.CopyFileRange(int(s.Fd()), nil, int(d.Fd()), nil, int(size), COPY_FILE_CLONE)
	if err != nil {
		if errors.Is(err, unix.ENOTSUP) {
			return errors.New("reflink failed: not supported on this filesystem")
		}
		return err
	}

	return nil
}
