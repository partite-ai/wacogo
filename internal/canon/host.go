package canon

import "github.com/tetratelabs/wazero"

// Host is the canon-side factory hub. Owns the wazero runtime used to build
// host modules and stub modules. One Host per Engine.
type Host struct {
	runtime wazero.Runtime
}

// NewHost wraps a wazero.Runtime in a canon Host.
func NewHost(runtime wazero.Runtime) *Host {
	return &Host{runtime: runtime}
}
