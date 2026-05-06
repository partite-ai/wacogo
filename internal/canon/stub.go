package canon

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/partite-ai/wacogo/internal/wasm"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

var stubCounter atomic.Uint64

// buildStubModule compiles and instantiates a tiny wasm module that imports
// hostModName.hostFuncName with sig and re-exports it as exportName. This
// detour is required because wazero's ImportResolver only works with real
// wasm module instances, not host modules.
//
// The returned module owns its instance; callers are responsible for Close
// via CallAdapter.Close.
func buildStubModule(
	ctx context.Context,
	runtime wazero.Runtime,
	hostModName, hostFuncName string,
	sig wasm.FuncSig,
	stubInstName, exportName string,
) (api.Module, error) {
	var mb wasm.ModuleBuilder
	importIdx := mb.AddImportFunc(hostModName, hostFuncName, sig)
	mb.AddExportFunc(exportName, importIdx)
	stubWasm := mb.Encode()

	compiled, err := runtime.CompileModule(ctx, stubWasm)
	if err != nil {
		return nil, fmt.Errorf("compile stub: %w", err)
	}

	n := stubCounter.Add(1)
	instName := fmt.Sprintf("%s_%d", stubInstName, n)
	stubInst, err := runtime.InstantiateModule(ctx, compiled,
		wazero.NewModuleConfig().WithName(instName))
	if err != nil {
		return nil, fmt.Errorf("instantiate stub: %w", err)
	}

	return stubInst, nil
}
