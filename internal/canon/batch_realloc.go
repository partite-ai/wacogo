package canon

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/partite-ai/wacogo/internal/wasm"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

// batchReallocTargetModName is the placeholder import-module name baked into
// the compiled helper and each realloc bridge. It is never registered in the
// store; each instantiation rebinds it through an ImportResolver.
const batchReallocTargetModName = "wacogo:realloc_target"

// batchReallocImportName is the function name the helper imports. wazero can
// rebind an import's module but not its name, so each helper gets a bridge
// exporting its target's realloc under this name. That is what lets every
// adapter share a single compiled helper.
const batchReallocImportName = "realloc"

// batchReallocHelper is a per-adapter tiny wasm module that batches N
// cabi_realloc calls behind a single Go→wasm boundary crossing. The helper
// imports the target's cabi_realloc and exports a function that loops over
// N (size, align) pairs (placed in the helper's private scratch memory by
// the runner) calling the target's realloc N times internally — wasm→wasm
// calls inside the runtime are roughly an order of magnitude cheaper than
// wasm↔Go calls, so collapsing N crossings into 1 is a substantial win for
// any transfer plan that allocates per-element (list<string>,
// list<list<X>>, list<record-with-string>, etc.).
//
// The helper's scratch memory layout per call:
//
//	scratch[i*8 .. i*8+4]      = size_i  (input, written by Go)
//	scratch[i*8+4 .. i*8+8]    = align_i (input, written by Go)
//	scratch[n*8 + i*4 .. n*8 + (i+1)*4] = ptr_i (output, written by wasm)
//
// Scratch is clobbered every call — the helper does not maintain any
// allocator state of its own.
type batchReallocHelper struct {
	mod            api.Module
	bridge         api.Module
	bridgeCompiled wazero.CompiledModule
	batch          api.Function
	scratch        api.Memory
	callStack      []uint64 // reused; sized 1 for (n)
}

var batchHelperCounter atomic.Uint64

// buildBatchReallocHelper instantiates a batched-realloc helper bound to
// the target realloc identified by (targetModName, targetFnName). The caller
// must have already instantiated a module by targetModName that exports
// targetFnName with the cabi_realloc signature.
func (h *Host) buildBatchReallocHelper(
	ctx context.Context,
	targetModName, targetFnName string,
) (_ *batchReallocHelper, err error) {
	if targetModName == "" || targetFnName == "" {
		return nil, fmt.Errorf("buildBatchReallocHelper: empty targetModName or targetFnName")
	}
	if h.batchHelperErr != nil {
		return nil, fmt.Errorf("compile batch_realloc helper: %w", h.batchHelperErr)
	}
	target := h.runtime.Module(targetModName)
	if target == nil {
		return nil, fmt.Errorf("batch_realloc helper: target module %q not found", targetModName)
	}

	helper := &batchReallocHelper{callStack: make([]uint64, 1)}
	defer func() {
		if err != nil {
			_ = helper.Close(ctx)
		}
	}()

	helper.bridgeCompiled, err = h.runtime.CompileModule(ctx, encodeReallocBridgeModule(targetFnName))
	if err != nil {
		return nil, fmt.Errorf("compile batch_realloc bridge: %w", err)
	}
	helper.bridge, err = h.runtime.InstantiateModule(importingFrom(ctx, target), helper.bridgeCompiled,
		wazero.NewModuleConfig().WithName(""))
	if err != nil {
		return nil, fmt.Errorf("instantiate batch_realloc bridge: %w", err)
	}

	// Back the helper's scratch with a plain Go slice rather than any
	// mmap-backed allocator the caller installed for guest modules.
	ictx := experimental.WithMemoryAllocator(importingFrom(ctx, helper.bridge), internalMemoryAllocator)
	instName := fmt.Sprintf("wacogo_batch_realloc_%d", batchHelperCounter.Add(1))
	helper.mod, err = h.runtime.InstantiateModule(ictx, h.batchHelper,
		wazero.NewModuleConfig().WithName(instName))
	if err != nil {
		return nil, fmt.Errorf("instantiate batch_realloc helper: %w", err)
	}

	helper.batch = helper.mod.ExportedFunction("batch")
	if helper.batch == nil {
		return nil, fmt.Errorf("batch_realloc helper: missing export 'batch'")
	}
	helper.scratch = helper.mod.ExportedMemory("scratch")
	if helper.scratch == nil {
		return nil, fmt.Errorf("batch_realloc helper: missing export 'scratch'")
	}
	return helper, nil
}

// importingFrom returns a context that binds the placeholder import module to m.
func importingFrom(ctx context.Context, m api.Module) context.Context {
	return experimental.WithImportResolver(ctx, func(name string) api.Module {
		if name == batchReallocTargetModName {
			return m
		}
		return nil
	})
}

// writeSizeAlign writes the i-th allocation request (size, align) into the
// scratch input region. Must be called for i=0..n-1 before invoke. Callers
// must call ensureRoomFor(i+1) first; this method does not bound-check.
func (h *batchReallocHelper) writeSizeAlign(i, size, align uint32) {
	h.scratch.WriteUint32Le(i*8, size)
	h.scratch.WriteUint32Le(i*8+4, align)
}

// readSize reads back the size of the i-th allocation request written by
// writeSizeAlign. Valid until the next writeSizeAlign for slot i.
func (h *batchReallocHelper) readSize(i uint32) uint32 {
	sz, _ := h.scratch.ReadUint32Le(i * 8)
	return sz
}

