package host_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
)

// orphan implements host.DestroyOrphan and records when it was called.
type orphan struct {
	id           int
	destroyedAt  atomic.Int32
	destroySeqCh chan int
	destroyError error
}

func (o *orphan) DestroyOrphan(_ context.Context) error {
	if o.destroySeqCh != nil {
		o.destroySeqCh <- o.id
	}
	return o.destroyError
}

// TestDrain_DestroyOrphanCalledInLIFOOrder verifies that Close walks
// extern entries in reverse insertion order and invokes DestroyOrphan
// on each, in preference to the registered Drop.
func TestDrain_DestroyOrphanCalledInLIFOOrder(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("orphan-host")
	b.AddFunction("ping", &host.FuncType{}, func(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error { return nil })
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}

	seq := make(chan int, 3)
	a := &orphan{id: 1, destroySeqCh: seq}
	b2 := &orphan{id: 2, destroySeqCh: seq}
	c := &orphan{id: 3, destroySeqCh: seq}
	inst.RegisterResource(a)
	inst.RegisterResource(b2)
	inst.RegisterResource(c)

	if err := inst.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	close(seq)
	var order []int
	for id := range seq {
		order = append(order, id)
	}
	// LIFO: c (3) → b2 (2) → a (1).
	want := []int{3, 2, 1}
	if len(order) != len(want) {
		t.Fatalf("DestroyOrphan call count = %d, want %d (sequence %v)", len(order), len(want), order)
	}
	for i, id := range want {
		if order[i] != id {
			t.Fatalf("DestroyOrphan order[%d] = %d, want %d (full %v)", i, order[i], id, order)
		}
	}
}

// TestDrain_FallsBackToDropInterface verifies that when an extern obj
// implements Drop but not DestroyOrphan, drain calls obj.Drop(ctx).
func TestDrain_FallsBackToDropInterface(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("drop-iface-host")
	b.AddFunction("ping", &host.FuncType{}, func(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error { return nil })

	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}

	d := &droppable{tag: "mine"}
	inst.RegisterResource(d)

	if err := inst.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if d.dropCount.Load() != 1 {
		t.Fatalf("Drop called %d times, want 1", d.dropCount.Load())
	}
}

// TestDrain_DestroyOrphanPreferredOverDrop verifies that when an obj
// implements both DestroyOrphan and Drop, only DestroyOrphan runs.
func TestDrain_DestroyOrphanPreferredOverDrop(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("preference-host")
	b.AddFunction("ping", &host.FuncType{}, func(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error { return nil })

	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}

	o := &orphanAndDrop{}
	inst.RegisterResource(o)

	if err := inst.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if o.destroyCount.Load() != 1 {
		t.Fatalf("DestroyOrphan called %d times, want 1", o.destroyCount.Load())
	}
	if o.dropCount.Load() != 0 {
		t.Fatalf("Drop called %d times, want 0 (DestroyOrphan should win)", o.dropCount.Load())
	}
}

// TestDrain_OrphanErrorJoined verifies that errors from DestroyOrphan
// are joined into Close's return value and don't stop iteration.
func TestDrain_OrphanErrorJoined(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("orphan-err-host")
	b.AddFunction("ping", &host.FuncType{}, func(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error { return nil })
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}

	errA := errors.New("orphan-a-failed")
	errB := errors.New("orphan-b-failed")
	inst.RegisterResource(&orphan{id: 1, destroyError: errA})
	inst.RegisterResource(&orphan{id: 2, destroyError: errB})

	err = inst.Close(ctx)
	if err == nil {
		t.Fatal("Close: expected joined error, got nil")
	}
	if !errors.Is(err, errA) || !errors.Is(err, errB) {
		t.Fatalf("Close err = %q, want to wrap both %v and %v", err.Error(), errA, errB)
	}
}

// droppable implements host.Drop but not host.DestroyOrphan.
type droppable struct {
	tag       string
	dropCount atomic.Int32
}

func (d *droppable) Drop(_ context.Context) error { d.dropCount.Add(1); return nil }

// orphanAndDrop implements both host.DestroyOrphan and host.Drop.
type orphanAndDrop struct {
	destroyCount atomic.Int32
	dropCount    atomic.Int32
}

func (o *orphanAndDrop) DestroyOrphan(_ context.Context) error {
	o.destroyCount.Add(1)
	return nil
}
func (o *orphanAndDrop) Drop(_ context.Context) error { o.dropCount.Add(1); return nil }
