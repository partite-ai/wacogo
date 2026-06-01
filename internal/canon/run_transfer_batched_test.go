package canon

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo/internal/wasm"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// testStringType is a minimal Type that visits as string, for tests that
// need a string element inside a composite (list<string>, record-with-string).
type testStringType struct{}

func (testStringType) Accept(v TypeVisitor) { v.VisitString() }

// reallocFromExport wraps a wazero-exported realloc as a canon ReallocFunc.
func reallocFromExport(fn api.Function) ReallocFunc {
	return func(ctx context.Context, origPtr, origSize, align, newSize uint32) (uint32, error) {
		stack := []uint64{uint64(origPtr), uint64(origSize), uint64(align), uint64(newSize)}
		if err := fn.CallWithStack(ctx, stack); err != nil {
			return 0, err
		}
		return uint32(stack[0]), nil
	}
}

// setupBumpCallee builds a wazero module that exposes a bump-allocator
// realloc and a memory. Bump starts at `bumpStart` and ignores align.
// Returns the instantiated module so tests can read its memory and pass
// (modName, "realloc") to buildBatchReallocHelper.
func setupBumpCallee(t *testing.T, rt wazero.Runtime, name string, bumpStart uint32) api.Module {
	t.Helper()
	ctx := context.Background()
	var b wasm.ModuleBuilder
	bumpIdx := b.AddGlobal(wasm.ValI32, true, int32(bumpStart))
	memIdx := b.AddMemory(1, 0)
	b.AddExportMemory("memory", memIdx)
	reallocSig := wasm.FuncSig{
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
	reallocIdx := b.AddFunc(reallocSig, nil, rc.Bytes())
	b.AddExportFunc("realloc", reallocIdx)
	compiled, err := rt.CompileModule(ctx, b.Encode())
	if err != nil {
		t.Fatalf("compile bump callee: %v", err)
	}
	mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(name))
	if err != nil {
		t.Fatalf("instantiate bump callee: %v", err)
	}
	t.Cleanup(func() { _ = mod.Close(ctx) })
	return mod
}

// TestRunPhasedTransfer_BatchedListString verifies the batched two-phase
// path of runPhasedTransfer end-to-end against a list<string>. Confirms
// that the discovery walk records the same number of allocations the
// transfer walk consumes (no visitor-pair drift) and that destination
// memory is correctly populated through batch-supplied pointers.
func TestRunPhasedTransfer_BatchedListString(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = rt.Close(ctx) })

	// Set up callee module with bump realloc starting at 1024.
	const bumpStart uint32 = 1024
	calleeMod := setupBumpCallee(t, rt, "callee_for_batched_list", bumpStart)

	// Set up caller (src) module with just a memory.
	var sb wasm.ModuleBuilder
	sb.AddMemory(1, 0)
	sb.AddExportMemory("memory", 0)
	srcCompiled, err := rt.CompileModule(ctx, sb.Encode())
	if err != nil {
		t.Fatalf("compile src: %v", err)
	}
	srcMod, err := rt.InstantiateModule(ctx, srcCompiled,
		wazero.NewModuleConfig().WithName("src_for_batched_list"))
	if err != nil {
		t.Fatalf("instantiate src: %v", err)
	}
	t.Cleanup(func() { _ = srcMod.Close(ctx) })

	helper, err := buildBatchReallocHelper(ctx, rt, calleeMod.Name(), "realloc")
	if err != nil {
		t.Fatalf("buildBatchReallocHelper: %v", err)
	}
	t.Cleanup(func() { _ = helper.close(ctx) })

	// Seed src memory. List buffer at offset 0 holds 3 (ptr, len) pairs;
	// string bytes live at offsets 100, 200, 300.
	srcMem := srcMod.Memory()
	strs := []struct {
		ptr, length uint32
		s           string
	}{
		{100, 3, "foo"},
		{200, 6, "barbaz"},
		{300, 4, "quux"},
	}
	for i, e := range strs {
		base := uint32(i) * 8
		if !srcMem.WriteUint32Le(base, e.ptr) {
			t.Fatalf("seed list ptr[%d]", i)
		}
		if !srcMem.WriteUint32Le(base+4, e.length) {
			t.Fatalf("seed list len[%d]", i)
		}
		if !srcMem.Write(e.ptr, []byte(e.s)) {
			t.Fatalf("seed string %d", i)
		}
	}

	// Compile a list<string> transfer (flat mode: 2 registers — list ptr, n).
	v := &flatTransferVisitor{}
	v.VisitList(testStringType{})
	if len(v.out) != 1 {
		t.Fatalf("expected 1 emitted step for list<string>, got %d", len(v.out))
	}

	tc := &transferContext{
		caller: &transferSide{
			Instance:       &testInstance{name: "caller"},
			Memory:         srcMem,
			StringEncoding: EncUTF8,
		},
		callee: &transferSide{
			Instance:       &testInstance{name: "callee"},
			Memory:         calleeMod.Memory(),
			Realloc:        reallocFromExport(calleeMod.ExportedFunction("realloc")),
			StringEncoding: EncUTF8,
			BatchHelper:    helper,
		},
		registers: []uint64{0, uint64(len(strs))}, // list ptr = 0, n = 3
	}

	runPhasedTransfer(ctx, tc, v.out, 0, 0)

	// Discovery should have recorded 4 allocations: 1 outer list buffer
	// + 1 per string. If this drifts from what transfer consumed, the
	// allocSource cursor overrun panic would have already fired.
	wantAllocs := uint32(1 + len(strs))
	if tc.allocSink.n != wantAllocs {
		t.Errorf("allocSink.n = %d, want %d", tc.allocSink.n, wantAllocs)
	}
	if tc.allocSrc.helper == nil {
		t.Errorf("allocSrc.helper = nil; batched path was not taken")
	}

	// Bump-allocator order: list buffer first (24 bytes), then strings in
	// declaration order (3, 6, 4 bytes).
	wantListPtr := bumpStart
	wantStrPtrs := []uint32{
		bumpStart + 24,
		bumpStart + 24 + 3,
		bumpStart + 24 + 3 + 6,
	}

	gotListPtr := uint32(tc.registers[0])
	gotN := uint32(tc.registers[1])
	if gotListPtr != wantListPtr {
		t.Errorf("dst list ptr (register 0) = %d, want %d", gotListPtr, wantListPtr)
	}
	if gotN != uint32(len(strs)) {
		t.Errorf("dst n (register 1) = %d, want %d", gotN, len(strs))
	}

	dstMem := calleeMod.Memory()
	for i, e := range strs {
		base := gotListPtr + uint32(i)*8
		gotPtr, _ := dstMem.ReadUint32Le(base)
		gotLen, _ := dstMem.ReadUint32Le(base + 4)
		if gotPtr != wantStrPtrs[i] {
			t.Errorf("string[%d] dst ptr = %d, want %d", i, gotPtr, wantStrPtrs[i])
		}
		if gotLen != e.length {
			t.Errorf("string[%d] dst len = %d, want %d", i, gotLen, e.length)
		}
		gotBytes, ok := dstMem.Read(gotPtr, gotLen)
		if !ok {
			t.Errorf("string[%d] dst bytes oob read at %d", i, gotPtr)
			continue
		}
		if string(gotBytes) != e.s {
			t.Errorf("string[%d] dst bytes = %q, want %q", i, string(gotBytes), e.s)
		}
	}
}

