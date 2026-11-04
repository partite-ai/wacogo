package canon

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// BuildAdapter constructs a cross-component adapter for a single lowered
// function: the Go transfer function and its core signature, for the caller to
// put in a host module.
//
// Call Close on the returned *CallAdapter when the adapter is no longer needed
// (e.g. when the component instance is closed).
func (h *Host) BuildAdapter(
	ctx context.Context,
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

	if flatParamCount <= maxFlatParams {
		for _, p := range params {
			for _, cv := range flattenTypeOf(p) {
				adapterParamTypes = append(adapterParamTypes, coreValueTypeToAPI(cv))
			}
		}
	} else {
		adapterParamTypes = []api.ValueType{api.ValueTypeI32}
	}

	resultViaMemory := flatResultCount > maxFlatResults
	if resultViaMemory {
		adapterParamTypes = append(adapterParamTypes, api.ValueTypeI32)
	} else {
		for _, r := range results {
			for _, cv := range flattenTypeOf(r) {
				adapterResultTypes = append(adapterResultTypes, coreValueTypeToAPI(cv))
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
	//
	// Precompute the callee side (memory, realloc, resource table, etc.)
	// once here; it never varies across calls. Also precompute the wrapped
	// post-return.
	//
	// The caller side is precomputed when caller.Memory is known at
	// construction. When it is nil, the caller's memory is resolved per
	// call from the calling wasm module (mod.Memory()); in that case we
	// cache the non-memory parts on callerSideTpl and rebuild the side
	// per call.
	calleeSide := &transferSide{
		Instance:       callee.Instance,
		Memory:         callee.Memory,
		Realloc:        callee.realloc(),
		StringEncoding: callee.StringEncoding,
		ResourceTable:  callee.Instance.ResourceTable(),
	}
	// caller.Memory is nil iff the corresponding canon-lower had no
	// (memory ...) option — i.e. the signature doesn't transfer any
	// indirect data (strings/lists). The transfer step closures only
	// dereference caller Memory when they need to memcpy such data,
	// so a nil Memory is correct here for primitive-only signatures.
	callerSide := &transferSide{
		Instance:       caller.Instance,
		Memory:         caller.Memory,
		Realloc:        caller.realloc(),
		StringEncoding: caller.StringEncoding,
		ResourceTable:  caller.Instance.ResourceTable(),
	}
	// Per-adapter batched-realloc helpers. One per side that has a
	// realloc with a known wazero module/export name. The helpers let
	// transfer steps that would otherwise issue N per-element realloc
	// calls (e.g. list<string>) collapse them into a single Go↔wasm
	// crossing; the loop calling cabi_realloc happens entirely in wasm.
	// Construction is deferred to a helper because errors here must be
	// surfaced as adapter-build failures, not transfer-time panics.
	var reallocHelpers []*batchReallocHelper
	// A Go allocator needs no batching: each allocation is a Go call.
	if callee.ReallocModName != "" && callee.GoRealloc == nil {
		helper, err := h.buildBatchReallocHelper(ctx, callee.ReallocModName, callee.ReallocFnExport)
		if err != nil {
			return nil, fmt.Errorf("wacogo: build callee batch-realloc helper: %w", err)
		}
		calleeSide.BatchHelper = helper
		reallocHelpers = append(reallocHelpers, helper)
	}
	if caller.ReallocModName != "" && caller.GoRealloc == nil {
		helper, err := h.buildBatchReallocHelper(ctx, caller.ReallocModName, caller.ReallocFnExport)
		if err != nil {
			// Best-effort teardown of the callee helper before bubbling up.
			for _, built := range reallocHelpers {
				_ = built.Close(ctx)
			}
			return nil, fmt.Errorf("wacogo: build caller batch-realloc helper: %w", err)
		}
		callerSide.BatchHelper = helper
		reallocHelpers = append(reallocHelpers, helper)
	}
	af := &adapterFunc{
		caller:            caller,
		callee:            callee,
		plan:              plan,
		nCallerFlatParams: nCallerFlatParams,
		callerSide:        callerSide,
		calleeSide:        calleeSide,
		postReturn:        wrapPostReturn(callee.PostReturn),
		core:              callee.core(),
	}
	af.tc.callee = calleeSide
	af.tc.budget.limit = h.maxLiftBytes

	// 6. Return the CallAdapter. Any batch-realloc helpers are closed when the
	// adapter is closed.
	return &CallAdapter{
		Func:           af,
		Params:         adapterParamTypes,
		Results:        adapterResultTypes,
		reallocHelpers: reallocHelpers,
	}, nil
}

// adapterFunc is a GoModuleFunction that bridges a cross-component call
// via a pre-compiled transferPlan.
type adapterFunc struct {
	caller            CallSide
	callee            Callee
	plan              *transferPlan
	nCallerFlatParams uint32

	// Precomputed at BuildAdapter time.
	callerSide *transferSide
	calleeSide *transferSide
	postReturn PostReturnFunc
	core       coreCallable

	// tc is per-call state reused across Call invocations. Safe because
	// the callee's reentrance gate serialises calls and forbids re-entry
	// of the same instance through the same adapter. tc.callee is set
	// once at construction; tc.caller is rebound per call (the result-
	// phase side swap leaves caller/callee swapped); tc.registers is
	// rebound to the wazero host-callback stack each call;
	// tc.Task.NumBorrows is reset (Task.End drains releases on every
	// path but does not zero NumBorrows).
	tc transferContext
}

func (a *adapterFunc) Call(ctx context.Context, mod api.Module, stack []uint64) {
	if !a.caller.Instance.CanLeave() {
		trapf("cannot leave component instance")
	}
	a.tc.caller = a.callerSide
	a.tc.callee = a.calleeSide
	a.tc.registers = stack
	a.tc.Task.NumBorrows = 0
	runTransferPlan(ctx, a.plan, &a.tc, a.core,
		a.postReturn, a.nCallerFlatParams, mod)
}
