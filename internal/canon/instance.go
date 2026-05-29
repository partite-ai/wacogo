// Package canon is the canonical ABI implementation.
package canon

import "context"

// Instance is the canon-side view of a component instance. The defining
// instance of a resource type, the callee instance of a call binding, and
// the caller/callee of a transfer all flow through this interface.
//
// Pointer-equality on Instance is meaningful: same-component shortcuts
// compare two Instance values for identity, so concrete implementations
// must always thread the same pointer for a given component instance.
type Instance interface {
	// Enter locks the instance against reentrance. The returned exit
	// closure must be called exactly once when the call completes
	// (defer it). Returns a non-nil error if the instance is already
	// entered.
	Enter(ctx context.Context) (exit func(ctx context.Context), err error)

	// CanLeave reports the may_leave flag. The flag is true initially
	// and is cleared for the duration of canonical-ABI value lowering
	// (and post-return) so that, while those intermediate sequences
	// run, the component cannot make outgoing calls — preserving the
	// spec invariant that lifting-then-lowering can be fused into a
	// direct copy. Many "leaving" intrinsics (canon.lower, resource.new,
	// resource.drop, task.return, etc.) trap when this is false; see
	// the canonical-ABI definitions.py for the exhaustive list.
	CanLeave() bool

	// SuspendLeave clears may_leave for the duration of a canonical-ABI
	// value-lowering window (param lowering, result lowering, post-return).
	// The returned closure restores the prior may_leave value and must be
	// called exactly once when the window closes; defer it. Nested calls
	// stack — the closure restores whatever value was in effect on entry.
	SuspendLeave() (restore func())

	// ResourceTable returns the per-instance resource handle table.
	ResourceTable() ResourceTable

	// Poison marks the instance as unusable due to a trap. Subsequent
	// Enter calls must fail with the captured reason. Idempotent.
	Poison(reason error)
}