// TestRunPhasedTransfer_GrowsScratchForLargeN verifies that when the
// discovery walk requests more allocation slots than the helper's initial
// 1-page scratch can hold, the helper grows scratch as needed and the
// batched path still completes correctly. n=6000 exceeds the 5461 slots
// that fit in 1 page (65536/12), so growth is mandatory.
func TestRunPhasedTransfer_GrowsScratchForLargeN(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = rt.Close(ctx) })

	const n = 6000
	calleeMod := setupBumpCallee(t, rt, "callee_for_grow", 1024)

	var sb wasm.ModuleBuilder
	sb.AddMemory(2, 0) // 2 pages so the 8*n=48000-byte list buffer fits
	sb.AddExportMemory("memory", 0)
	srcCompiled, err := rt.CompileModule(ctx, sb.Encode())
	if err != nil {
		t.Fatalf("compile src: %v", err)
	}
	srcMod, err := rt.InstantiateModule(ctx, srcCompiled,
		wazero.NewModuleConfig().WithName("src_for_grow"))
	if err != nil {
		t.Fatalf("instantiate src: %v", err)
	}
	t.Cleanup(func() { _ = srcMod.Close(ctx) })

	helper, err := buildBatchReallocHelper(ctx, rt, calleeMod.Name(), "realloc")
	if err != nil {
		t.Fatalf("buildBatchReallocHelper: %v", err)
	}
	t.Cleanup(func() { _ = helper.close(ctx) })

	initialScratch := helper.scratch.Size()

	// Src list buffer is n entries of (ptr=0, len=0) — zero-length strings.
	// Each still costs 1 allocation slot in discovery; this is the cheapest
	// way to force a large N without large data movement.
	srcMem := srcMod.Memory()

	v := &flatTransferVisitor{}
	v.VisitList(testStringType{})

	tc := &transferContext{
		caller: &transferSide{
			Instance:       &testInstance{name: "caller"},
			Memory:         srcMem,
			StringEncoding: EncUTF8,
		},
		callee: &transferSide{
			Instance:       &testInstance{name: "callee"},
			Memory:         calleeMod.Memory(),
			Realloc:        reallocFromExport(calleeMod.ExportedFunction("realloc")),
			StringEncoding: EncUTF8,
			BatchHelper:    helper,
		},
		registers: []uint64{0, n},
	}

	runPhasedTransfer(ctx, tc, v.out, 0, 0)

	// Scratch must have grown — 1 + 6000 = 6001 slots * 12 bytes = 72012
	// bytes, exceeding the initial 65536-byte page.
	if helper.scratch.Size() <= initialScratch {
		t.Errorf("scratch did not grow: still %d bytes (initial %d) for %d slots",
			helper.scratch.Size(), initialScratch, n+1)
	}
	// Batched path must have been taken.
	if tc.allocSrc.helper == nil {
		t.Errorf("allocSrc.helper = nil; batched path was not taken")
	}
	// All n+1 allocations recorded.
	if tc.allocSink.n != n+1 {
		t.Errorf("allocSink.n = %d, want %d", tc.allocSink.n, n+1)
	}
	// And the transfer completed correctly.
	gotN := uint32(tc.registers[1])
	if gotN != n {
		t.Errorf("dst n = %d, want %d", gotN, n)
	}
}
