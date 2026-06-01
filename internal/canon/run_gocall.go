package canon

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// runGocallPlan executes a compiled gocallPlan for a Go→component call
// (Func.Call). Returns the lifted result []Val or an error. No panic/recover
// boundary — every failure flows through error returns.
func runGocallPlan(
	ctx context.Context,
	plan *gocallPlan,
	gcc *gocallContext,
	args []Val,
	calleeFn api.Function,
	postReturn PostReturnFunc,
) (results []Val, err error) {
	// Returns are named so the borrow-trap defer below can write err.
	if len(args) != len(plan.paramSteps) {
		return nil, fmt.Errorf("expected %d args, got %d", len(plan.paramSteps), len(args))
	}

	// 1. Enter callee (reentrance lock).
	if err := gcc.callee.Instance.Enter(ctx); err != nil {
		return nil, err
	}
	defer gcc.callee.Instance.Exit(ctx)

	// Borrow-trap & release defer. gcc.Task.End() drains per-call release
	// closures (caller-side unlends from LendTo) and returns an error if
	// any callee-side borrows are still live. Method-bound defer (rather
	// than a func literal) so escape analysis keeps gcc on the caller's
	// stack frame.
	defer gcc.endTask(&err)

	// 2-3. Lower params under may_leave=false. The callee's realloc may
	// be invoked here (param block, plus string/list buffers); spec
	// requires that realloc cannot make outgoing calls (resource.new,
	// resource.drop, canon.lower, ...) while a lowering is in flight.
	//
	// plan.coreStack is the buffer we hand to CallWithStack below: params
	// in at slots 0..nParamRegs, results overwrite slots 0..nResultRegs
	// after the call. Zero before use — variant flat encodings leave
	// unused joined-payload slots untouched, and canon ABI requires
	// those to be zero on the wire. Sized to max(nParamRegs, nResultRegs)
	// at plan-compile time.
	stack := plan.coreStack
	clear(stack)
	gcc.registers = stack
	var paramBase uint32
	if err := func() error {
		prev := gcc.callee.Instance.SuspendLeave()
		defer gcc.callee.Instance.RestoreLeave(prev)

		if plan.paramMemSize > 0 {
			ptr, rerr := callRealloc(ctx, gcc.callee.Realloc, gcc.callee.Memory, 0, 0, plan.paramMaxAlign, plan.paramMemSize)
			if rerr != nil {
				return rerr
			}
			paramBase = ptr
			stack[0] = uint64(ptr)
		}

		for i, step := range plan.paramSteps {
			if serr := step(ctx, gcc, args[i], paramBase); serr != nil {
				return serr
			}
		}
		return nil
	}(); err != nil {
		return nil, err
	}

	// 4. Invoke the callee core function. CallWithStack reuses the
	// same buffer for params and results — no per-call allocation in
	// wazero. If wasm traps here, the callee instance's state is
	// unreliable: poison it before returning so subsequent Enter calls
	// see the trap reason.
	if cerr := calleeFn.CallWithStack(ctx, stack); cerr != nil {
		gcc.callee.Instance.Poison(cerr)
		return nil, cerr
	}

	// 5. Lift results. Flat-mode steps read gcc.registers (= stack);
	// mem-mode steps read callee.Memory at base+off.
	var resultBase uint32
	if plan.nResultRegs > 0 {
		resultBase = uint32(stack[0])
	}
	if plan.returnMem {
		if err := checkAlignment(resultBase, plan.resultMaxAlign); err != nil {
			return nil, err
		}
	}
	results = make([]Val, len(plan.resultSteps))
	for i, step := range plan.resultSteps {
		v, err := step(ctx, gcc, resultBase)
		if err != nil {
			return nil, err
		}
		results[i] = v
	}

	// 6. Post-return hook. Runs callee wasm under may_leave=false per spec
	// (post-return is a cleanup-only window, no outgoing calls allowed).
	if postReturn != nil {
		prev := gcc.callee.Instance.SuspendLeave()
		err = postReturn(ctx, stack[:plan.nResultRegs])
		gcc.callee.Instance.RestoreLeave(prev)
		if err != nil {
			return nil, err
		}
	}

	// 7. Exit callee — fires via defer above.
	return results, nil
}
