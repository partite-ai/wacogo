package host

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/partite-ai/wacogo/internal/core"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// hostCounter provides unique per-instance host-module names.
var hostCounter atomic.Uint64

// buildPerInstanceHostMod constructs a fresh wazero host module whose
// Go functions close directly over the supplied *ComponentInstance
// wrapper. One host module per Instantiate call; closing the
// *ComponentInstance also closes this module.
//
// h.core must be filled before any of these closures fire — the host
// module is built before core.NewInstance returns, but the closures
// run only at call time, well after Component.Instantiate has
// populated h.core.
func buildPerInstanceHostMod(
	ctx context.Context,
	rt wazero.Runtime,
	comp *Component,
	h *ComponentInstance,
) (api.Module, error) {
	modName := fmt.Sprintf("%s_host_%d", comp.name, hostCounter.Add(1))
	hmb := rt.NewHostModuleBuilder(modName)

	for i := range comp.allFuncs {
		fr := comp.allFuncs[i] // capture pointer for closure (stable across iterations)
		paramTypes := valueTypesFromCoreBytes(fr.flatParams)
		resultTypes := valueTypesFromCoreBytes(fr.flatResults)
		fn := api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			cc := core.NewCallContext(h.core, nil, mod.Memory(), wrapRealloc(mod.ExportedFunction("realloc")))
			if err := fr.userFn(ctx, cc, h, stack); err != nil {
				// wazero converts a panic from a host function into a wasm
				// trap on the calling instance. Panicking with the error
				// surfaces it to the caller verbatim.
				panic(err)
			}
		})
		hmb.NewFunctionBuilder().
			WithGoModuleFunction(fn, paramTypes, resultTypes).
			Export(fr.hostModExport)
	}

	for i := range comp.allResources {
		rr := comp.allResources[i] // capture pointer for closure
		if rr.hostModExport == "" {
			continue
		}
		fn := api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			rep := uint32(stack[0])
			obj, _ := h.releaseResource(ExternHandle(rep))
			if rr.userDtor != nil {
				_ = rr.userDtor(ctx, h, obj)
			}
		})
		hmb.NewFunctionBuilder().
			WithGoModuleFunction(fn,
				[]api.ValueType{api.ValueTypeI32},
				nil).
			Export(rr.hostModExport)
	}

	// Use Compile + InstantiateModule (not the convenience Instantiate) so
	// that the returned api.Module is a raw *wasm.ModuleInstance. The
	// experimental.WithImportResolver path in wazero's store.resolveImports
	// does a hard type-assert to *wasm.ModuleInstance; the wrapped
	// hostModuleInstance value returned by the convenience Instantiate
	// method would panic on that assertion.
	compiled, err := hmb.Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("wacogo/host: compile per-instance hostMod: %w", err)
	}
	mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(modName))
	if err != nil {
		return nil, fmt.Errorf("wacogo/host: instantiate per-instance hostMod: %w", err)
	}
	return mod, nil
}

// wrapRealloc adapts a wazero realloc export into a core.ReallocFunc.
// Returns nil if fn is nil.
func wrapRealloc(fn api.Function) core.ReallocFunc {
	if fn == nil {
		return nil
	}
	return func(ctx context.Context, origPtr, origSize, align, newSize uint32) (uint32, error) {
		ret, err := fn.Call(ctx, uint64(origPtr), uint64(origSize), uint64(align), uint64(newSize))
		if err != nil {
			return 0, err
		}
		return uint32(ret[0]), nil
	}
}

// valueTypesFromCoreBytes converts internal/wasm.ValI32/… bytes into
// api.ValueType elements.
func valueTypesFromCoreBytes(b []byte) []api.ValueType {
	out := make([]api.ValueType, len(b))
	for i, v := range b {
		out[i] = apiValueTypeFromByte(v)
	}
	return out
}

func apiValueTypeFromByte(b byte) api.ValueType {
	switch b {
	case 0x7f:
		return api.ValueTypeI32
	case 0x7e:
		return api.ValueTypeI64
	case 0x7d:
		return api.ValueTypeF32
	case 0x7c:
		return api.ValueTypeF64
	default:
		panic(fmt.Sprintf("wacogo/host: unknown core value type byte 0x%x", b))
	}
}
