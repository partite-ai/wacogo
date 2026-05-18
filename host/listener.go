package host

import "context"

// CallKind identifies the kind of host-side invocation a CallListener
// observes.
type CallKind uint8

const (
	// CallKindFunction is a normal host-component function invocation.
	CallKindFunction CallKind = iota + 1
	// CallKindDestructor is a resource destructor invocation. The
	// destructor runs after the resource has been released from the
	// extern table; name is the resource's component-level export name
	// and stack is a one-element slice holding the canonical rep.
	CallKindDestructor
)

// CallListener observes host-component function and destructor
// invocations on a ComponentInstance. Attach one via WithCallListener
// at Instantiate time.
//
// Both methods run synchronously on the call goroutine. Implementations
// must not mutate stack. AfterCall is guaranteed to run for every
// BeforeCall, even if the underlying handler panics; in that case err
// reports the recovered panic value (wrapped in a generated error when
// the panic value is not itself an error).
type CallListener interface {
	BeforeCall(ctx context.Context, h *ComponentInstance, kind CallKind, name string, stack []uint64)
	AfterCall(ctx context.Context, h *ComponentInstance, kind CallKind, name string, stack []uint64, err error)
}
