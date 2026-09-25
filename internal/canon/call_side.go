package canon

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/tetratelabs/wazero/api"
)

// CallSide describes one side of a canonical-ABI call: a component instance
// plus its memory/realloc/encoding options. Used by Callee and by transfer
// factories.
//
// Instance must be non-nil. Every call has a defining component instance
// on each side; callers that don't need reentrance gating or a resource
// table for tests can supply a no-op stub.
type CallSide struct {
	Instance       Instance
	Memory         api.Memory
	Realloc        api.Function
	StringEncoding StringEncoding

	// GoRealloc, when set, is used in place of Realloc: an allocator
	// implemented in Go (a host component's staging memory), so that
	// allocating does not call into wasm. Realloc may still be set, for
	// callers that need a wasm function.
	GoRealloc ReallocFunc

	// ReallocModName is the wazero module name (set at instantiation time
	// via WithName) of the module that exports Realloc. Used by
	// Host.BuildAdapter to wire batched-realloc helper imports — the
	// helper imports "tgt"."<name>" and wazero resolves "tgt" against
	// this module name. Optional: when empty, no batched-realloc helper
	// is built for this side, and the transfer takes the per-element
	// fallback path.
	ReallocModName string

	// ReallocFnExport is the export name of Realloc within
	// ReallocModName. Typically "cabi_realloc" or "realloc". Required
	// when ReallocModName is set.
	ReallocFnExport string
}

// Callee extends CallSide with the concrete core function to invoke and an
// optional post-return hook. Constructed by callers (root or test code) and
// handed to NewCallBinding / Host.BuildAdapter.
type Callee struct {
	CallSide
	CoreFunc   api.Function
	PostReturn api.Function // optional; nil if the callee has no post-return

	// Direct, when set, runs the callee in Go instead of calling CoreFunc:
	// for host components, whose functions are implemented in Go, it
	// skips the round trip through wazero (and, with close-on-context-done,
	// the goroutine wazero starts for every call). It must behave as
	// CoreFunc would.
	Direct DirectFunc
}

// DirectFunc runs a Go-implemented core function. stack is the core
// param/result stack, as for api.Function.CallWithStack. It reports
// failure by panicking, as a wazero host function does.
type DirectFunc func(ctx context.Context, stack []uint64)

// CallWithStack runs d, turning a panic into the error wazero would have
// returned for it.
func (d DirectFunc) CallWithStack(ctx context.Context, stack []uint64) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = recoveredError(r)
		}
	}()
	d(ctx, stack)
	return nil
}

func recoveredError(r any) error {
	switch e := r.(type) {
	case runtime.Error:
		// A bug in the host function: keep where it happened.
		return fmt.Errorf("%w (recovered by wacogo)\n\nGo runtime stack trace:\n%s", e, debug.Stack())
	case error:
		return e
	}
	return fmt.Errorf("%v", r)
}

// coreCallable is what the run functions invoke for the callee's core
// function: an api.Function, or a DirectFunc.
type coreCallable interface {
	CallWithStack(ctx context.Context, stack []uint64) error
}

// core returns what to invoke for c's core function.
func (c *Callee) core() coreCallable {
	if c.Direct != nil {
		return c.Direct
	}
	return c.CoreFunc
}

// realloc returns s's allocator: GoRealloc, or Realloc wrapped.
func (s *CallSide) realloc() ReallocFunc {
	if s.GoRealloc != nil {
		return s.GoRealloc
	}
	return wrapRealloc(s.Realloc)
}
