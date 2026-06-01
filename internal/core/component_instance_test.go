package core

import (
	"context"
	"testing"
)

func TestComponentInstance_EnterReentranceTrap(t *testing.T) {
	inst := &ComponentInstance{}
	if err := inst.Enter(context.Background()); err != nil {
		t.Fatalf("first Enter err = %v", err)
	}
	defer inst.Exit(context.Background())
	if err := inst.Enter(context.Background()); err == nil {
		t.Fatal("second Enter returned nil err, want reentrance trap")
	}
}

func TestComponentInstance_RunInComponentReleasesOnReturn(t *testing.T) {
	inst := &ComponentInstance{}
	ctx := context.Background()
	if err := inst.RunInComponent(ctx, func() error { return nil }); err != nil {
		t.Fatalf("RunInComponent err = %v", err)
	}
	if err := inst.RunInComponent(ctx, func() error { return nil }); err != nil {
		t.Fatalf("second RunInComponent err = %v", err)
	}
}

func TestComponentInstance_CanLeaveDefault(t *testing.T) {
	inst := &ComponentInstance{canLeave: true}
	if !inst.CanLeave() {
		t.Fatal("CanLeave = false, want true")
	}
}

func TestComponentInstance_NilSafety(t *testing.T) {
	var i *ComponentInstance
	if !i.CanLeave() {
		t.Fatal("nil CanLeave = false, want true")
	}
	if err := i.Enter(context.Background()); err != nil {
		t.Fatalf("nil Enter err = %v", err)
	}
	i.Exit(context.Background())
	// SuspendLeave/RestoreLeave on a nil receiver must be no-ops —
	// runners invoke them unconditionally.
	prev := i.SuspendLeave()
	i.RestoreLeave(prev)
}

func TestComponentInstance_SuspendLeaveStacks(t *testing.T) {
	inst := &ComponentInstance{canLeave: true}
	if !inst.CanLeave() {
		t.Fatal("precondition: CanLeave = false")
	}
	r1 := inst.SuspendLeave()
	if inst.CanLeave() {
		t.Fatal("after SuspendLeave: CanLeave = true, want false")
	}
	r2 := inst.SuspendLeave()
	if inst.CanLeave() {
		t.Fatal("nested SuspendLeave: CanLeave = true, want false")
	}
	inst.RestoreLeave(r2)
	if inst.CanLeave() {
		t.Fatal("after inner restore: CanLeave = true, want false (outer still suspended)")
	}
	inst.RestoreLeave(r1)
	if !inst.CanLeave() {
		t.Fatal("after outer restore: CanLeave = false, want true")
	}
}
