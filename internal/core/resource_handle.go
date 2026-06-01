package core

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo/internal/canon"
)

// ResourceHandle is the core-side handle interface. Methods speak in
// core types — TransferTarget for transfer destinations, core.ResourceHandle
// for returns. The canon-side view (canon.ResourceHandle) is provided
// via the canonResourceHandleView adapter for canon visitor use.
type ResourceHandle interface {
	HandleID() uint32
	Rep() uint32
	LendTo(target TransferTarget, task *canon.Task) (ResourceHandle, error)
	TransferOwn(target TransferTarget) (ResourceHandle, error)
	Drop(ctx context.Context) error

	// Instance returns the *ComponentInstance whose canon table holds
	// this handle's entry. Nil for detached handles.
	Instance() *ComponentInstance

	// Type returns the *TypeResource that identifies this handle's
	// resource type. Type().Instance() is the defining component
	// instance.
	Type() *TypeResource
}

// liveResourceHandle is the table-backed implementation. valid flips to
// false on Drop or TransferOwn — subsequent calls panic.
type liveResourceHandle struct {
	table   *ResourceTable
	slotIdx uint32
	valid   bool
}

func (h *liveResourceHandle) checkValid(op string) {
	if !h.valid {
		panic(fmt.Sprintf("wacogo/core: %s on invalid resource handle", op))
	}
}

func (h *liveResourceHandle) HandleID() uint32 {
	h.checkValid("HandleID")
	return h.slotIdx + 1
}

func (h *liveResourceHandle) Instance() *ComponentInstance {
	h.checkValid("Instance")
	return h.table.owner
}

func (h *liveResourceHandle) Type() *TypeResource {
	h.checkValid("Type")
	return h.table.entries[h.slotIdx].tr
}

func (h *liveResourceHandle) Rep() uint32 {
	h.checkValid("Rep")
	return h.table.entries[h.slotIdx].rep
}

func (h *liveResourceHandle) LendTo(target TransferTarget, task *canon.Task) (ResourceHandle, error) {
	h.checkValid("LendTo")
	e := &h.table.entries[h.slotIdx]
	tr := e.tr
	rep := e.rep

	if target == nil {
		return nil, fmt.Errorf("LendTo: nil target")
	}
	tt, ok := target.(*ResourceTable)
	if !ok {
		return nil, fmt.Errorf("LendTo: invalid target type %T", target)
	}

	// bump source numLends and register release on task — done in both
	// the same-component shortcut and the general cross-component path.
	e.numLends++
	src := h
	task.AddRelease(func() {
		if !src.valid {
			return
		}
		if src.table.entries[src.slotIdx].numLends > 0 {
			src.table.entries[src.slotIdx].numLends--
		}
	})

	// Same-component shortcut: callee is the resource's defining
	// instance. Rep flows through unchanged — no callee-side entry,
	// NumBorrows not incremented.
	if tr != nil && tr.instance != nil && tt.owner == tr.instance {
		return &detachedResourceHandle{tr: tr, rep: rep, valid: true}, nil
	}

	// General cross-component path: allocate borrow on target.
	return tt.issueBorrow(tr, rep, task), nil
}

func (h *liveResourceHandle) TransferOwn(target TransferTarget) (ResourceHandle, error) {
	h.checkValid("TransferOwn")
	if target == nil {
		return nil, fmt.Errorf("TransferOwn: nil target")
	}
	e := &h.table.entries[h.slotIdx]
	if e.kind != kindOwn {
		return nil, fmt.Errorf("handle %d is borrow, not own", h.HandleID())
	}
	if e.numLends > 0 {
		return nil, fmt.Errorf("cannot remove owned resource while borrowed (handle %d, %d outstanding borrows)", h.HandleID(), e.numLends)
	}
	tr := e.tr
	rep := e.rep
	h.table.free(h.slotIdx)
	h.valid = false
	return target.IssueOwn(tr, rep), nil
}

