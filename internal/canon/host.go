package canon

import (
	"sync"

	"github.com/tetratelabs/wazero"
)

// Host is the canon-side factory hub. Owns the wazero runtime used to build
// host modules and stub modules. One Host per Engine.
type Host struct {
	runtime wazero.Runtime

	// batchHelperCache memoizes compiled batched-realloc helper modules,
	// keyed by the target realloc's export name (the helper's only
	// target-specific bake-in; the module name is a fixed placeholder
	// rebound per instantiation via an ImportResolver). In practice the
	// key is almost always "cabi_realloc", so the whole process compiles
	// a single helper module instead of one per adapter.
	mu               sync.Mutex
	batchHelperCache map[string]wazero.CompiledModule
}

// NewHost wraps a wazero.Runtime in a canon Host.
func NewHost(runtime wazero.Runtime) *Host {
	return &Host{
		runtime:          runtime,
		batchHelperCache: make(map[string]wazero.CompiledModule),
	}
}
