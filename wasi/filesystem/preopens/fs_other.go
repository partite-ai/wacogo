//go:build !unix

package preopens

import "io/fs"

func linkAt(_ fs.File, _ string, _ fs.File, _ string) error {
	return errUnsupported
}

func renameAt(_ fs.File, _ string, _ fs.File, _ string) error {
	return errUnsupported
}