// ensureRoomFor grows scratch as needed so it can hold n input+output slots
// (12 bytes/slot: 8 input, 4 output). The helper module declares scratch
// with no maximum, so Grow is bounded only by wazero's default cap; any
// failure is a wazero-level limit and panics — the batched pipeline has
// no other path forward at that point.
func (h *batchReallocHelper) ensureRoomFor(n uint32) {
	needed := uint64(n) * 12
	have := uint64(h.scratch.Size())
	if needed <= have {
		return
	}
	const pageSize = 65536
	deltaPages := uint32((needed - have + pageSize - 1) / pageSize)
	if _, ok := h.scratch.Grow(deltaPages); !ok {
		panic(fmt.Sprintf("batchReallocHelper: failed to grow scratch by %d pages for %d slots", deltaPages, n))
	}
}

// invoke runs the helper's loop, which calls targetRealloc n times
// internally and writes the resulting pointers into the scratch output
// region (scratch[n*8 .. n*8 + n*4]).
func (h *batchReallocHelper) invoke(ctx context.Context, n uint32) error {
	h.callStack[0] = uint64(n)
	if err := h.batch.CallWithStack(ctx, h.callStack); err != nil {
		return fmt.Errorf("batch_realloc helper: %w", err)
	}
	return nil
}

// readPtr reads the i-th allocated pointer from the scratch output region.
// Valid only between an invoke call and the next writeSizeAlign/invoke pair.
func (h *batchReallocHelper) readPtr(n, i uint32) uint32 {
	p, _ := h.scratch.ReadUint32Le(n*8 + i*4)
	return p
}

// Close releases the helper's modules, the helper before the bridge it imports
// from. Called via the adapter's Close.
func (h *batchReallocHelper) Close(ctx context.Context) error {
	if h == nil {
		return nil
	}
	var errs []error
	if h.mod != nil {
		errs = append(errs, h.mod.Close(ctx))
	}
	if h.bridge != nil {
		errs = append(errs, h.bridge.Close(ctx))
	}
	if h.bridgeCompiled != nil {
		errs = append(errs, h.bridgeCompiled.Close(ctx))
	}
	return errors.Join(errs...)
}

// encodeReallocBridgeModule builds a module with no code of its own that exports
// the target's realloc, named targetFnName, under batchReallocImportName.
func encodeReallocBridgeModule(targetFnName string) []byte {
	var mb wasm.ModuleBuilder
	idx := mb.AddImportFunc(batchReallocTargetModName, targetFnName, wasm.FuncSig{
		Params:  []byte{wasm.ValI32, wasm.ValI32, wasm.ValI32, wasm.ValI32},
		Results: []byte{wasm.ValI32},
	})
	mb.AddExportFunc(batchReallocImportName, idx)
	return mb.Encode()
}

// encodeBatchReallocHelperModule builds the wasm bytecode for the helper, which
// imports batchReallocImportName from the batchReallocTargetModName placeholder.
//
// The body implements the loop:
//
//	for i := uint32(0); i < n; i++ {
//	    size  := scratch[i*8]
//	    align := scratch[i*8 + 4]
//	    scratch[n*8 + i*4] = realloc(0, 0, align, size)
//	}
//
// Param 0 = n. One i32 local: i.
func encodeBatchReallocHelperModule() []byte {
	var mb wasm.ModuleBuilder

	reallocSig := wasm.FuncSig{
		Params:  []byte{wasm.ValI32, wasm.ValI32, wasm.ValI32, wasm.ValI32},
		Results: []byte{wasm.ValI32},
	}
	reallocIdx := mb.AddImportFunc(batchReallocTargetModName, batchReallocImportName, reallocSig)

	memIdx := mb.AddMemory(1, 0)
	mb.AddExportMemory("scratch", memIdx)

	batchSig := wasm.FuncSig{
		Params:  []byte{wasm.ValI32},
		Results: nil,
	}
	var c wasm.CodeBuilder
	// block $exit
	c.Block(0x40)
	//   br_if $exit (i32.eqz n)
	c.LocalGet(0)
	c.I32Eqz()
	c.BrIf(0)
	//   loop $loop
	c.Loop(0x40)
	//     store address = n*8 + i*4
	c.LocalGet(0) // n
	c.I32Const(3)
	c.I32Shl()    // n*8
	c.LocalGet(1) // i
	c.I32Const(2)
	c.I32Shl() // i*4
	c.I32Add() // ptrs_base + i*4
	//     realloc args: (0, 0, align=scratch[i*8+4], size=scratch[i*8])
	c.I32Const(0)
	c.I32Const(0)
	c.LocalGet(1)
	c.I32Const(3)
	c.I32Shl()
	c.I32Load(4, 2) // load align at offset i*8 + 4, align=2 (=log2(4))
	c.LocalGet(1)
	c.I32Const(3)
	c.I32Shl()
	c.I32Load(0, 2) // load size at offset i*8, align=2
	c.Call(reallocIdx)
	c.I32Store(0, 2)
	//     i++
	c.LocalGet(1)
	c.I32Const(1)
	c.I32Add()
	c.LocalSet(1)
	//     br_if $loop (i < n)
	c.LocalGet(1)
	c.LocalGet(0)
	c.I32LtU()
	c.BrIf(0)
	c.End() // loop
	c.End() // block
	c.End() // func

	body := c.Bytes()
	locals := []wasm.LocalEntry{wasm.NewLocalEntry(1, wasm.ValI32)}
	batchIdx := mb.AddFunc(batchSig, locals, body)
	mb.AddExportFunc("batch", batchIdx)

	return mb.Encode()
}
