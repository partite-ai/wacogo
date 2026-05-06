package canon

// Test-only ResourceTable implementation. canon tests need a concrete
// canon.ResourceTable to seed handles, but canon cannot import core (would
// create a cycle), so this stub is a parallel implementation kept inside
// canon's test build only. Production uses *core.ResourceTable.

import (
	"context"
	"fmt"
)

const stubFreeListNone = int32(-1)

type stubResourceKind uint8

const (
	stubKindOwn    stubResourceKind = 1
	stubKindBorrow stubResourceKind = 2
)

type stubEntry struct {
	rep          uint32
	resourceType ResourceType
	kind         stubResourceKind
	occupied     bool
	numLends     uint32
	borrowScope  *Task
}

// stubResourceTable mirrors core.ResourceTable's semantics for canon tests.
type stubResourceTable struct {
	owner    Instance
	entries  []stubEntry
	freeList int32
}

func newStubResourceTable() *stubResourceTable {
	return &stubResourceTable{freeList: stubFreeListNone}
}

func newStubResourceTableFor(owner Instance) *stubResourceTable {
	return &stubResourceTable{owner: owner, freeList: stubFreeListNone}
}

func (t *stubResourceTable) alloc(rt ResourceType, rep uint32, kind stubResourceKind) uint32 {
	if t.freeList != stubFreeListNone {
		idx := uint32(t.freeList)
		t.freeList = int32(t.entries[idx].rep)
		t.entries[idx] = stubEntry{rep: rep, resourceType: rt, kind: kind, occupied: true}
		return idx
	}
	idx := uint32(len(t.entries))
	t.entries = append(t.entries, stubEntry{rep: rep, resourceType: rt, kind: kind, occupied: true})
	return idx
}

func (t *stubResourceTable) IssueOwn(rt ResourceType, rep uint32) ResourceHandle {
	idx := t.alloc(rt, rep, stubKindOwn)
	return &stubHandle{t: t, slotIdx: idx, valid: true}
}

func (t *stubResourceTable) IssueBorrow(rt ResourceType, rep uint32, scope *Task) ResourceHandle {
	idx := t.alloc(rt, rep, stubKindBorrow)
	t.entries[idx].borrowScope = scope
	scope.NumBorrows++
	return &stubHandle{t: t, slotIdx: idx, valid: true}
}

func (t *stubResourceTable) Owner() Instance { return t.owner }

func (t *stubResourceTable) LookupOwn(rt ResourceType, h uint32) (ResourceHandle, error) {
	idx, e, err := t.lookup(rt, h)
	if err != nil {
		return nil, err
	}
	if e.kind != stubKindOwn {
		return nil, fmt.Errorf("handle %d is borrow, not own", h)
	}
	return &stubHandle{t: t, slotIdx: idx, valid: true}, nil
}

func (t *stubResourceTable) LookupBorrowable(rt ResourceType, h uint32) (ResourceHandle, error) {
	idx, _, err := t.lookup(rt, h)
	if err != nil {
		return nil, err
	}
	return &stubHandle{t: t, slotIdx: idx, valid: true}, nil
}

func (t *stubResourceTable) lookup(rt ResourceType, handle uint32) (uint32, *stubEntry, error) {
	if handle == 0 {
		return 0, nil, fmt.Errorf("unknown handle index 0 (reserved)")
	}
	idx := handle - 1
	if idx >= uint32(len(t.entries)) || !t.entries[idx].occupied {
		return 0, nil, fmt.Errorf("unknown handle index %d", handle)
	}
	e := &t.entries[idx]
	if e.resourceType != rt {
		return 0, nil, fmt.Errorf("handle index %d has wrong type", handle)
	}
	return idx, e, nil
}

func (t *stubResourceTable) free(idx uint32) {
	t.entries[idx].occupied = false
	t.entries[idx].resourceType = nil
	t.entries[idx].kind = 0
	t.entries[idx].rep = uint32(t.freeList)
	t.freeList = int32(idx)
}

var _ ResourceTable = (*stubResourceTable)(nil)

// stubHandle satisfies ResourceHandle for the stub table.
type stubHandle struct {
	t       *stubResourceTable
	slotIdx uint32
	valid   bool
}

func (h *stubHandle) checkValid(op string) {
	if !h.valid {
		panic(fmt.Sprintf("stubHandle.%s on invalid handle", op))
	}
}

func (h *stubHandle) HandleID() uint32 {
	h.checkValid("HandleID")
	return h.slotIdx + 1
}

func (h *stubHandle) Rep() uint32 {
	h.checkValid("Rep")
	return h.t.entries[h.slotIdx].rep
}

func (h *stubHandle) Type() ResourceType {
	h.checkValid("Type")
	return h.t.entries[h.slotIdx].resourceType
}

func (h *stubHandle) LendTo(target ResourceTable, task *Task) (ResourceHandle, error) {
	h.checkValid("LendTo")
	e := &h.t.entries[h.slotIdx]
	rt := e.resourceType
	rep := e.rep

	if target == nil {
		return nil, fmt.Errorf("LendTo: nil target")
	}

	e.numLends++
	src := h
	task.AddRelease(func() {
		if src.valid && src.t.entries[src.slotIdx].numLends > 0 {
			src.t.entries[src.slotIdx].numLends--
		}
	})

	// Same-component shortcut: return a rep carrier whose HandleID()
	// is rep so the visitor can write it directly into the wasm slot.
	if target.Owner() != nil && target.Owner() == rt.DefiningInstance() {
		return &repCarrier{rep: rep}, nil
	}
	return target.IssueBorrow(rt, rep, task), nil
}

func (h *stubHandle) TransferOwn(target TransferTarget) (ResourceHandle, error) {
	h.checkValid("TransferOwn")
	e := &h.t.entries[h.slotIdx]
	if e.kind != stubKindOwn {
		return nil, fmt.Errorf("handle %d is borrow, not own", h.HandleID())
	}
	if e.numLends > 0 {
		return nil, fmt.Errorf("cannot transfer owned resource with outstanding borrows")
	}
	if target == nil {
		return nil, fmt.Errorf("TransferOwn: nil target")
	}
	rt := e.resourceType
	rep := e.rep
	h.t.free(h.slotIdx)
	h.valid = false
	return target.IssueOwn(rt, rep), nil
}

func (h *stubHandle) Drop(ctx context.Context) error {
	h.checkValid("Drop")
	e := &h.t.entries[h.slotIdx]
	switch e.kind {
	case stubKindBorrow:
		if e.borrowScope != nil {
			e.borrowScope.BorrowDropped()
		}
		h.t.free(h.slotIdx)
		h.valid = false
		return nil
	case stubKindOwn:
		if e.numLends > 0 {
			return fmt.Errorf("cannot drop owned resource while borrowed")
		}
		rt := e.resourceType
		rep := e.rep
		h.t.free(h.slotIdx)
		h.valid = false
		dtor := rt.Destructor()
		if dtor == nil {
			return nil
		}
		defining := rt.DefiningInstance()
		if defining == nil {
			return dtor(ctx, rep)
		}
		exit, err := defining.Enter(ctx)
		if err != nil {
			return err
		}
		defer exit(ctx)
		return dtor(ctx, rep)
	default:
		return fmt.Errorf("unknown kind")
	}
}

var _ ResourceHandle = (*stubHandle)(nil)