func (h *liveResourceHandle) Drop(ctx context.Context) error {
	h.checkValid("Drop")
	e := &h.table.entries[h.slotIdx]
	switch e.kind {
	case kindBorrow:
		if e.borrowScope != nil {
			e.borrowScope.BorrowDropped()
		}
		h.table.free(h.slotIdx)
		h.valid = false
		return nil
	case kindOwn:
		if e.numLends > 0 {
			return fmt.Errorf("cannot drop owned resource while borrowed (handle %d, %d outstanding borrows)", h.HandleID(), e.numLends)
		}
		tr := e.tr
		rep := e.rep
		h.table.free(h.slotIdx)
		h.valid = false
		return runDtor(ctx, tr, rep, h.table.owner)
	default:
		return fmt.Errorf("unknown kind for handle %d", h.HandleID())
	}
}

// detachedResourceHandle is a minimal rep carrier returned by the
// same-component LendTo shortcut. Its only valid operations are
// HandleID/Rep/Type — visitors call HandleID() to write rep into the
// wasm slot. LendTo/TransferOwn/Drop/Detach are not meaningful here:
// the carrier is a transient artifact of the shortcut and never
// reaches user code through the canon API.
type detachedResourceHandle struct {
	tr  *TypeResource
	rep uint32
	// valid is preserved (always set true at construction) so external
	// invariant checks that test it remain stable.
	valid bool
}

func (h *detachedResourceHandle) HandleID() uint32             { return h.rep }
func (h *detachedResourceHandle) Rep() uint32                  { return h.rep }
func (h *detachedResourceHandle) Type() *TypeResource          { return h.tr }
func (h *detachedResourceHandle) Instance() *ComponentInstance { return nil }

func (h *detachedResourceHandle) LendTo(TransferTarget, *canon.Task) (ResourceHandle, error) {
	panic("wacogo/core: LendTo on detached handle (carrier)")
}
func (h *detachedResourceHandle) TransferOwn(TransferTarget) (ResourceHandle, error) {
	panic("wacogo/core: TransferOwn on detached handle (carrier)")
}
func (h *detachedResourceHandle) Drop(context.Context) error {
	panic("wacogo/core: Drop on detached handle (carrier)")
}

// dtorDepthKey is the context key for tracking nested dtor invocations.
type dtorDepthKey struct{}

// maxDtorDepth caps the depth of cross-instance dtor chains. Without this
// cap, two instances whose dtors reference each other's resources could
// deadlock during teardown (or recurse without bound in pathological host
// wiring).
const maxDtorDepth = 32

// runDtor invokes tr's destructor with the spec-required gating:
//   - nil tr or nil dtor: no-op.
//   - caller == defining: same-component shortcut; defining instance
//     is already entered, so invoke directly (Enter would deadlock).
//   - defining == nil: test stub fallback; invoke directly.
//   - else: Enter the defining instance, run dtor, defer Exit.
//
// A cumulative dtor-call depth carried on ctx caps recursion at
// maxDtorDepth; exceeding the cap returns an error rather than risking
// deadlock or stack overflow.
func runDtor(ctx context.Context, tr *TypeResource, rep uint32, caller *ComponentInstance) error {
	if tr == nil {
		return nil
	}
	dtor := tr.dtor
	if dtor == nil {
		return nil
	}
	depth, _ := ctx.Value(dtorDepthKey{}).(int)
	if depth >= maxDtorDepth {
		return fmt.Errorf("dtor recursion depth %d exceeds maximum %d", depth, maxDtorDepth)
	}
	ctx = context.WithValue(ctx, dtorDepthKey{}, depth+1)
	defining := tr.instance
	if caller == defining {
		return dtor(ctx, rep)
	}
	if defining == nil {
		return dtor(ctx, rep)
	}
	if err := defining.Enter(ctx); err != nil {
		return err
	}
	defer defining.Exit(ctx)
	return dtor(ctx, rep)
}

// Interface satisfaction assertions.
var _ ResourceHandle = (*liveResourceHandle)(nil)
var _ ResourceHandle = (*detachedResourceHandle)(nil)
