package core

// Verifies destructor orchestration via the core ResourceHandle.Drop method.

import (
	"context"
	"errors"
	"testing"
)

// newInstanceForDropTest creates a *ComponentInstance suitable for use as a
// defining instance in drop tests. The instance's Enter/CanLeave semantics are
// tested via the real implementation.
func newInstanceForDropTest(t *testing.T) *ComponentInstance {
	t.Helper()
	ctx := context.Background()
	e := NewEngine(ctx)
	t.Cleanup(func() { _ = e.Close(ctx) })
	inst, err := NewInstance(e, &InstanceSpec{})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	return inst
}

func TestResourceDrop_OwnNoDtor(t *testing.T) {
	tbl := NewResourceTable(nil)
	tr := &TypeResource{}
	h := tbl.IssueOwn(tr, 99)
	if err := h.Drop(context.Background()); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	// Slot must be free: a follow-up LookupOwn should fail.
	if _, err := tbl.LookupOwn(tr, 1); err == nil {
		t.Errorf("LookupOwn on dropped handle should error")
	}
}

func TestResourceDrop_OwnSameComponentShortcut(t *testing.T) {
	defining := newInstanceForDropTest(t)
	called := false
	var gotRep uint32
	tr := &TypeResource{
		instance: defining,
		dtor: func(_ context.Context, rep uint32) error {
			called = true
			gotRep = rep
			return nil
		},
	}
	tbl := NewResourceTable(defining)
	h := tbl.IssueOwn(tr, 42)

	// Drop from the defining instance itself (same-component shortcut):
	// dtor should run even while the instance is entered.
	if err := defining.RunInComponent(context.Background(), func() error {
		return h.Drop(context.Background())
	}); err != nil {
		t.Fatalf("Drop in component: %v", err)
	}
	if !called {
		t.Error("dtor was not called")
	}
	if gotRep != 42 {
		t.Errorf("dtor rep: got %d, want 42", gotRep)
	}
}

func TestResourceDrop_OwnCrossComponentEntersDefining(t *testing.T) {
	defining := newInstanceForDropTest(t)
	enteredAtCall := false
	tr := &TypeResource{
		instance: defining,
		dtor: func(_ context.Context, _ uint32) error {
			enteredAtCall = defining.entered
			return nil
		},
	}
	tbl := NewResourceTable(nil)
	h := tbl.IssueOwn(tr, 7)

	if err := h.Drop(context.Background()); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	if !enteredAtCall {
		t.Error("expected defining instance to be entered during dtor")
	}
	if defining.entered {
		t.Error("expected defining instance to be exited after Drop")
	}
}

func TestResourceDrop_OwnCrossComponentReentranceError(t *testing.T) {
	defining := newInstanceForDropTest(t)
	called := false
	tr := &TypeResource{
		instance: defining,
		dtor: func(_ context.Context, _ uint32) error {
			called = true
			return nil
		},
	}
	tbl := NewResourceTable(nil)
	h := tbl.IssueOwn(tr, 1)

	// Defining instance is already entered; a cross-component Drop must
	// surface a reentrance error and skip the dtor.
	defining.entered = true
	defer func() { defining.entered = false }()
	err := h.Drop(context.Background())
	if err == nil {
		t.Fatal("expected reentrance error, got nil")
	}
	if called {
		t.Error("dtor must NOT run when defining instance is already entered")
	}
}

func TestResourceDrop_OwnCrossComponentDtorErrorPropagates(t *testing.T) {
	defining := newInstanceForDropTest(t)
	sentinel := errors.New("dtor blew up")
	tr := &TypeResource{
		instance: defining,
		dtor:     func(_ context.Context, _ uint32) error { return sentinel },
	}
	tbl := NewResourceTable(nil)
	h := tbl.IssueOwn(tr, 1)

	err := h.Drop(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
	if defining.entered {
		t.Error("defining instance must be exited even when dtor errors")
	}
}
