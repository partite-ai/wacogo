package host_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
)

// TestClose_DropsOwnedHandlesViaRegisteredDtor verifies that closing
// an instance with outstanding own-kind canon handles invokes the
// registered Drop (userDtor) for each, before the wazero runtime tears
// down — exercising the close-time drop pass added to core.Close.
func TestClose_DropsOwnedHandlesViaRegisteredDtor(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	dropCount := atomic.Int32{}
	dropOrder := make(chan int, 8)

	b := e.NewHostBuilder("close-drop-host")
	rt := b.AddResource("r", func(_ context.Context, _ *host.ComponentInstance, obj any) error {
		dropCount.Add(1)
		dropOrder <- obj.(*tagged).id
		return nil
	})
	_ = rt

	// stash mints an own<r> but never returns it, so the canon slot
	// stays live past the host fn return.
	b.AddFunction("stash", &host.FuncType{
		Params: []host.Param{{Name: "id", Type: host.S32}},
	}, func(_ context.Context, cc *core.CallContext, h *host.ComponentInstance, stack []uint64) error {
		obj := &tagged{id: int(int32(stack[0]))}
		eh := h.RegisterResource(obj)
		tr := h.Core().ExportedType("r").(*core.TypeResource)
		_ = cc.IssueOwnHandle(tr, uint32(eh))
		return nil
	})

	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}

	// Stash three handles.
	for _, id := range []int32{1, 2, 3} {
		if _, err := inst.Core().ExportedFunc("stash").Call(ctx, core.ValS32(id)); err != nil {
			t.Fatalf("stash(%d): %v", id, err)
		}
	}

	// Closing fires the registered userDtor for each outstanding own
	// handle, in LIFO order.
	if err := inst.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := dropCount.Load(); got != 3 {
		t.Fatalf("dropCount = %d, want 3", got)
	}
	close(dropOrder)
	var got []int
	for id := range dropOrder {
		got = append(got, id)
	}
	want := []int{3, 2, 1}
	if len(got) != len(want) {
		t.Fatalf("order len = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i, id := range want {
		if got[i] != id {
			t.Fatalf("dtor order[%d] = %d, want %d (full %v)", i, got[i], id, got)
		}
	}
}

type tagged struct{ id int }
