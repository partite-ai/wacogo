package core

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo/internal/canon"
	"github.com/partite-ai/wacogo/wasmparser"
	"github.com/tetratelabs/wazero/api"
)

// Func is a callable component-level function. It holds only the information
// needed to invoke the function; the name under which it is exported (and the
// wasmparser type handle used for instantiation type-checking) live on
// ExportedFunc.
type Func struct {
	funcType *FuncType
	binding  *canon.CallBinding
}

// Type returns the component model function type.
func (f *Func) Type() *FuncType { return f.funcType }

// Call invokes the function.
func (f *Func) Call(ctx context.Context, args ...Val) ([]Val, error) {
	if f.funcType == nil {
		return nil, fmt.Errorf("wacogo: call: function has no type information")
	}
	if f.binding == nil {
		return nil, fmt.Errorf("wacogo: call: missing canon call binding")
	}
	if len(args) != len(f.funcType.Params) {
		return nil, fmt.Errorf("wacogo: call: expected %d args, got %d",
			len(f.funcType.Params), len(args))
	}
	return f.binding.Call(ctx, args)
}

// CallRaw invokes the core function backing this component function
// with parameters that have already been lowered into the caller
// instance according to the canonical ABI. write is invoked once with
// a freshly-sized stack slice into which the caller writes the flat
// encoding of the args; read is invoked once with the post-call stack
// to decode the flat results. Both callbacks receive a caller-side
// CallContext (bound to the supplied caller instance) and a
// callee-side CallContext (bound to the instance exporting this
// function).
//
// Highly advanced operation. Most code should call Call instead, which
// performs arity and type validation around the canonical-ABI
// transcoder. CallRaw bypasses both — the caller is fully responsible
// for matching the flat encoding of the function's params and results.
//
// The stack slice has length max(nParamRegs, nResultRegs); the same
// slice carries the args in and the results out, per wazero's
// CallWithStack semantics. Returns an error if any borrows handed to
// the callee remain outstanding when the call returns.
func (f *Func) CallRaw(
	ctx context.Context,
	caller *ComponentInstance,
	write func(caller, callee *CallContext, stack []uint64),
	read func(caller, callee *CallContext, stack []uint64),
) (err error) {
	if f.funcType == nil {
		return fmt.Errorf("wacogo: CallRaw: function has no type information")
	}
	if f.binding == nil {
		return fmt.Errorf("wacogo: CallRaw: missing canon call binding")
	}

	callee := f.binding.Callee()
	task := &canon.Task{}
	callerCC := NewCallContext(caller, task, nil, nil)
	realloc := ReallocFunc(callee.GoRealloc)
	if realloc == nil {
		realloc = wrapAPIRealloc(callee.Realloc)
	}
	calleeCC := NewCallContext(
		instanceFromCanon(callee.Instance),
		task,
		callee.Memory,
		realloc,
	)

	defer func() {
		if endErr := task.End(); endErr != nil && err == nil {
			err = endErr
		}
	}()

	return f.binding.CallRaw(ctx,
		func(stack []uint64) { write(callerCC, calleeCC, stack) },
		func(stack []uint64) { read(callerCC, calleeCC, stack) },
	)
}

// wrapAPIRealloc adapts a wazero realloc export into ReallocFunc.
// Returns nil if fn is nil.
func wrapAPIRealloc(fn api.Function) ReallocFunc {
	if fn == nil {
		return nil
	}
	return func(ctx context.Context, origPtr, origSize, align, newSize uint32) (uint32, error) {
		out, err := fn.Call(ctx, uint64(origPtr), uint64(origSize), uint64(align), uint64(newSize))
		if err != nil {
			return 0, err
		}
		return uint32(out[0]), nil
	}
}

// instanceFromCanon recovers the *ComponentInstance backing a canon.Instance
// previously produced by InstanceAsCanon. Returns nil if the canon.Instance
// is not a canonInstanceView.
func instanceFromCanon(ci canon.Instance) *ComponentInstance {
	if v, ok := ci.(canonInstanceView); ok {
		return v.i
	}
	return nil
}

// ExportedFunc is a Func as it sits at an export/import boundary: a callable
// paired with the name it was exported under and its wasmparser type handle.
// The zero-value Name ("") and nil parser function-type handle are legitimate
// for funcs that have been constructed but not yet wired into a named export.
type ExportedFunc struct {
	*Func
	Name string

	parserFuncType *wasmparser.FuncType
	instance       *ComponentInstance
}

// NewExportedFunc returns an *ExportedFunc with the given name, function
// body, and wasmparser function-type handle. parserFuncType may be nil
// for funcs that have not been linked to a parser-validated component.
func NewExportedFunc(name string, fn *Func, parserFuncType *wasmparser.FuncType) *ExportedFunc {
	return &ExportedFunc{Func: fn, Name: name, parserFuncType: parserFuncType}
}

// ParserFunctionType returns the wasmparser function-type handle for
// this exported function, or nil if none was attached. Use it for
// extended type information beyond the runtime API.
func (e *ExportedFunc) ParserFunctionType() *wasmparser.FuncType {
	return e.parserFuncType
}

// Instance returns the *ComponentInstance that exports this func, or nil
// if the func has not yet been installed into an instance's export map.
// Populated by ComponentInstance.initExports.
func (e *ExportedFunc) Instance() *ComponentInstance { return e.instance }

// NewFunc constructs a *Func from a component-level *FuncType and a
// prebuilt canon call binding. Intended for external construction
// layers (e.g. the host package) that synthesize Funcs outside the
// standard loader/instantiate path.
func NewFunc(ft *FuncType, binding *canon.CallBinding) *Func {
	return &Func{funcType: ft, binding: binding}
}
