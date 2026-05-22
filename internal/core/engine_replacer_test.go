package core

import (
	"context"
	"os"
	"testing"

	"github.com/partite-ai/wacogo/internal/wasm"
	"github.com/tetratelabs/wazero"
)

// TestWithCoreModuleReplacer_SubstitutesInlineModule loads simple-add.wasm
// — a component whose single inline core module exports add(i32,i32)→i32
// — and uses a replacer that swaps in a module of identical shape whose
// "add" actually subtracts. After the swap, calling add(10, 3) should
// return 7 (sub), not 13 (the original sum).
func TestWithCoreModuleReplacer_SubstitutesInlineModule(t *testing.T) {
	ctx := context.Background()

	var replacerFiredFor wazero.CompiledModule
	var replaceWith wazero.CompiledModule

	engine := NewEngine(ctx, WithCoreModuleReplacer(func(cm wazero.CompiledModule) wazero.CompiledModule {
		replacerFiredFor = cm
		return replaceWith
	}))
	defer engine.Close(ctx)

	// Build the substitute module on the same runtime. Same exports as
	// the original (add, memory, realloc) but "add" subtracts.
	var b wasm.ModuleBuilder
	subBody := []byte{0x20, 0x00, 0x20, 0x01, 0x6b, 0x0b} // local.get 0; local.get 1; i32.sub; end
	subIdx := b.AddFunc(wasm.FuncSig{Params: []byte{wasm.ValI32, wasm.ValI32}, Results: []byte{wasm.ValI32}}, nil, subBody)
	b.AddExportFunc("add", subIdx)
	zeroBody := []byte{0x41, 0x00, 0x0b} // i32.const 0; end
	rIdx := b.AddFunc(wasm.FuncSig{
		Params:  []byte{wasm.ValI32, wasm.ValI32, wasm.ValI32, wasm.ValI32},
		Results: []byte{wasm.ValI32},
	}, nil, zeroBody)
	b.AddExportFunc("realloc", rIdx)
	b.AddMemory(1, 0)
	b.AddExportMemory("memory", 0)

	var err error
	replaceWith, err = engine.runtime.CompileModule(ctx, b.Encode())
	if err != nil {
		t.Fatalf("CompileModule(substitute): %v", err)
	}

	f, err := os.Open("testdata/simple-add.wasm")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	comp, err := engine.LoadComponent(ctx, f)
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}

	if replacerFiredFor == nil {
		t.Fatal("replacer was never invoked")
	}
	if replacerFiredFor == replaceWith {
		t.Fatal("replacer received the substitute, not the freshly-compiled original")
	}
	if len(comp.compiledModules) != 1 {
		t.Fatalf("want 1 inline module slot, got %d", len(comp.compiledModules))
	}
	if comp.compiledModules[0].module != replaceWith {
		t.Fatal("compiled-module slot did not hold the substitute after load")
	}

	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	results, err := inst.ExportedFunc("add").Call(ctx, ValS32(10), ValS32(3))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := int32(results[0].(ValS32)); got != 7 {
		t.Fatalf("substitute add(10,3) = %d, want 7 (sub)", got)
	}
}

// TestWithCoreModuleReplacer_NilKeepsOriginal verifies that a replacer
// returning nil leaves the engine's freshly-compiled module in place.
func TestWithCoreModuleReplacer_NilKeepsOriginal(t *testing.T) {
	ctx := context.Background()

	calls := 0
	engine := NewEngine(ctx, WithCoreModuleReplacer(func(wazero.CompiledModule) wazero.CompiledModule {
		calls++
		return nil
	}))
	defer engine.Close(ctx)

	f, err := os.Open("testdata/simple-add.wasm")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	comp, err := engine.LoadComponent(ctx, f)
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}
	if calls != 1 {
		t.Fatalf("replacer fired %d times, want 1", calls)
	}

	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	results, err := inst.ExportedFunc("add").Call(ctx, ValS32(10), ValS32(3))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := int32(results[0].(ValS32)); got != 13 {
		t.Fatalf("original add(10,3) = %d, want 13", got)
	}
}
