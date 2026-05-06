// Package wasierr exposes errors shared by the wasi/* skeleton packages.
package wasierr

import "errors"

// ErrNotImplemented is returned (wrapped with the call site) by every
// not-yet-implemented method in the wasi/* skeleton packages. Use
// errors.Is to detect.
var ErrNotImplemented = errors.New("wasi: not implemented")
