package fixturetests_test

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/counterhost"
)

// TestNewCounterHandle_Unbound verifies that NewCounterHandle returns
// an unbound handle: methods dispatch through the embedded local impl
// without crossing any canon boundary, and Drop is a no-op (idempotent).
func TestNewCounterHandle_Unbound(t *testing.T) {
	ctx := context.Background()
	impl := &handleTestCounter{value: 42}
	h := counterhost.NewCounterHandle(impl)
	if h == nil {
		t.Fatal("NewCounterHandle returned nil")
	}
	got, err := h.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if got != 42 {
		t.Errorf("Current() pre-bind: got %d, want 42", got)
	}
	if err := h.Increment(ctx); err != nil {
		t.Fatalf("Increment: %v", err)
	}
	got, err = h.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if got != 43 {
		t.Errorf("Current() after Increment(): got %d, want 43", got)
	}
	// Drop on unbound is a no-op; idempotent.
	if err := h.Drop(ctx); err != nil {
		t.Errorf("Drop() on unbound: unexpected error %v", err)
	}
	if err := h.Drop(ctx); err != nil {
		t.Errorf("Drop() on unbound (idempotent): unexpected error %v", err)
	}
}

// TestWrapInstance_NewCounter_BoundHandle exercises the cross-instance
// own-result path: wrapped.NewCounter calls into the callee instance,
// which mints an own handle in its canon table; the wrap.go trampoline
// re-issues it on the caller side and returns a *CounterHandle whose
// state is a boundHandleState. We observe the binding indirectly: the
// returned handle must be non-nil, methods on it must round-trip
// correctly through the remote counter, and Drop must succeed.
func TestWrapInstance_NewCounter_BoundHandle(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := counterhost.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	inst, err := fac.NewInstance(ctx, &handleTestHost{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer inst.Close(ctx)

	wrapped := counterhost.WrapInstance(inst, inst.Core())
	h, err := wrapped.NewCounter(ctx, 100)
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}
	if h == nil {
		t.Fatal("NewCounter returned nil")
	}

	// Round-trip a method through the remote counter to confirm the
	// handle is actually bound to inst's canon table.
	got, err := h.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if got != 100 {
		t.Errorf("Current() on remote handle: got %d, want 100", got)
	}
	if err := h.Increment(ctx); err != nil {
		t.Fatalf("Increment: %v", err)
	}
	got, err = h.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if got != 101 {
		t.Errorf("Current() after Increment: got %d, want 101", got)
	}

	// Drop releases the own entry through the captured cc.Drop closure.
	if err := h.Drop(ctx); err != nil {
		t.Errorf("Drop() on bound handle: unexpected error %v", err)
	}
	// Idempotent: second Drop is a no-op.
	if err := h.Drop(ctx); err != nil {
		t.Errorf("Drop() on bound handle (idempotent): unexpected error %v", err)
	}
}

// TestLocalImpl asserts the contract for *CounterHandle.LocalImpl:
// for any handle whose Go impl is reachable in this process, LocalImpl
// returns (impl, true). The wrap-path remote handle minted by
// wrapped.NewCounter is defined inside the wrapped instance, so its
// rep resolves in that instance's extern table — LocalImpl must hand
// back the *handleTestCounter constructed by the host-side handler.
func TestLocalImpl(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := counterhost.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	inst, err := fac.NewInstance(ctx, &handleTestHost{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer inst.Close(ctx)

	wrapped := counterhost.WrapInstance(inst, inst.Core())
	rh, err := wrapped.NewCounter(ctx, 1)
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}
	if rh == nil {
		t.Fatal("NewCounter returned nil")
	}

	rImpl, ok := rh.LocalImpl()
	if !ok {
		t.Fatal("LocalImpl on wrap-path remote handle: ok=false, want true")
	}
	// The host-side NewCounter handler constructs a fresh *handleTestCounter
	// with value=1; wrap-path lift should hand that same impl back to us.
	hc, isHC := rImpl.(*handleTestCounter)
	if !isHC {
		t.Fatalf("LocalImpl returned %T, want *handleTestCounter", rImpl)
	}
	if got, err := hc.Current(ctx); err != nil || got != 1 {
		t.Errorf("LocalImpl impl.Current() = (%d, %v), want (1, nil)", got, err)
	}
	if err := rh.Drop(ctx); err != nil {
		t.Errorf("Drop on remote handle: %v", err)
	}
}

// handleTestCounter is a minimal Counter impl used by the unbound test.
type handleTestCounter struct{ value uint32 }

func (c *handleTestCounter) Current(ctx context.Context) (uint32, error) { return c.value, nil }
func (c *handleTestCounter) Increment(ctx context.Context) error         { c.value++; return nil }

// handleTestHost is a minimal Counterhost impl used by the wrap-instance
// test. NewCounter returns an unbound *CounterHandle so the bind.go
// trampoline exercises the lazy-bind path through the
// unregisteredHandleState state machine on the way out.
type handleTestHost struct{}

func (handleTestHost) NewCounter(ctx context.Context, initial uint32) (*counterhost.CounterHandle, error) {
	return counterhost.NewCounterHandle(&handleTestCounter{value: initial}), nil
}
