package canon

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo/internal/wasm"
	"github.com/tetratelabs/wazero"
)

// TestBatchReallocHelper_RoundTrip wires the helper to a stub "realloc"
// implemented in wasm (a bump allocator) and verifies that calling the
// helper's "batch" with N=4 sizes produces N distinct, correctly-spaced
// pointers in scratch[n*4..n*8].
func TestBatchReallocHelper_RoundTrip(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = rt.Close(ctx) })

	// Target module: a tiny bump allocator exposed as "realloc".
	// Bump starts at 1024, returns the pre-bump value, advances by newSize.
	// Sig: (param i32 i32 i32 i32) (result i32) -> (origPtr, origSize, align, newSize) -> ptr.
	// Body simplifies: ignore origPtr/origSize/align, just bump by newSize.
	var tb wasm.ModuleBuilder
	bumpIdx := tb.AddGlobal(wasm.ValI32, true, 1024)
	memIdx := tb.AddMemory(1, 0)
	tb.AddExportMemory("memory", memIdx)
	reallocSig := wasm.FuncSig{
		Params:  []byte{wasm.ValI32, wasm.ValI32, wasm.ValI32, wasm.ValI32},
		Results: []byte{wasm.ValI32},
	}
	var rc wasm.CodeBuilder
	// local 0 = origPtr, local 1 = origSize, local 2 = align, local 3 = newSize.
	// Push the current bump (return value) and save it.
	rc.GlobalGet(bumpIdx) // [ptr]
	rc.GlobalGet(bumpIdx) // [ptr, ptr]
	rc.LocalGet(3)        // [ptr, ptr, newSize]
	rc.I32Add()           // [ptr, ptr+newSize]
	rc.GlobalSet(bumpIdx) // [ptr]  bump now = ptr+newSize
	rc.End()
	reallocCoreIdx := tb.AddFunc(reallocSig, nil, rc.Bytes())
	tb.AddExportFunc("realloc", reallocCoreIdx)
	targetBytes := tb.Encode()

	targetCompiled, err := rt.CompileModule(ctx, targetBytes)
	if err != nil {
		t.Fatalf("compile target: %v", err)
	}
	targetMod, err := rt.InstantiateModule(ctx, targetCompiled,
		wazero.NewModuleConfig().WithName("tgt_for_batch_test"))
	if err != nil {
		t.Fatalf("instantiate target: %v", err)
	}

	helper, err := buildBatchReallocHelper(ctx, rt, targetMod.Name(), "realloc")
	if err != nil {
		t.Fatalf("buildBatchReallocHelper: %v", err)
	}
	t.Cleanup(func() { _ = helper.close(ctx) })

	sizes := []uint32{16, 32, 8, 64}
	for i, s := range sizes {
		helper.writeSizeAlign(uint32(i), s, 1)
	}
	if err := helper.invoke(ctx, uint32(len(sizes))); err != nil {
		t.Fatalf("helper.invoke: %v", err)
	}

	// Bump starts at 1024, allocations are sequential: 1024, 1040, 1072, 1080.
	want := []uint32{1024, 1024 + 16, 1024 + 16 + 32, 1024 + 16 + 32 + 8}
	n := uint32(len(sizes))
	for i, w := range want {
		got := helper.readPtr(n, uint32(i))
		if got != w {
			t.Errorf("ptr[%d] = %d, want %d", i, got, w)
		}
	}
}

// TestBatchReallocHelper_ZeroN verifies the n=0 fast path: no realloc
// invocations, no scratch writes, no errors.
func TestBatchReallocHelper_ZeroN(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = rt.Close(ctx) })

	// Trivial target: a realloc that panics. Verifies n=0 truly skips it.
	var tb wasm.ModuleBuilder
	memIdx := tb.AddMemory(1, 0)
	tb.AddExportMemory("memory", memIdx)
	reallocSig := wasm.FuncSig{
		Params:  []byte{wasm.ValI32, wasm.ValI32, wasm.ValI32, wasm.ValI32},
		Results: []byte{wasm.ValI32},
	}
	var rc wasm.CodeBuilder
	rc.Unreachable()
	rc.End()
	reallocCoreIdx := tb.AddFunc(reallocSig, nil, rc.Bytes())
	tb.AddExportFunc("realloc", reallocCoreIdx)
	targetBytes := tb.Encode()
	targetCompiled, err := rt.CompileModule(ctx, targetBytes)
	if err != nil {
		t.Fatalf("compile target: %v", err)
	}
	targetMod, err := rt.InstantiateModule(ctx, targetCompiled,
		wazero.NewModuleConfig().WithName("tgt_zero_n"))
	if err != nil {
		t.Fatalf("instantiate target: %v", err)
	}

	helper, err := buildBatchReallocHelper(ctx, rt, targetMod.Name(), "realloc")
	if err != nil {
		t.Fatalf("buildBatchReallocHelper: %v", err)
	}
	t.Cleanup(func() { _ = helper.close(ctx) })

	if err := helper.invoke(ctx, 0); err != nil {
		t.Fatalf("helper.invoke(0): %v", err)
	}
}
