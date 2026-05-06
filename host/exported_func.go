package host

import (
	"context"

	"github.com/partite-ai/wacogo/internal/core"
)

// ExportedFunc is a host-callable wrapper around an exported component
// function. Use it to invoke a component export from a host Func body
// with full canonical-ABI lift/lower control.
type ExportedFunc struct {
	fn *core.ExportedFunc
}

// WrapExportedFunc returns an ExportedFunc that invokes fn. fn must
// be an export taken from a live ComponentInstance.
func WrapExportedFunc(fn *core.ExportedFunc) *ExportedFunc {
	return &ExportedFunc{fn: fn}
}

// ParamType returns the component-model type of the i-th parameter
// of the wrapped function's signature. Panics if i is out of range.
func (f *ExportedFunc) ParamType(i int) core.Type {
	return f.fn.Type().Params[i].Type
}

// ResultType returns the component-model type of the i-th result of
// the wrapped function's signature. Panics if i is out of range.
func (f *ExportedFunc) ResultType(i int) core.Type {
	return f.fn.Type().Results[i].Type
}

// CallRaw invokes the wrapped function. write is called before the
// call to populate stack with flat arguments; read is called after to
// consume flat results. Each callback receives two CallContext values
// — caller for the host instance issuing the call and callee for the
// instance providing the export.
//
// CallRaw returns an error if any borrows handed to the callee were
// not dropped before the call returned.
func (f *ExportedFunc) CallRaw(
	ctx context.Context,
	caller *ComponentInstance,
	write func(caller, callee *CallContext, stack []uint64),
	read func(caller, callee *CallContext, stack []uint64),
) error {
	return f.fn.CallRaw(ctx, caller.Core(), write, read)
}
