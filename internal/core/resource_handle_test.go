package core

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo/internal/canon"
)

// stubInstance is a test-only canon.Instance whose pointer-equality is
// the only thing used. The Enter/CanLeave methods are no-ops.
type stubInstance struct{ name string }

func (*stubInstance) Enter(ctx context.Context) (func(context.Context), error) {
	return func(context.Context) {}, nil
}
func (*stubInstance) CanLeave() bool                     { return true }
func (*stubInstance) SuspendLeave() func()               { return func() {} }
func (*stubInstance) ResourceTable() canon.ResourceTable { return nil }

func TestLiveHandle_Drop_OwnRunsDtor(t *testing.T) {
	calls := 0
	tr := &TypeResource{
		dtor: func(_ context.Context, rep uint32) error {
			calls++
			if rep != 99 {
				t.Errorf("dtor rep = %d, want 99", rep)
			}
			return nil
		},
	}
	tbl := NewResourceTable(nil)
	h := tbl.IssueOwn(tr, 99)
	if err := h.Drop(context.Background()); err != nil {
		t.Fatalf("Drop err = %v", err)
	}
	if calls != 1 {
		t.Fatalf("dtor calls = %d, want 1", calls)
	}
}

func TestLiveHandle_Drop_BorrowSkipsDtor(t *testing.T) {
	calls := 0
	tr := &TypeResource{
		dtor: func(_ context.Context, _ uint32) error { calls++; return nil },
	}
	tbl := NewResourceTable(nil)
	var task canon.Task
	h := tbl.issueBorrow(tr, 1, &task)
	if err := h.Drop(context.Background()); err != nil {
		t.Fatalf("Drop err = %v", err)
	}
	if calls != 0 {
		t.Fatalf("dtor called %d times, want 0 for borrow drop", calls)
	}
	if task.NumBorrows != 0 {
		t.Fatalf("task.NumBorrows = %d after borrow drop, want 0", task.NumBorrows)
	}
}

func TestLiveHandle_TransferOwn_InvalidatesSource(t *testing.T) {
	tbl := NewResourceTable(nil)
	tr := &TypeResource{}
	src := tbl.IssueOwn(tr, 7)
	dst, err := src.TransferOwn(NewResourceTable(nil))
	if err != nil {
		t.Fatalf("TransferOwn err = %v", err)
	}
	if dst.Rep() != 7 {
		t.Fatalf("dst Rep = %d, want 7", dst.Rep())
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on use of invalidated source handle")
		}
	}()
	_ = src.Rep()
}

func TestLiveHandle_LendTo_RegistersRelease(t *testing.T) {
	caller := NewResourceTable(nil)
	callee := NewResourceTable(nil)
	tr := &TypeResource{}
	src := caller.IssueOwn(tr, 11)

	var task canon.Task
	dst, err := src.LendTo(callee, &task)
	if err != nil {
		t.Fatalf("LendTo err = %v", err)
	}
	if dst.Rep() != 11 {
		t.Fatalf("dst Rep = %d, want 11", dst.Rep())
	}
	if task.NumBorrows != 1 {
		t.Fatalf("task.NumBorrows = %d after LendTo, want 1", task.NumBorrows)
	}
	if err := dst.Drop(context.Background()); err != nil {
		t.Fatalf("dst.Drop err = %v", err)
	}
	if task.NumBorrows != 0 {
		t.Fatalf("task.NumBorrows = %d after dst.Drop, want 0", task.NumBorrows)
	}
	if err := task.End(); err != nil {
		t.Fatalf("task.End err = %v", err)
	}
	if got := caller.entries[0].numLends; got != 0 {
		t.Fatalf("caller numLends = %d after End, want 0", got)
	}
}

func TestLiveHandle_LendTo_SameComponentShortcut(t *testing.T) {
	// The same-component shortcut fires when tt.owner == tr.instance.
	// Both must be the same *ComponentInstance.
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)
	inst, err := NewInstance(e, &InstanceSpec{})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	tr := &TypeResource{instance: inst}

	caller := NewResourceTable(inst)
	callee := NewResourceTable(inst)

	src := caller.IssueOwn(tr, 13)
	var task canon.Task
	dst, err := src.LendTo(callee, &task)
	if err != nil {
		t.Fatalf("LendTo err = %v", err)
	}
	if dst.HandleID() != 13 {
		t.Fatalf("shortcut HandleID = %d, want 13 (rep passthrough)", dst.HandleID())
	}
	if task.NumBorrows != 0 {
		t.Fatalf("task.NumBorrows = %d in same-component shortcut, want 0", task.NumBorrows)
	}
	if err := task.End(); err != nil {
		t.Fatalf("task.End err = %v", err)
	}
	if got := caller.entries[0].numLends; got != 0 {
		t.Fatalf("caller numLends = %d in shortcut, want 0", got)
	}
}

func TestLiveHandle_UseAfterDropPanics(t *testing.T) {
	tbl := NewResourceTable(nil)
	h := tbl.IssueOwn(&TypeResource{}, 1)
	if err := h.Drop(context.Background()); err != nil {
		t.Fatalf("Drop err = %v", err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on use after Drop")
		}
	}()
	_ = h.Rep()
}

func TestLiveHandle_TransferOwn_RejectsBorrow(t *testing.T) {
	tbl := NewResourceTable(nil)
	var task canon.Task
	bh := tbl.issueBorrow(&TypeResource{}, 1, &task)
	if _, err := bh.TransferOwn(NewResourceTable(nil)); err == nil {
		t.Fatal("TransferOwn accepted borrow handle, want error")
	}
}

func TestLiveHandle_TransferOwn_RejectsLent(t *testing.T) {
	caller := NewResourceTable(nil)
	callee := NewResourceTable(nil)
	tr := &TypeResource{}
	src := caller.IssueOwn(tr, 1)
	var task canon.Task
	if _, err := src.LendTo(callee, &task); err != nil {
		t.Fatalf("LendTo err = %v", err)
	}
	if _, err := src.TransferOwn(NewResourceTable(nil)); err == nil {
		t.Fatal("TransferOwn allowed transfer with outstanding borrow, want error")
	}
}

func TestResourceHandle_InstanceAndType(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)
	tr := NewTypeResource(nil)
	inst, err := NewInstance(e, &InstanceSpec{
		BuildExports: func(*ComponentInstance) ([]InstanceTypeSlot, []InstanceExport, error) {
			return []InstanceTypeSlot{{Name: "r", Type: tr}}, nil, nil
		},
	})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	rh := inst.resources.IssueOwn(tr, 99)

	if got := rh.Instance(); got != inst {
		t.Fatalf("rh.Instance() = %p, want %p", got, inst)
	}
	if got := rh.Type(); got != tr {
		t.Fatalf("rh.Type() = %p, want %p", got, tr)
	}
	if got := rh.Type().Instance(); got != inst {
		t.Fatalf("rh.Type().Instance() = %p, want %p", got, inst)
	}
}
