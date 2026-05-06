package canon

import (
	"context"
	"strings"
	"testing"
)

// liftIntoVal mints a *ValOwnHandle by transferring an own out of src
// into the val. Mirrors the lift visitor's call sequence so canon-side
// tests can exercise *ValOwnHandle directly without going through the
// visitor pipeline.
func liftIntoVal(t *testing.T, src ResourceHandle) *ValOwnHandle {
	t.Helper()
	val := newValOwnHandleEmpty()
	if _, err := src.TransferOwn(val); err != nil {
		t.Fatalf("TransferOwn into val: %v", err)
	}
	return val
}

func TestValOwnHandle_Drop(t *testing.T) {
	rt := &testResourceType{name: "R"}
	src := newStubResourceTable()
	live := src.IssueOwn(rt, 5)
	val := liftIntoVal(t, live)

	if err := val.Drop(context.Background()); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	// Idempotent.
	if err := val.Drop(context.Background()); err != nil {
		t.Fatalf("second Drop: %v", err)
	}
}

func TestValOwnHandle_DropAfterTransferErrors(t *testing.T) {
	rt := &testResourceType{name: "R"}
	src := newStubResourceTable()
	live := src.IssueOwn(rt, 5)
	val := liftIntoVal(t, live)

	// Transfer the val out (simulating a successful lower).
	dst := newStubResourceTable()
	if _, err := val.TransferOwn(dst); err != nil {
		t.Fatalf("TransferOwn: %v", err)
	}
	err := val.Drop(context.Background())
	if err == nil || !strings.Contains(err.Error(), "transferred") {
		t.Fatalf("Drop after transfer err = %v, want transferred", err)
	}
}

func TestValOwnHandle_DropOnEmptyIsNoOp(t *testing.T) {
	var v ValOwnHandle
	if err := v.Drop(context.Background()); err != nil {
		t.Fatalf("Drop on empty: %v", err)
	}
}

// --- state machine tests ---

func TestValOwnHandle_TransferOwnIntoTable(t *testing.T) {
	rt := &testResourceType{name: "R"}
	src := newStubResourceTable()
	live := src.IssueOwn(rt, 7)
	val := liftIntoVal(t, live)

	dst := newStubResourceTable()
	h, err := val.TransferOwn(dst)
	if err != nil {
		t.Fatalf("TransferOwn: %v", err)
	}
	if h.Rep() != 7 {
		t.Fatalf("rep = %d, want 7", h.Rep())
	}
}

func TestValOwnHandle_TransferOwnAfterTransferErrors(t *testing.T) {
	rt := &testResourceType{name: "R"}
	src := newStubResourceTable()
	live := src.IssueOwn(rt, 7)
	val := liftIntoVal(t, live)

	dst := newStubResourceTable()
	if _, err := val.TransferOwn(dst); err != nil {
		t.Fatalf("first TransferOwn: %v", err)
	}
	_, err := val.TransferOwn(dst)
	if err == nil || !strings.Contains(err.Error(), "transferred") {
		t.Fatalf("second TransferOwn err = %v, want transferred", err)
	}
}

func TestValOwnHandle_LendToCrossComponent(t *testing.T) {
	rt := &testResourceType{name: "R"}
	src := newStubResourceTable()
	live := src.IssueOwn(rt, 11)
	val := liftIntoVal(t, live)

	dst := newStubResourceTable()
	task := &Task{}
	borrow, err := val.LendTo(dst, task)
	if err != nil {
		t.Fatalf("LendTo: %v", err)
	}
	if val.numLends != 1 {
		t.Fatalf("numLends = %d, want 1", val.numLends)
	}
	if borrow.Rep() != 11 {
		t.Fatalf("borrow rep = %d, want 11", borrow.Rep())
	}
	if task.NumBorrows != 1 {
		t.Fatalf("task.NumBorrows = %d, want 1", task.NumBorrows)
	}

	if err := borrow.Drop(context.Background()); err != nil {
		t.Fatalf("borrow.Drop: %v", err)
	}
	if val.numLends != 1 {
		t.Fatalf("numLends after borrow drop = %d, want 1 (decrement waits for task end)", val.numLends)
	}
	if err := task.End(); err != nil {
		t.Fatalf("task.End: %v", err)
	}
	if val.numLends != 0 {
		t.Fatalf("numLends after task end = %d, want 0", val.numLends)
	}
}

func TestValOwnHandle_TransferOwnWhileLentErrors(t *testing.T) {
	rt := &testResourceType{name: "R"}
	src := newStubResourceTable()
	live := src.IssueOwn(rt, 11)
	val := liftIntoVal(t, live)

	dst := newStubResourceTable()
	task := &Task{}
	if _, err := val.LendTo(dst, task); err != nil {
		t.Fatalf("LendTo: %v", err)
	}
	_, err := val.TransferOwn(dst)
	if err == nil || !strings.Contains(err.Error(), "outstanding") {
		t.Fatalf("err = %v, want outstanding-borrows", err)
	}
}

func TestValOwnHandle_DropRunsDtor(t *testing.T) {
	called := false
	rt := &testResourceTypeWithDtor{
		name: "R",
		dtor: func(_ context.Context, rep uint32) error {
			called = true
			if rep != 99 {
				t.Errorf("dtor rep = %d, want 99", rep)
			}
			return nil
		},
	}
	src := newStubResourceTable()
	live := src.IssueOwn(rt, 99)
	val := liftIntoVal(t, live)

	if err := val.Drop(context.Background()); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	if !called {
		t.Error("dtor not called")
	}
}

// testResourceTypeWithDtor is a testResourceType that carries a dtor
// closure (the bare testResourceType's Destructor returns nil).
type testResourceTypeWithDtor struct {
	name string
	dtor func(context.Context, uint32) error
}

func (*testResourceTypeWithDtor) IsResourceType()            {}
func (*testResourceTypeWithDtor) DefiningInstance() Instance { return nil }
func (r *testResourceTypeWithDtor) Destructor() func(context.Context, uint32) error {
	return r.dtor
}
