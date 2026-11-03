package canon

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// CallBinding is a Go→component call binding: a compiled gocall plan
// pre-bound to a Callee. Constructed once per exported Func, invoked many
// times.
type CallBinding struct {
	plan       *gocallPlan
	callee     Callee
	side       *transferSide  // precomputed at construction
	postReturn PostReturnFunc // wrapped at construction; nil if callee has no post-return
	core       coreCallable   // callee.Direct or callee.CoreFunc

	// gcc is per-call state reused across Call invocations. Safe because
	// each callee instance enforces single-threaded, non-reentrant access
	// via its reentrance gate. gcc.callee is set once at construction;
	// gcc.Task.NumBorrows is reset at the start of each Call (Task.End
	// drains releases on every path but does not zero NumBorrows).
	gcc gocallContext

	// realloc is side.Realloc with the canonical ABI's checks on its
	// result, poisoning the callee on failure; handed out by Realloc.
	realloc ReallocFunc
}

// NewCallBinding compiles a gocall plan for params/results and captures the
// Callee it will always dispatch against. maxLiftBytes bounds the bytes
// each result lift may materialize (0 means unlimited). The transferSide
// and wrapped post-return are precomputed here so each Call avoids those
// per-invocation allocations.
func NewCallBinding(params, results []Type, callee Callee, maxLiftBytes uint64) *CallBinding {
	side := &transferSide{
		Instance:       callee.Instance,
		Memory:         callee.Memory,
		Realloc:        callee.realloc(),
		StringEncoding: callee.StringEncoding,
		ResourceTable:  callee.Instance.ResourceTable(),
	}
	cb := &CallBinding{
		plan:       compileGocallPlan(params, results),
		callee:     callee,
		side:       side,
		postReturn: wrapPostReturn(callee.PostReturn),
		core:       callee.core(),
		realloc:    checkedRealloc(callee.Instance, callee.Memory, side.Realloc),
	}
	cb.gcc.callee = side
	cb.gcc.budget.limit = maxLiftBytes
	return cb
}

// Realloc returns the callee's realloc, for CallRaw write callbacks that
// lower into its memory, or nil if the callee has none. A pointer that is
// null, misaligned or out of bounds is reported as an error, and any
// failure poisons the callee as a trap in its realloc would.
func (cb *CallBinding) Realloc() ReallocFunc { return cb.realloc }

// Callee returns the captured callee descriptor by value. Used by root-side
// code that needs the underlying memory/realloc/CoreFunc (e.g. adapter
// construction that treats this Func as a target in a cross-component call).
func (cb *CallBinding) Callee() Callee { return cb.callee }

// Call executes a Go→component call. Traps raised inside the canonical-ABI
// pipeline are recovered and returned as errors; no *Trap sentinel leaks
// past this boundary.
func (cb *CallBinding) Call(ctx context.Context, args []Val) (results []Val, err error) {
	defer func() {
		if r := recover(); r != nil {
			if t, ok := r.(*Trap); ok {
				err = t.asError()
				return
			}
			panic(r)
		}
	}()

	cb.gcc.Task.NumBorrows = 0
	return runGocallPlan(ctx, cb.plan, &cb.gcc, args, cb.core, cb.postReturn)
}

// CallRaw invokes the function using caller-supplied flat-stack closures,
// bypassing the gocall plan's Val lift/lower steps. writeArgs populates the
// stack before the wasm call; readResult reads from it after, charging
// budget for what it lifts. If writeArgs fails, the call is not made. If
// readResult fails, post-return is skipped and the callee is poisoned.
//
// The reentrance gate and post-return hook are still honoured. No resource
// table or realloc operations are performed — the caller is responsible for
// all encoding.
func (cb *CallBinding) CallRaw(
	ctx context.Context,
	writeArgs func(stack []uint64) error,
	readResult func(stack []uint64, budget *LiftBudget) error,
) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if t, ok := r.(*Trap); ok {
				err = t.asError()
				return
			}
			panic(r)
		}
	}()

	if err := cb.callee.Instance.Enter(ctx); err != nil {
		return err
	}
	defer cb.callee.Instance.Exit(ctx)

	stack := cb.plan.coreStack
	clear(stack)
	// Lowering runs the callee's realloc, which may not leave the
	// component, as in runGocallPlan and runTransferPlan.
	prev := cb.callee.Instance.SuspendLeave()
	err = writeArgs(stack)
	cb.callee.Instance.RestoreLeave(prev)
	if err != nil {
		return err
	}

	if err = cb.core.CallWithStack(ctx, stack); err != nil {
		cb.callee.Instance.Poison(err)
		return err
	}

	if err = cb.readResultAndPostReturn(ctx, stack, readResult); err != nil {
		cb.callee.Instance.Poison(err)
		return err
	}
	return nil
}

