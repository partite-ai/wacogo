package canon

import (
	"github.com/tetratelabs/wazero/api"
)

// transferSide describes one side of a call: the component instance
// (whose Enter gates may_enter for the callee), memory, realloc, string
// encoding, and resource table. Caller-side may have a nil Instance —
// no reentrance gating applies.
//
// BatchHelper, when non-nil, points to a per-adapter wasm helper that
// batches N cabi_realloc calls behind a single Go→wasm crossing. It is
// instantiated at adapter build time only when the transfer plan contains
// a step that benefits from batching (e.g. list<string> param transfer)
// and the side's StringEncoding matches the source's. Runtime code that
// can take a batched path checks for nil and falls back to per-element
// realloc when it is missing.
type transferSide struct {
	Instance       Instance
	Memory         api.Memory
	Realloc        ReallocFunc
	StringEncoding StringEncoding
	ResourceTable  ResourceTable
	BatchHelper    *batchReallocHelper
}

// transferContext is the per-call state for a component→component transfer.
type transferContext struct {
	caller    *transferSide
	callee    *transferSide
	registers []uint64 // the wazero host-callback stack
	Task      Task

	// allocSrc and allocSink are per-call scratch reused across runTransferPlan
	// invocations. They escape to heap because the visitor closures take
	// them by pointer through indirect calls; living on the adapter-owned
	// transferContext means that heap allocation is paid once at adapter
	// build time, not once per call.
	allocSrc  allocSource
	allocSink allocSink
}

// endTaskOrTrap is the per-call End-of-task hook used as a method-bound
// defer in runTransferPlan. Drains tc.Task and traps on a drain error.
// If a panic is in flight, preserves it (the borrow-leak message is
// informative only at that point). Kept as a method (not a func literal)
// so escape analysis can keep tc on the caller's stack.
func (tc *transferContext) endTaskOrTrap() {
	endErr := tc.Task.End()
	if r := recover(); r != nil {
		panic(r)
	}
	if endErr != nil {
		trapf("%s", endErr.Error())
	}
}

// gocallContext is the per-call state for a Go→component call.
type gocallContext struct {
	callee    *transferSide
	registers []uint64 // core args/results slice
	Task      Task
}

// endTask is the per-call End-of-task hook used as a method-bound defer
// in runGocallPlan. Drains gcc.Task and writes any drain error into *perr
// when *perr is still nil. Kept as a method (not a func literal) so escape
// analysis can keep the gocallContext on the caller's stack.
func (gcc *gocallContext) endTask(perr *error) {
	if endErr := gcc.Task.End(); endErr != nil && *perr == nil {
		*perr = endErr
	}
}
