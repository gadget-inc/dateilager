//go:build linux

package files

import (
	"errors"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

// FICLONE is the ioctl command for file cloning (reflink)
const FICLONE = 0x40049409

// reflinkFile performs the actual reflink action using the FICLONE ioctl.
// This creates a copy-on-write clone of the source file.
//
// This operation requires both files to be on the same filesystem that supports
// reflinks (like btrfs or xfs with reflink=1).
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

	// Use FICLONE ioctl to create a reflink
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, d.Fd(), FICLONE, s.Fd())
	if errno != 0 {
		if errno == unix.ENOTTY || errno == unix.EOPNOTSUPP {
			return errors.New("reflink failed: not supported on this filesystem")
		}
		return errno
	}

	return nil
}
