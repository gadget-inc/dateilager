//go:build linux

package files

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// reflinkFile performs the actual reflink action using the FICLONE ioctl
// without handling any fallback mechanism. On Linux, this uses the FICLONE ioctl
// which efficiently creates a copy-on-write clone of the entire source file.
//
// This operation requires both files to be on the same filesystem that supports
// reflinks (like btrfs or xfs with reflink=1 mount option).
func reflinkFile(source, target string) error {
	s, err := os.Open(source)
	if err != nil {
		return err
	}
	defer s.Close()

	d, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer d.Close()

	ss, err := s.SyscallConn()
	if err != nil {
		return err
	}
	sd, err := d.SyscallConn()
	if err != nil {
		return err
	}

	var err2, err3 error

	err = sd.Control(func(dfd uintptr) {
		err2 = ss.Control(func(sfd uintptr) {
			// int ioctl(int dest_fd, FICLONE, int src_fd);
			err3 = unix.IoctlFileClone(int(dfd), int(sfd))
		})
	})

	if err != nil {
		// sd.Control failed
		return err
	}
	if err2 != nil {
		// ss.Control failed
		return err2
	}

	if err3 != nil && errors.Is(err3, unix.ENOTSUP) {
		return errors.New("reflink failed: not supported on this filesystem")
	}

	// err3 is ioctl() response
	return err3
}
