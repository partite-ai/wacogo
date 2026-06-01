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
//   - callerCoreModule: the wasm module that originated the call. Attached
//     to ctx via WithCallerCoreModule so host-function callbacks reached
//     through the callee can recover the wasm caller. May be nil for
//     test contexts.
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
	callerCoreModule api.Module,
) {
	if callerCoreModule != nil {
		ctx = WithCallerCoreModule(ctx, callerCoreModule)
	}
	// 1. Enter callee (reentrance lock). Capture the callee pointer here,
	// before the later side swap, so the deferred Exit always releases the
	// lock on the original callee even after we swap sides for the result
	// phase.
	calleeInst := tc.callee.Instance
	if err := calleeInst.Enter(ctx); err != nil {
		panic(&Trap{msg: err.Error()})
	}
	defer calleeInst.Exit(ctx)

	// Borrow-trap & release defer. tc.Task.End() drains per-call release
	// closures (caller-side unlends from LendTo) and traps if any
	// callee-side borrows are still live. Single source of truth.
	// LIFO ordering: this defer registers AFTER the Exit defer above, so
	// it fires FIRST (releases run, then reentrance lock). Method-bound
	// defer (rather than a func literal) so escape analysis can keep tc
	// on the caller's stack frame.
	defer tc.endTaskOrTrap()

	// 2-4. Lower params into the callee under may_leave=false. The callee's
	// realloc may be invoked here (param block, plus string/list buffers);
	// spec requires that realloc cannot make outgoing calls (resource.new,
	// resource.drop, canon.lower, ...) while a lowering is in flight.
	// plan.coreStack is the buffer we hand to CallWithStack below: params
	// in at slots 0..nParamRegs, results overwrite slots 0..nResultRegs
	// after the call. Zero before use — variant flat encodings leave
	// unused joined-payload slots untouched, and canon ABI requires
	// those to be zero on the wire. Sized to max(nParamRegs, nResultRegs)
	// at plan-compile time.
	calleeStack := plan.coreStack
	clear(calleeStack)
	func() {
		prev := tc.callee.Instance.SuspendLeave()
		defer tc.callee.Instance.RestoreLeave(prev)

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
			calleeStack[0] = uint64(calleePtr)
		}
		// Flat params: steps read from tc.registers and write back to tc.registers
		// (same slice, canonicalization in place). The callee stack is built
		// from tc.registers after param steps run.

		runPhasedTransfer(ctx, tc, plan.paramSteps, paramSrcBase, paramDstBase)

		if plan.paramMemSize == 0 {
			copy(calleeStack, tc.registers[:nCallerFlatParams])
		}
	}()

	// 5. Invoke the callee core function via CallWithStack — reuses
	// calleeStack for both params and results, no per-call alloc in
	// wazero. If wasm traps here, the callee instance's state is
	// unreliable: poison it before re-raising so subsequent calls see
	// the trap reason.
	if cerr := calleeFn.CallWithStack(ctx, calleeStack); cerr != nil {
		tc.callee.Instance.Poison(cerr)
		panic(&Trap{msg: cerr.Error()})
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
		prev := tc.callee.Instance.SuspendLeave()
		defer tc.callee.Instance.RestoreLeave(prev)

		var resultSrcBase, resultDstBase uint32
		if plan.returnMem {
			// Post-swap: tc.caller is the original callee, which returned a
			// single i32 pointer to its memory in calleeStack[0]. We read
			// result data from that pointer.
			calleeResultPtr := uint32(calleeStack[0])
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
			copy(tc.registers, calleeStack[:plan.nResultRegs])
		}

		runPhasedTransfer(ctx, tc, plan.resultSteps, resultSrcBase, resultDstBase)
	}()

	// 9. Post-return hook, if provided. Runs in the original callee's
	// instance (post-swap that's tc.caller) under may_leave=false.
	if postReturn != nil {
		prev := tc.caller.Instance.SuspendLeave()
		err := postReturn(ctx, calleeStack[:plan.nResultRegs])
		tc.caller.Instance.RestoreLeave(prev)
		if err != nil {
			panic(&Trap{msg: err.Error()})
		}
	}

	// 10. Exit callee — fires via defer above.
}

// runPhasedTransfer orchestrates a batched-realloc two-phase walk when
// the callee side has a batch helper, falling back to single-phase serial
// realloc when it doesn't. In the batched path:
//
//  1. Discovery: walk every step's sizeStep (if non-nil), writing each
//     requested (size, align) into the helper's scratch input region.
//  2. Batch: one helper.invoke call performs N internal wasm→wasm
//     cabi_realloc calls, populating scratch's output region with N
//     pointers.
//  3. Transfer: walk every step's transferStep, supplying a
//     batchedAllocSource that hands the pre-allocated pointers back in
//     declaration order.
//
// In the serial fallback: the transfer walk runs once with a
// serialAllocSource that issues cabi_realloc per request, matching the
// original behaviour.
func runPhasedTransfer(ctx context.Context, tc *transferContext, steps []transferPlanStep, srcBase, dstBase uint32) {
	helper := tc.callee.BatchHelper
	// allocSrc and allocSink live on the (heap-allocated, per-adapter)
	// transferContext so their addresses can be safely shared with the
	// indirect closures the discovery and transfer walks invoke — no per-
	// call escape allocation.
	tc.allocSrc.helper = nil
	tc.allocSrc.n = 0
	tc.allocSrc.idx = 0
	if helper != nil {
		tc.allocSink.helper = helper
		tc.allocSink.n = 0
		for _, step := range steps {
			if step.sizes != nil {
				step.sizes(tc, srcBase, &tc.allocSink)
			}
		}
		if tc.allocSink.n > 0 {
			if err := helper.invoke(ctx, tc.allocSink.n); err != nil {
				panic(&Trap{msg: err.Error()})
			}
		}
		tc.allocSrc.helper = helper
		tc.allocSrc.n = tc.allocSink.n
	}
	for _, step := range steps {
		step.transfer(ctx, tc, srcBase, dstBase, &tc.allocSrc)
	}
}
