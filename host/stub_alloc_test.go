package host

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo/internal/core"
	"github.com/tetratelabs/wazero/api"
)

// TestStubAllocator_MatchesWasmRealloc runs the same allocations through
// the stub's wasm realloc and through stubAllocator, on two instances of
// one stub, and requires the same pointers, memory sizes and contents.
func TestStubAllocator_MatchesWasmRealloc(t *testing.T) {
	ctx := context.Background()
	b, e := newTestBuilder(t)
	b.AddFunction("noop", &FuncType{}, func(context.Context, *core.CallContext, *ComponentInstance, []uint64) error {
		return nil
	})
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)

	rt := core.WazeroRuntime(e)
	newStub := func() api.Module {
		hostMod, err := buildPerInstanceHostMod(ctx, rt, comp, &ComponentInstance{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { hostMod.Close(ctx) })
		stub, err := instantiateStubModule(ctx, rt, comp.compiledStub, hostMod)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { stub.Close(ctx) })
		return stub
	}
	wasmStub, goStub := newStub(), newStub()
	wasmRealloc := wasmStub.ExportedFunction("realloc")
	alloc, err := newStubAllocator(goStub)
	if err != nil {
		t.Fatal(err)
	}

	type op struct {
		reset                        bool
		oldPtr, oldSize, align, size uint32
	}
	ops := []op{
		{align: 1, size: 3},
		{align: 8, size: 16},
		{align: 4, size: 0},
		{align: 2, size: 100},
		{align: 1, size: 70000}, // grows the memory
		{reset: true},
		{align: 8, size: 24},
		{oldPtr: 16, oldSize: 24, align: 8, size: 48}, // grow in place of the last block: copies
		{oldPtr: 16, oldSize: 48, align: 4, size: 8},  // shrink: copies the prefix
		{align: 1, size: 3 << 16},                     // grows by several pages
	}
	var bumpW api.MutableGlobal = wasmStub.ExportedGlobal("bump").(api.MutableGlobal)
	for i, o := range ops {
		if o.reset {
			bumpW.Set(uint64(uint32(stubBumpInitial)))
			alloc.reset()
			continue
		}
		// Distinct contents at the source, so copies are checked.
		for _, m := range []api.Memory{wasmStub.Memory(), goStub.Memory()} {
			if o.oldSize > 0 {
				src, _ := m.Read(o.oldPtr, o.oldSize)
				for j := range src {
					src[j] = byte(i + j)
				}
			}
		}
		res, err := wasmRealloc.Call(ctx, uint64(o.oldPtr), uint64(o.oldSize), uint64(o.align), uint64(o.size))
		if err != nil {
			t.Fatalf("op %d: wasm realloc: %v", i, err)
		}
		got, err := alloc.realloc(ctx, o.oldPtr, o.oldSize, o.align, o.size)
		if err != nil {
			t.Fatalf("op %d: Go realloc: %v", i, err)
		}
		if want := uint32(res[0]); got != want {
			t.Fatalf("op %d %+v: Go realloc = %d, wasm = %d", i, o, got, want)
		}
		if gw, gg := bumpW.Get(), alloc.bump.Get(); gw != gg {
			t.Fatalf("op %d: bump: Go %d, wasm %d", i, gg, gw)
		}
		mw, mg := wasmStub.Memory(), goStub.Memory()
		if mw.Size() != mg.Size() {
			t.Fatalf("op %d: memory size: Go %d, wasm %d", i, mg.Size(), mw.Size())
		}
		bw, _ := mw.Read(0, mw.Size())
		bg, _ := mg.Read(0, mg.Size())
		if !bytes.Equal(bw, bg) {
			t.Fatalf("op %d: memory contents differ", i)
		}
	}
}
