package canon

import (
	"github.com/tetratelabs/wazero/api"
)

// transferSide describes one side of a call: the component instance
// (whose Enter gates may_enter for the callee), memory, realloc, string
// encoding, and resource table. Caller-side may have a nil Instance —
// no reentrance gating applies.
type transferSide struct {
	Instance       Instance
	Memory         api.Memory
	Realloc        ReallocFunc
	StringEncoding StringEncoding
	ResourceTable  ResourceTable
}

// transferContext is the per-call state for a component→component transfer.
type transferContext struct {
	caller    *transferSide
	callee    *transferSide
	registers []uint64 // the wazero host-callback stack
	Task      Task
}

// newTransferContext constructs a transferContext with the given sides and
// register stack. stack is the wazero host-callback slice — caller params
// on entry, callee results on exit.
func newTransferContext(caller, callee *transferSide, stack []uint64) *transferContext {
	return &transferContext{
		caller:    caller,
		callee:    callee,
		registers: stack,
	}
}

// newGocallContext constructs a gocallContext for a Go→component call.
// Input args and result Vals flow through runGocallPlan's parameters and
// return value — the context holds only per-call state shared across
// step closures (callee side, core register slice).
func newGocallContext(callee *transferSide) *gocallContext {
	return &gocallContext{callee: callee}
}

// gocallContext is the per-call state for a Go→component call.
type gocallContext struct {
	callee    *transferSide
	registers []uint64 // core args/results slice
	Task      Task
}
