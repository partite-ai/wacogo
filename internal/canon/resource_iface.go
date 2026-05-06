// internal/canon/resource_iface.go
package canon

import "context"

// TransferTarget is a destination for an own handle being transferred
// in. Implemented by ResourceTable (allocates a fresh wire-handle slot)
// and *ValOwnHandle (stores a free-floating handle in the Val).
type TransferTarget interface {
	// IssueOwn places (rt, rep) in the target and returns the resulting
	// handle. The visitor that invokes TransferOwn writes the returned
	// handle's HandleID() into the wasm slot when the target is a table.
	IssueOwn(rt ResourceType, rep uint32) ResourceHandle
}

// ResourceTable is the canon-side view of a per-instance canonical-ABI
// resource table. core.ResourceTable is the concrete implementation;
// tests may provide stubs. Embeds TransferTarget — every table is a
// valid TransferOwn destination.
type ResourceTable interface {
	TransferTarget

	// LookupOwn returns the existing own-kind handle for h, or an
	// error if h is unknown, of the wrong kind, or of the wrong
	// resource type.
	LookupOwn(rt ResourceType, h uint32) (ResourceHandle, error)

	// LookupBorrowable returns the existing handle for h. Either own
	// or borrow kind is acceptable — borrow lifts may source from an
	// own handle (lift_borrow accepts both per spec). Errors on
	// unknown handle or wrong type.
	LookupBorrowable(rt ResourceType, h uint32) (ResourceHandle, error)

	// IssueBorrow allocates a fresh borrow-kind handle for rep, scoped
	// to task. task's NumBorrows is incremented; dropping the returned
	// handle decrements it. Used by LendTo on a free-floating handle
	// (valBacking) to allocate the callee-side borrow.
	IssueBorrow(rt ResourceType, rep uint32, task *Task) ResourceHandle

	// Owner returns the component instance that owns this table, or
	// nil if the table is unowned (e.g. test stubs). LendTo's same-
	// component shortcut compares Owner() against the resource type's
	// defining instance.
	Owner() Instance
}

// ResourceHandle is one entry in a ResourceTable, viewed as an opaque
// handle. Operations on a handle whose underlying slot has been freed
// (via Drop, TransferOwn, or LendTo) panic — use-after-free is a
// programmer bug, not a runtime condition.
type ResourceHandle interface {
	// HandleID returns the 1-indexed slot id this handle refers to,
	// or 0 for detached handles (no underlying table slot).
	HandleID() uint32

	// Rep returns the rep value stored against the handle.
	Rep() uint32

	// Type returns the resource type associated with this handle. Used by
	// visitors that need to confirm the call's expected type matches the
	// handle's actual type before lowering.
	Type() ResourceType

	// LendTo lifts a borrow of this handle scoped to task. The source
	// handle's numLends is incremented; an unlend release is registered
	// on task; a fresh borrow handle is allocated on target (unless the
	// same-component shortcut applies, in which case rep flows through
	// unchanged with no allocation and no release). The returned handle
	// is the callee-side view; callers write its HandleID() into the
	// wasm slot.
	LendTo(target ResourceTable, task *Task) (ResourceHandle, error)

	// TransferOwn moves an own handle to target, invalidating the
	// source. target must be non-nil — pass a *ValOwnHandle to lift
	// out into Go-side ownership, or a ResourceTable to allocate a
	// callee-side wire handle.
	TransferOwn(target TransferTarget) (ResourceHandle, error)

	// Drop removes the slot and runs the dtor (own kinds only). For
	// borrow kinds, just decrements the borrow scope. Idempotent only
	// in the sense that the handle is invalid afterward.
	Drop(ctx context.Context) error
}
