package host_test

import (
	"context"
	"os"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
	"github.com/tetratelabs/wazero/api"
)

// TestCallerCoreModule_FlowsFromWasmCallerThroughCanon verifies that a
// wasm component calling into a host-component function can recover its
// own core wasm module via host.CallerCoreModule(ctx). The fixture
// consumer.wasm imports "provider" and calls it; we satisfy the import
// with a host function that snapshots ctx and asserts the snapshot is
// the consumer's core wasm module (identified by its exclusive
// "quadruple" export).
func TestCallerCoreModule_FlowsFromWasmCallerThroughCanon(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	var seen api.Module
	hb := e.NewHostBuilder("provider-host")
	hb.AddFunction("provider", &host.FuncType{
		Params:  []host.Param{{Name: "x", Type: host.S32}},
		Results: []host.ResultDecl{{Name: "", Type: host.S32}},
	}, func(ctx context.Context, _ *core.CallContext, _ *host.ComponentInstance, stack []uint64) error {
		seen = host.CallerCoreModule(ctx)
		stack[0] = uint64(int32(stack[0]) * 2)
		return nil
	})
	hComp, err := hb.Build(ctx)
	if err != nil {
		t.Fatalf("host Build: %v", err)
	}
	defer hComp.Close(ctx)
	hInst, err := hComp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("host Instantiate: %v", err)
	}
	defer hInst.Close(ctx)

	f, err := os.Open("../internal/core/testdata/consumer.wasm")
	if err != nil {
		t.Fatalf("open consumer.wasm: %v", err)
	}
	defer f.Close()
	comp, err := e.LoadComponent(ctx, f)
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}

	inst, err := comp.Instantiate(ctx, wacogo.WithFuncImport("provider", hInst.Core().ExportedFunc("provider")))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	if _, err := inst.ExportedFunc("quadruple").Call(ctx, core.ValS32(3)); err != nil {
		t.Fatalf("quadruple(3): %v", err)
	}

	if seen == nil {
		t.Fatal("host.CallerCoreModule(ctx) was nil inside host callback")
	}
	// The consumer's core module has "quadruple"; the host stub does not.
	if seen.ExportedFunction("quadruple") == nil {
		t.Fatalf("captured caller module %q lacks 'quadruple' export — not the consumer's core module", seen.Name())
	}
}

// TestCallerCoreModule_NilWhenNotSet verifies that CallerCoreModule
// returns nil when no caller module is attached to ctx.
func TestCallerCoreModule_NilWhenNotSet(t *testing.T) {
	if got := host.CallerCoreModule(context.Background()); got != nil {
		t.Fatalf("CallerCoreModule on bare ctx = %v, want nil", got)
	}
}
