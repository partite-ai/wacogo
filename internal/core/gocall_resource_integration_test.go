package core

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo/internal/canon"
)

// gocallTestSetup wires up a *ComponentInstance with a single resource
// type whose dtor records calls. Returns the instance, the *TypeResource,
// and a counter the dtor increments.
func gocallTestSetup(t *testing.T) (*ComponentInstance, *TypeResource, *int) {
	t.Helper()
	ctx := context.Background()
	e := NewEngine(ctx)
	t.Cleanup(func() { _ = e.Close(ctx) })
	inst, err := NewInstance(e, &InstanceSpec{})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	calls := new(int)
	tr := &TypeResource{
		instance: inst,
		dtor: func(_ context.Context, _ uint32) error {
			*calls++
			return nil
		},
	}
	return inst, tr, calls
}

// TestGocallOwnLiftAndDrop simulates the visitor lift path against a
// real instance's resource table, then drops the lifted Val and
// verifies the destructor runs exactly once under defining-instance
// Enter gating.
func TestGocallOwnLiftAndDrop(t *testing.T) {
	inst, tr, calls := gocallTestSetup(t)
	tbl := inst.resources

	srcH := tbl.IssueOwn(tr, 99)
	wire := srcH.HandleID()

	src, err := tbl.LookupOwn(tr, wire)
	if err != nil {
		t.Fatalf("LookupOwn: %v", err)
	}
	// Lift via the new pattern: transfer into a *canon.ValOwnHandle.
	// canonResourceHandleView wraps the core handle as a canon
	// ResourceHandle so we can call TransferOwn(val) on it.
	val := &canon.ValOwnHandle{}
	if _, err := (canonResourceHandleView{h: src}).TransferOwn(val); err != nil {
		t.Fatalf("TransferOwn into val: %v", err)
	}

	if err := val.Drop(context.Background()); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	if *calls != 1 {
		t.Fatalf("dtor calls = %d, want 1", *calls)
	}
	if inst.entered {
		t.Error("defining instance still entered after Drop")
	}

	// Idempotent.
	if err := val.Drop(context.Background()); err != nil {
		t.Fatalf("second Drop: %v", err)
	}
	if *calls != 1 {
		t.Fatalf("dtor calls after second Drop = %d, want still 1", *calls)
	}

	// Source slot must be free in the original callee table.
	if _, err := tbl.LookupOwn(tr, wire); err == nil {
		t.Error("source handle still in table after lift+drop")
	}
}
