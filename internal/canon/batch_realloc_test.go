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

	helper, err := NewHost(ctx, rt, 0).buildBatchReallocHelper(ctx, targetMod.Name(), "realloc")
	if err != nil {
		t.Fatalf("buildBatchReallocHelper: %v", err)
	}
	t.Cleanup(func() { _ = helper.Close(ctx) })

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

	helper, err := NewHost(ctx, rt, 0).buildBatchReallocHelper(ctx, targetMod.Name(), "realloc")
	if err != nil {
		t.Fatalf("buildBatchReallocHelper: %v", err)
	}
	t.Cleanup(func() { _ = helper.Close(ctx) })

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

	host := NewHost(ctx, rt, 0)

	tgtA := bumpTarget("tgt_share_a", "realloc", 1000)
	tgtB := bumpTarget("tgt_share_b", "realloc", 2000)

	helperA, err := host.buildBatchReallocHelper(ctx, tgtA.Name(), "realloc")
	if err != nil {
		t.Fatalf("build helperA: %v", err)
	}
	t.Cleanup(func() { _ = helperA.Close(ctx) })
	helperB, err := host.buildBatchReallocHelper(ctx, tgtB.Name(), "realloc")
	if err != nil {
		t.Fatalf("build helperB: %v", err)
	}
	t.Cleanup(func() { _ = helperB.Close(ctx) })

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

	// A target exporting its realloc under another name is reached the same way,
	// through the same compiled helper.
	compiled := host.batchHelper
	tgtC := bumpTarget("tgt_share_c", "cabi_realloc", 3000)
	helperC, err := host.buildBatchReallocHelper(ctx, tgtC.Name(), "cabi_realloc")
	if err != nil {
		t.Fatalf("build helperC: %v", err)
	}
	t.Cleanup(func() { _ = helperC.Close(ctx) })
	helperC.writeSizeAlign(0, 16, 1)
	if err := helperC.invoke(ctx, 1); err != nil {
		t.Fatalf("helperC.invoke: %v", err)
	}
	if got := helperC.readPtr(1, 0); got != 3000 {
		t.Errorf("helperC ptr = %d, want 3000", got)
	}
	if host.batchHelper != compiled {
		t.Errorf("a new realloc name replaced the compiled helper")
	}
}

// TestBatchReallocHelper_CompileFailureReported checks a helper that couldn't be
// compiled when the Host was created fails when one is needed, not before.
func TestBatchReallocHelper_CompileFailureReported(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	if err := rt.Close(ctx); err != nil {
		t.Fatal(err)
	}

	host := NewHost(ctx, rt, 0)
	if host.batchHelperErr == nil {
		t.Fatal("expected compiling against a closed runtime to fail")
	}
	if _, err := host.buildBatchReallocHelper(ctx, "target", "realloc"); err == nil {
		t.Fatal("expected the compile failure to be reported")
	}
}

// TestBatchReallocHelper_FailureReleasesBridge checks a helper that can't bind to
// its target's realloc reports it, and that the Host stays usable.
func TestBatchReallocHelper_FailureReleasesBridge(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = rt.Close(ctx) })

	var tb wasm.ModuleBuilder
	tb.AddExportMemory("memory", tb.AddMemory(1, 0))
	var rc wasm.CodeBuilder
	rc.I32Const(0)
	rc.End()
	tb.AddExportFunc("realloc", tb.AddFunc(wasm.FuncSig{
		Params:  []byte{wasm.ValI32, wasm.ValI32, wasm.ValI32, wasm.ValI32},
		Results: []byte{wasm.ValI32},
	}, nil, rc.Bytes()))
	compiled, err := rt.CompileModule(ctx, tb.Encode())
	if err != nil {
		t.Fatal(err)
	}
	target, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("tgt_missing"))
	if err != nil {
		t.Fatal(err)
	}

	host := NewHost(ctx, rt, 0)
	if _, err := host.buildBatchReallocHelper(ctx, target.Name(), "not_exported"); err == nil {
		t.Fatal("expected a missing realloc export to fail")
	}
	helper, err := host.buildBatchReallocHelper(ctx, target.Name(), "realloc")
	if err != nil {
		t.Fatalf("after a failure: %v", err)
	}
	if err := helper.Close(ctx); err != nil {
		t.Fatal(err)
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

	helper, err := NewHost(ctx, rt, 0).buildBatchReallocHelper(cctx, targetMod.Name(), "realloc")
	if err != nil {
		t.Fatalf("buildBatchReallocHelper: %v", err)
	}
	t.Cleanup(func() { _ = helper.Close(ctx) })

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
