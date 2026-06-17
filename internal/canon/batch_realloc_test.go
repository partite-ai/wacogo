package canon

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/partite-ai/wacogo/internal/wasm"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
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

	helper, err := NewHost(rt).buildBatchReallocHelper(ctx, targetMod.Name(), "realloc")
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

	helper, err := NewHost(rt).buildBatchReallocHelper(ctx, targetMod.Name(), "realloc")
	if err != nil {
		t.Fatalf("buildBatchReallocHelper: %v", err)
	}
	t.Cleanup(func() { _ = helper.close(ctx) })

	if err := helper.invoke(ctx, 0); err != nil {
		t.Fatalf("helper.invoke(0): %v", err)
	}
}

// TestBatchReallocHelper_SharesCompiledModule verifies the core
// optimization: two helpers built from the same Host against the same
// realloc export name reuse a single compiled module (one cache entry),
// while a distinct export name compiles a second. Both helpers, bound to
// different target instances via the per-instantiation ImportResolver, must
// route to their own target's realloc.
func TestBatchReallocHelper_SharesCompiledModule(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = rt.Close(ctx) })

	// bumpTarget builds a target module exporting a bump allocator under
	// exportName, starting at bumpStart.
	bumpTarget := func(name, exportName string, bumpStart int32) api.Module {
		var tb wasm.ModuleBuilder
		bumpIdx := tb.AddGlobal(wasm.ValI32, true, bumpStart)
		memIdx := tb.AddMemory(1, 0)
		tb.AddExportMemory("memory", memIdx)
		sig := wasm.FuncSig{
			Params:  []byte{wasm.ValI32, wasm.ValI32, wasm.ValI32, wasm.ValI32},
			Results: []byte{wasm.ValI32},
		}
		var rc wasm.CodeBuilder
		rc.GlobalGet(bumpIdx)
		rc.GlobalGet(bumpIdx)
		rc.LocalGet(3)
		rc.I32Add()
		rc.GlobalSet(bumpIdx)
		rc.End()
		fnIdx := tb.AddFunc(sig, nil, rc.Bytes())
		tb.AddExportFunc(exportName, fnIdx)
		compiled, err := rt.CompileModule(ctx, tb.Encode())
		if err != nil {
			t.Fatalf("compile target %q: %v", name, err)
		}
		mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(name))
		if err != nil {
			t.Fatalf("instantiate target %q: %v", name, err)
		}
		return mod
	}

	host := NewHost(rt)

	tgtA := bumpTarget("tgt_share_a", "realloc", 1000)
	tgtB := bumpTarget("tgt_share_b", "realloc", 2000)

	helperA, err := host.buildBatchReallocHelper(ctx, tgtA.Name(), "realloc")
	if err != nil {
		t.Fatalf("build helperA: %v", err)
	}
	t.Cleanup(func() { _ = helperA.close(ctx) })
	helperB, err := host.buildBatchReallocHelper(ctx, tgtB.Name(), "realloc")
	if err != nil {
		t.Fatalf("build helperB: %v", err)
	}
	t.Cleanup(func() { _ = helperB.close(ctx) })

	// Both helpers used the same export name -> exactly one compiled module.
	if got := len(host.batchHelperCache); got != 1 {
		t.Fatalf("cache entries after two same-name helpers = %d, want 1", got)
	}

	// Each helper routes to its own target despite sharing the compiled
	// module: helperA bumps from 1000, helperB from 2000.
	helperA.writeSizeAlign(0, 16, 1)
	if err := helperA.invoke(ctx, 1); err != nil {
		t.Fatalf("helperA.invoke: %v", err)
	}
	if got := helperA.readPtr(1, 0); got != 1000 {
		t.Errorf("helperA ptr = %d, want 1000", got)
	}
	helperB.writeSizeAlign(0, 16, 1)
	if err := helperB.invoke(ctx, 1); err != nil {
		t.Fatalf("helperB.invoke: %v", err)
	}
	if got := helperB.readPtr(1, 0); got != 2000 {
		t.Errorf("helperB ptr = %d, want 2000", got)
	}

	// A distinct export name compiles a second module.
	tgtC := bumpTarget("tgt_share_c", "cabi_realloc", 3000)
	helperC, err := host.buildBatchReallocHelper(ctx, tgtC.Name(), "cabi_realloc")
	if err != nil {
		t.Fatalf("build helperC: %v", err)
	}
	t.Cleanup(func() { _ = helperC.close(ctx) })
	if got := len(host.batchHelperCache); got != 2 {
		t.Fatalf("cache entries after distinct export name = %d, want 2", got)
	}
}

// TestBatchReallocHelper_OverridesCallerMemoryAllocator verifies that a
// caller-installed MemoryAllocator (e.g. an mmap-backed one) riding in on the
// instantiation context is NOT used to back the helper's scratch memory — we
// override it with the cheap slice allocator — while the helper still works.
func TestBatchReallocHelper_OverridesCallerMemoryAllocator(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = rt.Close(ctx) })

	// Bump-allocator target exporting "realloc".
	var tb wasm.ModuleBuilder
	bumpIdx := tb.AddGlobal(wasm.ValI32, true, 1024)
	memIdx := tb.AddMemory(1, 0)
	tb.AddExportMemory("memory", memIdx)
	sig := wasm.FuncSig{
		Params:  []byte{wasm.ValI32, wasm.ValI32, wasm.ValI32, wasm.ValI32},
		Results: []byte{wasm.ValI32},
	}
	var rc wasm.CodeBuilder
	rc.GlobalGet(bumpIdx)
	rc.GlobalGet(bumpIdx)
	rc.LocalGet(3)
	rc.I32Add()
	rc.GlobalSet(bumpIdx)
	rc.End()
	fnIdx := tb.AddFunc(sig, nil, rc.Bytes())
	tb.AddExportFunc("realloc", fnIdx)
	compiled, err := rt.CompileModule(ctx, tb.Encode())
	if err != nil {
		t.Fatalf("compile target: %v", err)
	}
	targetMod, err := rt.InstantiateModule(ctx, compiled,
		wazero.NewModuleConfig().WithName("tgt_alloc_override"))
	if err != nil {
		t.Fatalf("instantiate target: %v", err)
	}

	// Caller installs a counting allocator in the ctx, as an embedder would.
	var calls int32
	callerAlloc := experimental.MemoryAllocatorFunc(func(_, _ uint64) experimental.LinearMemory {
		atomic.AddInt32(&calls, 1)
		return &sliceMemory{}
	})
	cctx := experimental.WithMemoryAllocator(ctx, callerAlloc)

	helper, err := NewHost(rt).buildBatchReallocHelper(cctx, targetMod.Name(), "realloc")
	if err != nil {
		t.Fatalf("buildBatchReallocHelper: %v", err)
	}
	t.Cleanup(func() { _ = helper.close(ctx) })

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("caller allocator used for internal helper scratch: %d calls, want 0", got)
	}

	// The overridden slice-backed scratch still works end to end.
	helper.writeSizeAlign(0, 16, 1)
	if err := helper.invoke(ctx, 1); err != nil {
		t.Fatalf("helper.invoke: %v", err)
	}
	if got := helper.readPtr(1, 0); got != 1024 {
		t.Errorf("ptr = %d, want 1024", got)
	}
}
