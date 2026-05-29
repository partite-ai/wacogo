//go:build unix

package preopens

import (
	"io/fs"

	"golang.org/x/sys/unix"
)

// linkAt invokes linkat(2) when both files expose unix file descriptors,
// using AT_SYMLINK_FOLLOW to match POSIX hard-link semantics.
func linkAt(srcFile fs.File, srcPath string, dstFile fs.File, dstPath string) error {
	src, ok := as[fdFile](srcFile)
	if !ok {
		return errUnsupported
	}
	dst, ok := as[fdFile](dstFile)
	if !ok {
		return errUnsupported
	}
	return unix.Linkat(int(src.Fd()), srcPath, int(dst.Fd()), dstPath, unix.AT_SYMLINK_FOLLOW)
}

// renameAt invokes renameat(2) when both files expose unix file
// descriptors.
func renameAt(srcFile fs.File, srcPath string, dstFile fs.File, dstPath string) error {
	src, ok := as[fdFile](srcFile)
	if !ok {
		return errUnsupported
	}
	dst, ok := as[fdFile](dstFile)
	if !ok {
		return errUnsupported
	}
	return unix.Renameat(int(src.Fd()), srcPath, int(dst.Fd()), dstPath)
}
