package canon

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/partite-ai/wacogo/internal/wasm"
	"github.com/tetratelabs/wazero/api"
)

var adapterCounter atomic.Uint64

// BuildAdapter constructs a cross-component adapter for a single lowered
// function: a host module containing the Go transfer function plus a tiny
// wasm stub module that imports and re-exports it as "adapt".
//
// The returned *CallAdapter owns both modules; call Close when the adapter
// is no longer needed (e.g. when the component instance is closed).
func (h *Host) BuildAdapter(
	ctx context.Context,
	baseName string,
	caller CallSide,
	callee Callee,
	params, results []Type,
) (*CallAdapter, error) {
	// 1. Compute flat param/result counts for the adapter's core ABI signature.
	flatParamCount := 0
	for _, p := range params {
		flatParamCount += len(flattenTypeOf(p))
	}
	flatResultCount := 0
	for _, r := range results {
		flatResultCount += len(flattenTypeOf(r))
	}

	// 2. Decide adapter signature per canonical-ABI rules
	// (flat vs single-i32 indirect based on maxFlatParams/maxFlatResults).
	var adapterParamTypes []api.ValueType
	var adapterResultTypes []api.ValueType
	var wasmParams []byte
	var wasmResults []byte

	if flatParamCount <= maxFlatParams {
		for _, p := range params {
			for _, cv := range flattenTypeOf(p) {
				adapterParamTypes = append(adapterParamTypes, coreValueTypeToAPI(cv))
				wasmParams = append(wasmParams, coreValueTypeToWasm(cv))
			}
		}
	} else {
		adapterParamTypes = []api.ValueType{api.ValueTypeI32}
		wasmParams = []byte{wasm.ValI32}
	}

	resultViaMemory := flatResultCount > maxFlatResults
	if resultViaMemory {
		adapterParamTypes = append(adapterParamTypes, api.ValueTypeI32)
		wasmParams = append(wasmParams, wasm.ValI32)
	} else {
		for _, r := range results {
			for _, cv := range flattenTypeOf(r) {
				adapterResultTypes = append(adapterResultTypes, coreValueTypeToAPI(cv))
				wasmResults = append(wasmResults, coreValueTypeToWasm(cv))
			}
		}
	}

	// 3. Compile the transfer plan. calleeInstance drives the same-component
	// shortcut in the transfer visitors (lower_borrow is skipped when the
	// borrow's defining instance equals the callee). Both are Instance
	// interface values, so nil-interface == nil-interface and pointer equality
	// works correctly without any typed-nil/untyped-nil gymnastics.
	plan := compileTransferPlan(params, results, callee.Instance)

	// 4. Compute nCallerFlatParams via flatCount summation.
	var nCallerFlatParams uint32
	for _, p := range params {
		nCallerFlatParams += flatCount(p)
	}

	// 5. Construct the adapterFunc.
	af := &adapterFunc{
		caller:            caller,
		callee:            callee,
		plan:              plan,
		nCallerFlatParams: nCallerFlatParams,
	}

	// 6. Register the adapterFunc as a host module.
	n := adapterCounter.Add(1)
	hostModName := fmt.Sprintf("%s_host_%d", baseName, n)
	hostMod, err := h.runtime.NewHostModuleBuilder(hostModName).
		NewFunctionBuilder().
		WithGoModuleFunction(af, adapterParamTypes, adapterResultTypes).
		Export("adapt").
		Instantiate(ctx)
	if err != nil {
		return nil, fmt.Errorf("wacogo: build adapter: instantiate host module: %w", err)
	}

	// 7. Build the stub via buildStubModule.
	sig := wasm.FuncSig{Params: wasmParams, Results: wasmResults}
	stubInstName := fmt.Sprintf("%s_stub", baseName)
	stubInst, err := buildStubModule(ctx, h.runtime, hostModName, "adapt", sig, stubInstName, "adapt")
	if err != nil {
		_ = hostMod.Close(ctx)
		return nil, fmt.Errorf("wacogo: build adapter: %w", err)
	}

	// 8. Return the CallAdapter.
	return &CallAdapter{
		Module: stubInst,
		Name:   "adapt",
		aux:    []api.Module{hostMod},
	}, nil
}

// adapterFunc is a GoModuleFunction that bridges a cross-component call
// via a pre-compiled transferPlan.
type adapterFunc struct {
	caller            CallSide
	callee            Callee
	plan              *transferPlan
	nCallerFlatParams uint32
}

func (a *adapterFunc) Call(ctx context.Context, mod api.Module, stack []uint64) {
	// Resolve caller memory: use the captured value, falling back to the
	// calling wasm module's memory if not resolved at construction time.
	callerMem := a.caller.Memory
	if callerMem == nil {
		callerMem = mod.Memory()
	}

	callerSide := &transferSide{
		Instance:       a.caller.Instance,
		Memory:         callerMem,
		Realloc:        wrapRealloc(a.caller.Realloc),
		StringEncoding: a.caller.StringEncoding,
		ResourceTable:  a.caller.Instance.ResourceTable(),
	}

	calleeSide := &transferSide{
		Instance:       a.callee.Instance,
		Memory:         a.callee.Memory,
		Realloc:        wrapRealloc(a.callee.Realloc),
		StringEncoding: a.callee.StringEncoding,
		ResourceTable:  a.callee.Instance.ResourceTable(),
	}

	tc := newTransferContext(callerSide, calleeSide, stack)

	runTransferPlan(ctx, a.plan, tc, a.callee.CoreFunc,
		wrapPostReturn(a.callee.PostReturn), a.nCallerFlatParams)
}
