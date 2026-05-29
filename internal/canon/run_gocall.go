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
	calleeExit, err := gcc.callee.Instance.Enter(ctx)
	if err != nil {
		return nil, err
	}
	exited := false
	defer func() {
		if !exited {
			calleeExit(ctx)
		}
	}()

	// Borrow-trap & release defer. gcc.Task.End() drains per-call release
	// closures (caller-side unlends from LendTo) and returns an error if
	// any callee-side borrows are still live.
	defer func() {
		if endErr := gcc.Task.End(); endErr != nil && err == nil {
			err = endErr
		}
	}()

	// 2-3. Lower params under may_leave=false. The callee's realloc may
	// be invoked here (param block, plus string/list buffers); spec
	// requires that realloc cannot make outgoing calls (resource.new,
	// resource.drop, canon.lower, ...) while a lowering is in flight.
	var coreArgs []uint64
	if err := func() error {
		restore := gcc.callee.Instance.SuspendLeave()
		defer restore()

		var paramBase uint32
		if plan.paramMemSize > 0 {
			ptr, rerr := callRealloc(ctx, gcc.callee.Realloc, gcc.callee.Memory, 0, 0, plan.paramMaxAlign, plan.paramMemSize)
			if rerr != nil {
				return rerr
			}
			paramBase = ptr
			coreArgs = []uint64{uint64(ptr)}
		} else {
			coreArgs = make([]uint64, plan.nParamRegs)
			gcc.registers = coreArgs
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

	// 4. Invoke the callee core function. If wasm traps here, the
	// callee instance's state is unreliable: poison it before returning
	// so subsequent Enter calls see the trap reason.
	var coreResults []uint64
	coreResults, err = calleeFn.Call(ctx, coreArgs...)
	if err != nil {
		gcc.callee.Instance.Poison(err)
		return nil, err
	}

	// 5. Lift results. Flat-mode steps ignore the base arg and read from
	// gcc.registers; mem-mode steps read from callee.Memory at base+off.
	gcc.registers = coreResults
	var resultBase uint32
	if len(coreResults) > 0 {
		resultBase = uint32(coreResults[0])
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
		restore := gcc.callee.Instance.SuspendLeave()
		err = postReturn(ctx, coreResults)
		restore()
		if err != nil {
			return nil, err
		}
	}

	// 7. Exit callee.
	exited = true
	calleeExit(ctx)
	return results, nil
}
