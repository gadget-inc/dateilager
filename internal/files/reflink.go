//go:build !linux && !darwin

package files

import (
	"errors"
	"io/fs"
)

// reflinkFile performs the actual reflink action using the FICLONE ioctl
// without handling any fallback mechanism. On Linux, this uses the FICLONE ioctl
// which efficiently creates a copy-on-write clone of the entire source file.
//
// This operation requires both files to be on the same filesystem that supports
// reflinks (like btrfs or xfs with reflink=1 mount option).
func reflinkFile(source, target string, perm fs.FileMode) error {
	return errors.New("reflink failed: not supported on this platform")
}
