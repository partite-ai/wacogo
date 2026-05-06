package core

import (
	"fmt"

	"github.com/partite-ai/wacogo/internal/canon"
)

const freeListNone = int32(-1)

// TransferTarget is the destination side of a resource handle
// transfer/lend operation. The marker method keeps it package-private:
// external callers receive a TransferTarget from CallContext.TransferTarget()
// and can pass it to LendTo/TransferOwn but cannot manipulate the
// underlying state directly. Production: *ResourceTable. The canon
// bridge supplies a private shim implementation when transferring
// into a canon-side target (e.g. *canon.ValOwnHandle).
type TransferTarget interface {
	transferTarget()
	// IssueOwn places (tr, rep) in the target and returns the resulting
	// handle. liveResourceHandle.TransferOwn invokes this after freeing
	// its source slot.
	IssueOwn(tr *TypeResource, rep uint32) ResourceHandle
}

func (*ResourceTable) transferTarget() {}

// Compile-time assertion that *ResourceTable satisfies TransferTarget.
// (IssueOwn already has the required signature.)
var _ TransferTarget = (*ResourceTable)(nil)

type resourceKind uint8

const (
	kindOwn    resourceKind = 1
	kindBorrow resourceKind = 2
)

// ResourceTable manages canonical-ABI resource handles for a component
// instance. Handles are 1-indexed; 0 is reserved as "invalid".
//
// Not safe for concurrent use; callers must ensure single-threaded access.
type ResourceTable struct {
	owner    *ComponentInstance
	entries  []resourceEntry
	freeList int32
}

type resourceEntry struct {
	rep      uint32
	tr       *TypeResource
	kind     resourceKind
	occupied bool
	numLends uint32
	borrowScope *canon.Task
}

// NewResourceTable creates an empty resource table owned by owner. owner is
// the component instance that owns this table — used by LendTo to detect
// the same-component borrow shortcut. May be nil for tests.
func NewResourceTable(owner *ComponentInstance) *ResourceTable {
	return &ResourceTable{owner: owner, freeList: freeListNone}
}

// IssueOwn allocates a fresh own-kind handle.
func (t *ResourceTable) IssueOwn(tr *TypeResource, rep uint32) ResourceHandle {
	idx := t.alloc(tr, rep, kindOwn)
	return &liveResourceHandle{table: t, slotIdx: idx, valid: true}
}

// issueBorrow allocates a fresh borrow-kind handle scoped to scope.
// Unexported: only LendTo reaches this.
func (t *ResourceTable) issueBorrow(tr *TypeResource, rep uint32, scope *canon.Task) ResourceHandle {
	idx := t.alloc(tr, rep, kindBorrow)
	t.entries[idx].borrowScope = scope
	scope.BorrowIssued()
	return &liveResourceHandle{table: t, slotIdx: idx, valid: true}
}

func (t *ResourceTable) alloc(tr *TypeResource, rep uint32, kind resourceKind) uint32 {
	if t.freeList != freeListNone {
		idx := uint32(t.freeList)
		t.freeList = int32(t.entries[idx].rep)
		t.entries[idx] = resourceEntry{rep: rep, tr: tr, kind: kind, occupied: true}
		return idx
	}
	idx := uint32(len(t.entries))
	t.entries = append(t.entries, resourceEntry{rep: rep, tr: tr, kind: kind, occupied: true})
	return idx
}

// LookupOwn finds an own-kind entry. Errors on wrong kind, wrong type, or unknown handle.
func (t *ResourceTable) LookupOwn(tr *TypeResource, h uint32) (ResourceHandle, error) {
	idx, e, err := t.lookup(tr, h)
	if err != nil {
		return nil, err
	}
	if e.kind != kindOwn {
		return nil, fmt.Errorf("handle %d is borrow, not own", h)
	}
	return &liveResourceHandle{table: t, slotIdx: idx, valid: true}, nil
}

// LookupBorrowable finds an entry of either kind.
func (t *ResourceTable) LookupBorrowable(tr *TypeResource, h uint32) (ResourceHandle, error) {
	idx, _, err := t.lookup(tr, h)
	if err != nil {
		return nil, err
	}
	return &liveResourceHandle{table: t, slotIdx: idx, valid: true}, nil
}

func (t *ResourceTable) lookup(tr *TypeResource, handle uint32) (uint32, *resourceEntry, error) {
	if handle == 0 {
		return 0, nil, fmt.Errorf("unknown handle index 0 (reserved)")
	}
	idx := handle - 1
	if idx >= uint32(len(t.entries)) || !t.entries[idx].occupied {
		return 0, nil, fmt.Errorf("unknown handle index %d", handle)
	}
	e := &t.entries[idx]
	if e.tr != tr {
		return 0, nil, fmt.Errorf("handle index %d used with the wrong type, expected guest-defined resource but found a different guest-defined resource", handle)
	}
	return idx, e, nil
}

func (t *ResourceTable) free(idx uint32) {
	t.entries[idx].occupied = false
	t.entries[idx].tr = nil
	t.entries[idx].kind = 0
	t.entries[idx].rep = uint32(t.freeList)
	t.freeList = int32(idx)
}