// readResultAndPostReturn runs readResult and then the callee's
// post-return. A panic escaping either poisons the callee before
// propagating; error returns are poisoned by the caller.
func (cb *CallBinding) readResultAndPostReturn(ctx context.Context, stack []uint64, readResult func(stack []uint64, budget *LiftBudget) error) error {
	defer poisonOnPanic(cb.callee.Instance)
	if err := ctx.Err(); err != nil {
		return err
	}
	// The binding is single-threaded per callee, like the Call path that
	// shares gcc, so its budget can be reused.
	cb.gcc.budget.reset()
	if err := readResult(stack, &cb.gcc.budget); err != nil {
		return err
	}
	if cb.postReturn == nil {
		return nil
	}
	// Post-return is cleanup only: it runs under may_leave=false, as in
	// runGocallPlan and runTransferPlan.
	prev := cb.callee.Instance.SuspendLeave()
	defer cb.callee.Instance.RestoreLeave(prev)
	return cb.postReturn(ctx, stack)
}

// checkedRealloc wraps r with callRealloc's checks on the returned
// pointer, reporting their traps as errors, and poisons inst when r fails
// or breaks those checks. Returns nil if r is nil.
func checkedRealloc(inst Instance, mem api.Memory, r ReallocFunc) ReallocFunc {
	if r == nil {
		return nil
	}
	return func(ctx context.Context, origPtr, origSize, align, newSize uint32) (ptr uint32, err error) {
		defer func() {
			if rec := recover(); rec != nil {
				t, ok := rec.(*Trap)
				if !ok {
					panic(rec)
				}
				err = t.asError()
			}
			if err != nil {
				inst.Poison(err)
			}
		}()
		return callRealloc(ctx, r, mem, origPtr, origSize, align, newSize)
	}
}

// wrapRealloc adapts an api.Function realloc into ReallocFunc, or nil.
// The returned closure uses CallWithStack with a pre-allocated stack so
// every realloc invocation is allocation-free. Safe under the per-instance
// single-threaded invariant — the stack is shared across invocations of
// this closure but never concurrent.
func wrapRealloc(f api.Function) ReallocFunc {
	if f == nil {
		return nil
	}
	// Realloc sig: (i32 origPtr, i32 origSize, i32 align, i32 newSize) -> i32.
	// CallWithStack stack length must be max(numParams, numResults) = 4.
	stack := make([]uint64, 4)
	return func(ctx context.Context, origPtr, origSize, align, newSize uint32) (uint32, error) {
		stack[0] = uint64(origPtr)
		stack[1] = uint64(origSize)
		stack[2] = uint64(align)
		stack[3] = uint64(newSize)
		if err := f.CallWithStack(ctx, stack); err != nil {
			return 0, err
		}
		return uint32(stack[0]), nil
	}
}

// wrapPostReturn adapts an api.Function post-return into PostReturnFunc, or nil.
// Uses CallWithStack so the call is allocation-free; the caller-supplied
// results slice doubles as the wazero call stack (post-return params are
// the function's flat result types, no results returned).
func wrapPostReturn(f api.Function) PostReturnFunc {
	if f == nil {
		return nil
	}
	return func(ctx context.Context, results []uint64) error {
		return f.CallWithStack(ctx, results)
	}
}
