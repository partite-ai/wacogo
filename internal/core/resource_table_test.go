package core

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo/internal/canon"
)

func TestResourceTable_IssueOwn_LookupOwn_RoundTrip(t *testing.T) {
	tbl := NewResourceTable(nil)
	tr := &TypeResource{}
	h := tbl.IssueOwn(tr, 42)
	if h.HandleID() == 0 {
		t.Fatal("IssueOwn returned 0 handle")
	}
	if got := h.Rep(); got != 42 {
		t.Fatalf("Rep = %d, want 42", got)
	}

	got, err := tbl.LookupOwn(tr, h.HandleID())
	if err != nil {
		t.Fatalf("LookupOwn err = %v", err)
	}
	if got.Rep() != 42 {
		t.Fatalf("looked-up Rep = %d, want 42", got.Rep())
	}
}

func TestResourceTable_LookupOwn_RejectsBorrow(t *testing.T) {
	tbl := NewResourceTable(nil)
	tr := &TypeResource{}
	var task canon.Task
	bh := tbl.issueBorrow(tr, 7, &task)

	if _, err := tbl.LookupOwn(tr, bh.HandleID()); err == nil {
		t.Fatal("LookupOwn accepted a borrow handle, want error")
	}
}

func TestResourceTable_LookupBorrowable_AcceptsBoth(t *testing.T) {
	tbl := NewResourceTable(nil)
	tr := &TypeResource{}
	var task canon.Task
	oh := tbl.IssueOwn(tr, 1)
	bh := tbl.issueBorrow(tr, 2, &task)

	if _, err := tbl.LookupBorrowable(tr, oh.HandleID()); err != nil {
		t.Fatalf("LookupBorrowable rejected own: %v", err)
	}
	if _, err := tbl.LookupBorrowable(tr, bh.HandleID()); err != nil {
		t.Fatalf("LookupBorrowable rejected borrow: %v", err)
	}
}

func TestResourceTable_Lookup_RejectsZeroAndUnknown(t *testing.T) {
	tbl := NewResourceTable(nil)
	tr := &TypeResource{}
	if _, err := tbl.LookupOwn(tr, 0); err == nil {
		t.Fatal("LookupOwn accepted handle 0")
	}
	if _, err := tbl.LookupOwn(tr, 42); err == nil {
		t.Fatal("LookupOwn accepted unknown handle")
	}
}

func TestResourceTable_Lookup_RejectsWrongType(t *testing.T) {
	tbl := NewResourceTable(nil)
	trA := &TypeResource{}
	trB := &TypeResource{}
	h := tbl.IssueOwn(trA, 1)
	if _, err := tbl.LookupOwn(trB, h.HandleID()); err == nil {
		t.Fatal("LookupOwn accepted wrong resource type")
	}
}

func TestResourceTable_ReusesFreeSlots(t *testing.T) {
	tbl := NewResourceTable(nil)
	tr := &TypeResource{}
	h1 := tbl.IssueOwn(tr, 1)
	id1 := h1.HandleID()
	if err := h1.Drop(context.Background()); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	h2 := tbl.IssueOwn(tr, 2)
	id2 := h2.HandleID()
	if id1 != id2 {
		t.Errorf("expected free-list reuse: h1=%d, h2=%d", id1, id2)
	}
}

func TestResourceTable_LendOnBorrowSourceSucceeds(t *testing.T) {
	tbl := NewResourceTable(nil)
	callee := NewResourceTable(nil)
	tr := &TypeResource{}

	src := tbl.IssueOwn(tr, 42)
	var task canon.Task
	bh, err := src.LendTo(callee, &task)
	if err != nil {
		t.Fatalf("LendTo on own: %v", err)
	}
	if err := bh.Drop(context.Background()); err != nil {
		t.Fatalf("Drop borrow: %v", err)
	}
	if task.NumBorrows != 0 {
		t.Fatalf("NumBorrows after borrow drop: got %d, want 0", task.NumBorrows)
	}
	if err := task.End(); err != nil {
		t.Fatalf("task.End: %v", err)
	}
	// After End the unlend release fires, so TransferOwn must succeed.
	dst, err := src.TransferOwn(NewResourceTable(nil))
	if err != nil {
		t.Fatalf("TransferOwn after End: %v", err)
	}
	if dst.Rep() != 42 {
		t.Errorf("expected rep 42, got %d", dst.Rep())
	}
}
