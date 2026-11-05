package canon

import (
	"context"

	"github.com/tetratelabs/wazero"
)

// Host is the canon-side factory hub. Owns the wazero runtime used to build
// host modules and stub modules. One Host per Engine.
type Host struct {
	runtime wazero.Runtime

	// batchHelper is the one compiled batched-realloc helper, shared by every
	// adapter and freed with the runtime. If it failed to compile, as a runtime
	// configuration could make it, batchHelperErr says why and building a helper
	// reports it.
	batchHelper    wazero.CompiledModule
	batchHelperErr error

	// maxLiftBytes bounds each lift performed by adapters this Host
	// builds; 0 means unlimited.
	maxLiftBytes uint64
}

// NewHost wraps a wazero.Runtime in a canon Host. maxLiftBytes bounds the
// bytes any single lift performed by the Host's adapters may materialize
// (0 means unlimited).
func NewHost(ctx context.Context, runtime wazero.Runtime, maxLiftBytes uint64) *Host {
	helper, err := runtime.CompileModule(ctx, encodeBatchReallocHelperModule())
	return &Host{
		runtime:        runtime,
		batchHelper:    helper,
		batchHelperErr: err,
		maxLiftBytes:   maxLiftBytes,
	}
}
