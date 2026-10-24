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

	// maxLiftBytes bounds each lift performed by adapters this Host
	// builds; 0 means unlimited.
	maxLiftBytes uint64
}

// NewHost wraps a wazero.Runtime in a canon Host. maxLiftBytes bounds the
// bytes any single lift performed by the Host's adapters may materialize
// (0 means unlimited).
func NewHost(runtime wazero.Runtime, maxLiftBytes uint64) *Host {
	return &Host{
		runtime:          runtime,
		batchHelperCache: make(map[string]wazero.CompiledModule),
		maxLiftBytes:     maxLiftBytes,
	}
}
