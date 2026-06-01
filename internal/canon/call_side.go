package canon

import "github.com/tetratelabs/wazero/api"

// CallSide describes one side of a canonical-ABI call: a component instance
// plus its memory/realloc/encoding options. Used by Callee and by transfer
// factories.
//
// Instance must be non-nil. Every call has a defining component instance
// on each side; callers that don't need reentrance gating or a resource
// table for tests can supply a no-op stub.
type CallSide struct {
	Instance       Instance
	Memory         api.Memory
	Realloc        api.Function
	StringEncoding StringEncoding

	// ReallocModName is the wazero module name (set at instantiation time
	// via WithName) of the module that exports Realloc. Used by
	// Host.BuildAdapter to wire batched-realloc helper imports — the
	// helper imports "tgt"."<name>" and wazero resolves "tgt" against
	// this module name. Optional: when empty, no batched-realloc helper
	// is built for this side, and the transfer takes the per-element
	// fallback path.
	ReallocModName string

	// ReallocFnExport is the export name of Realloc within
	// ReallocModName. Typically "cabi_realloc" or "realloc". Required
	// when ReallocModName is set.
	ReallocFnExport string
}

// Callee extends CallSide with the concrete core function to invoke and an
// optional post-return hook. Constructed by callers (root or test code) and
// handed to NewCallBinding / Host.BuildAdapter.
type Callee struct {
	CallSide
	CoreFunc   api.Function
	PostReturn api.Function // optional; nil if the callee has no post-return
}
