package core_test

import (
	"context"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
)

// Round-trip a u32 add(a, b) -> a+b host-implemented function via
// CallRaw, bypassing Val construction.
func TestExportedFuncCallRaw_PrimitiveRoundTrip(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("addhost")
	b.AddFunction("add", &host.FuncType{
		Params:  []host.Param{{Name: "a", Type: host.U32}, {Name: "b", Type: host.U32}},
		Results: []host.ResultDecl{{Name: "", Type: host.U32}},
	}, func(_ context.Context, _ *core.CallContext, _ *host.ComponentInstance, stack []uint64) error {
		stack[0] = stack[0] + stack[1]
		return nil
	})
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	fn := inst.Core().ExportedFunc("add")
	if fn == nil {
		t.Fatal("missing add export")
	}

	var got uint32
	err = fn.CallRaw(ctx, inst.Core(),
		func(_, _ *core.CallContext, stack []uint64) { stack[0] = 7; stack[1] = 35 },
		func(_, _ *core.CallContext, stack []uint64) { got = uint32(stack[0]) },
	)
	if err != nil {
		t.Fatalf("CallRaw: %v", err)
	}
	if got != 42 {
		t.Fatalf("got %d, want 42", got)
	}
}

func TestExportedFuncCallRaw_TrapPropagated(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("traphost")
	b.AddFunction("trap", &host.FuncType{
		Params:  nil,
		Results: nil,
	}, func(_ context.Context, _ *core.CallContext, _ *host.ComponentInstance, _ []uint64) error {
		panic("intentional trap")
	})
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	err = inst.Core().ExportedFunc("trap").CallRaw(ctx, inst.Core(),
		func(_, _ *core.CallContext, _ []uint64) {},
		func(_, _ *core.CallContext, _ []uint64) {},
	)
	if err == nil {
		t.Fatal("expected trap error, got nil")
	}
	if !strings.Contains(err.Error(), "intentional trap") {
		t.Errorf("err %q missing trap message", err.Error())
	}
}
