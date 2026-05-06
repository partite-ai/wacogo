package host

import (
	"context"
)

// Func is the body of a host-component function. cc carries the
// per-call context (caller memory, resource-table operations); h is
// the bound ComponentInstance, providing access to UserState and
// resource registration. stack carries the canonical-ABI flat
// arguments on entry and receives the flat results on return.
//
// Returning a non-nil error traps the calling wasm component with the
// error message. A Func is invoked serially.
type Func func(ctx context.Context, cc *CallContext, h *ComponentInstance, stack []uint64) error
