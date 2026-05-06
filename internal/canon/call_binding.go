package canon

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// CallBinding is a Go→component call binding: a compiled gocall plan
// pre-bound to a Callee. Constructed once per exported Func, invoked many
// times.
type CallBinding struct {
	plan   *gocallPlan
	callee Callee
}

// NewCallBinding compiles a gocall plan for params/results and captures the
// Callee it will always dispatch against.
func NewCallBinding(params, results []Type, callee Callee) *CallBinding {
	return &CallBinding{
		plan:   compileGocallPlan(params, results),
		callee: callee,
	}
}

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
				err = fmt.Errorf("%s", t.Error())
				return
			}
			panic(r)
		}
	}()

	side := &transferSide{
		Instance:       cb.callee.Instance,
		Memory:         cb.callee.Memory,
		Realloc:        wrapRealloc(cb.callee.Realloc),
		StringEncoding: cb.callee.StringEncoding,
		ResourceTable:  cb.callee.Instance.ResourceTable(),
	}

	gcc := newGocallContext(side)
	return runGocallPlan(ctx, cb.plan, gcc, args, cb.callee.CoreFunc,
		wrapPostReturn(cb.callee.PostReturn))
}

// CallRaw invokes the function using caller-supplied flat-stack closures,
// bypassing the gocall plan's Val lift/lower steps. writeArgs populates the
// stack before the wasm call; readResult reads from it after.
//
// The reentrance gate and post-return hook are still honoured. No resource
// table or realloc operations are performed — the caller is responsible for
// all encoding.
func (cb *CallBinding) CallRaw(
	ctx context.Context,
	writeArgs func(stack []uint64),
	readResult func(stack []uint64),
) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if t, ok := r.(*Trap); ok {
				err = fmt.Errorf("%s", t.Error())
				return
			}
			panic(r)
		}
	}()

	exit, err := cb.callee.Instance.Enter(ctx)
	if err != nil {
		return err
	}
	exited := false
	defer func() {
		if !exited {
			exit(ctx)
		}
	}()

	n := cb.plan.nParamRegs
	if cb.plan.nResultRegs > n {
		n = cb.plan.nResultRegs
	}
	stack := make([]uint64, n)
	writeArgs(stack)

	if err = cb.callee.CoreFunc.CallWithStack(ctx, stack); err != nil {
		return err
	}

	readResult(stack)

	if pr := wrapPostReturn(cb.callee.PostReturn); pr != nil {
		if err = pr(ctx, stack); err != nil {
			return err
		}
	}

	exited = true
	exit(ctx)
	return nil
}

// wrapRealloc adapts an api.Function realloc into ReallocFunc, or nil.
func wrapRealloc(f api.Function) ReallocFunc {
	if f == nil {
		return nil
	}
	return func(ctx context.Context, origPtr, origSize, align, newSize uint32) (uint32, error) {
		out, err := f.Call(ctx, uint64(origPtr), uint64(origSize), uint64(align), uint64(newSize))
		if err != nil {
			return 0, err
		}
		if len(out) < 1 {
			return 0, fmt.Errorf("realloc returned no result")
		}
		return uint32(out[0]), nil
	}
}

// wrapPostReturn adapts an api.Function post-return into PostReturnFunc, or nil.
func wrapPostReturn(f api.Function) PostReturnFunc {
	if f == nil {
		return nil
	}
	return func(ctx context.Context, results []uint64) error {
		_, err := f.Call(ctx, results...)
		return err
	}
}
