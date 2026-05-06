package canon

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// runTransferPlan executes a compiled transferPlan for a component→component
// call. Called from adapterFunc's wazero host callback; panics of *Trap
// propagate to wazero unchanged.
//
// Arguments:
//   - plan: compiled transferPlan carrying param/result steps and mode flags.
//   - tc: per-call state. tc.registers is the wazero stack — caller params
//     on entry, callee results on exit.
//   - calleeFn: the callee component's canon-lifted core function.
//   - postReturn: optional callee post-return hook.
//   - nCallerFlatParams: caller-side flat slot count for params (used to
//     locate the canon-lower retptr in flat-params mode).
//
// nCallerFlatParams is derived at construction time by adapterFunc from
// the function's FuncType (flat count) and passed in here.
func runTransferPlan(
	ctx context.Context,
	plan *transferPlan,
	tc *transferContext,
	calleeFn api.Function,
	postReturn PostReturnFunc,
	nCallerFlatParams uint32,
) {
	// 1. Enter callee (reentrance lock). The exit closure is captured
	// here, before the later side swap, so we always release the lock
	// on the original callee even after we swap sides for the result
	// phase.
	calleeExit, err := tc.callee.Instance.Enter(ctx)
	if err != nil {
		panic(&Trap{msg: err.Error()})
	}
	exited := false
	defer func() {
		if !exited {
			calleeExit(ctx)
		}
	}()

	// Borrow-trap & release defer. tc.Task.End() drains per-call release
	// closures (caller-side unlends from LendTo) and returns an error if
	// any callee-side borrows are still live. Single source of truth.
	// LIFO ordering: this defer registers AFTER the Exit defer above, so
	// it fires FIRST (releases run, then reentrance lock).
	defer func() {
		if err := tc.Task.End(); err != nil {
			trapf("%s", err.Error())
		}
	}()

	// 2-4. Lower params into the callee under may_leave=false. The callee's
	// realloc may be invoked here (param block, plus string/list buffers);
	// spec requires that realloc cannot make outgoing calls (resource.new,
	// resource.drop, canon.lower, ...) while a lowering is in flight.
	var calleeCoreArgs []uint64
	func() {
		restore := tc.callee.Instance.SuspendLeave()
		defer restore()

		var paramSrcBase, paramDstBase uint32
		if plan.paramMemSize > 0 {
			// Mem params: the caller's wasm already has its param block allocated
			// in caller.Memory. Its pointer lives at tc.registers[0]. Allocate a
			// matching callee-side block and copy param values through the
			// transfer steps.
			callerParamPtr := uint32(tc.registers[0])
			// Caller must have pushed a param block pointer aligned to the
			// mem-mode param region's max alignment. Spec tests check this
			// trap message via the substring "unaligned pointer".
			mustTransfer(checkAlignment(callerParamPtr, plan.paramMaxAlign))
			calleePtr, err := callRealloc(ctx, tc.callee.Realloc, tc.callee.Memory, 0, 0, plan.paramMaxAlign, plan.paramMemSize)
			if err != nil {
				panic(&Trap{msg: err.Error()})
			}
			paramSrcBase = callerParamPtr
			paramDstBase = calleePtr
			calleeCoreArgs = []uint64{uint64(calleePtr)}
		}
		// Flat params: steps read from tc.registers and write back to tc.registers
		// (same slice, canonicalization in place). calleeCoreArgs is built from
		// tc.registers after param steps run.

		for _, step := range plan.paramSteps {
			step(ctx, tc, paramSrcBase, paramDstBase)
		}

		if plan.paramMemSize == 0 {
			calleeCoreArgs = make([]uint64, nCallerFlatParams)
			copy(calleeCoreArgs, tc.registers[:nCallerFlatParams])
		}
	}()

	// 5. Invoke the callee core function.
	calleeResults, err := calleeFn.Call(ctx, calleeCoreArgs...)
	if err != nil {
		panic(&Trap{msg: err.Error()})
	}

	// 6. Swap sides before result steps.
	//
	// Results flow callee→caller, but the transfer visitors hard-code
	// caller=src, callee=dst. Swap tc.caller ⇄ tc.callee so the existing
	// result-step closures operate in the correct direction without
	// per-visitor direction flags.
	//
	// After this swap:
	//   - tc.caller   = original callee (result data source)
	//   - tc.callee   = original caller (result data destination)
	//
	// The unlend defer (registered above) still works after this swap:
	// each lent entry captured its table pointer at lend time (via e.Table),
	// so it is unaffected by the current tc.caller/tc.callee bindings.
	// Post-return also uses calleeResults directly, not tc.
	tc.caller, tc.callee = tc.callee, tc.caller

	// 7-8. Lower results into the original caller under may_leave=false.
	// Post-swap, tc.callee == original caller — its realloc is the one
	// that may fire here (string/list result buffers).
	func() {
		restore := tc.callee.Instance.SuspendLeave()
		defer restore()

		var resultSrcBase, resultDstBase uint32
		if plan.returnMem {
			// Post-swap: tc.caller is the original callee, which returned a
			// single i32 pointer to its memory in calleeResults[0]. We read
			// result data from that pointer.
			calleeResultPtr := uint32(calleeResults[0])
			// Post-swap: tc.callee is the original caller. Its canon-lower
			// retptr lives at tc.registers[retPtrSlot]: slot 1 if mem-params,
			// otherwise nCallerFlatParams. We write result data into that
			// region.
			var retPtrSlot uint32
			if plan.paramMemSize > 0 {
				retPtrSlot = 1
			} else {
				retPtrSlot = nCallerFlatParams
			}
			callerResultPtr := uint32(tc.registers[retPtrSlot])
			// Both the callee's returned retptr and the caller's preallocated
			// retptr must be aligned to the result region's max alignment. Spec
			// tests check this verbatim via "unaligned pointer".
			mustTransfer(checkAlignment(calleeResultPtr, plan.resultMaxAlign))
			mustTransfer(checkAlignment(callerResultPtr, plan.resultMaxAlign))
			resultSrcBase = calleeResultPtr // read from original callee (now tc.caller)
			resultDstBase = callerResultPtr // write to original caller (now tc.callee)
		} else {
			// Flat results: callee returned register values directly. Copy them
			// into tc.registers so flat-mode result steps can read them. Result
			// steps write to tc.registers too, overwriting — this mirrors the
			// wazero stack convention where the same slice is used for params
			// in, results out.
			copy(tc.registers, calleeResults)
		}

		for _, step := range plan.resultSteps {
			step(ctx, tc, resultSrcBase, resultDstBase)
		}
	}()

	// 9. Post-return hook, if provided. Runs in the original callee's
	// instance (post-swap that's tc.caller) under may_leave=false.
	if postReturn != nil {
		restore := tc.caller.Instance.SuspendLeave()
		err := postReturn(ctx, calleeResults)
		restore()
		if err != nil {
			panic(&Trap{msg: err.Error()})
		}
	}

	// 10. Exit callee (release reentrance lock).
	exited = true
	calleeExit(ctx)
}
