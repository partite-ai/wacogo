package fixturetests_test

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/counterhost"
)

// TestBorrow_DropBeforeReturn exercises the borrow lifecycle end-to-end
// through the regenerated counterhost fixture: the wasm-side wrapper
// calls counter.current with self as borrow<counter>; counter is
// locally-defined on the callee instance, so the canon-ABI same-
// component shortcut applies — the rep flows through without an
// IssueBorrow/dropBorrow round-trip on the callee side.
//
// The post-call invariant we verify: the call returns successfully
// (no "outstanding borrows" error from the run-side defers) and the
// observable counter state is correct.
//
// This is the happy-path companion to the canon-ResourceTable unit
// tests (TestRunTransferPlan_TrapsOnUndroppedBorrow,
// TestRunGocallPlan_ErrorsOnUndroppedBorrow), which verify the
// trap-on-undropped behavior at the table level.
func TestBorrow_DropBeforeReturn(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := counterhost.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	inst, err := fac.NewInstance(ctx, &borrowTestHost{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer inst.Close(ctx)

	wrapped := counterhost.WrapInstance(inst, inst.Core())

	// constructor → own<counter>
	c, err := wrapped.NewCounter(ctx, 10)
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}

	// method call: borrow<counter> param + u32 result. Same-component
	// shortcut: rep flows through; NumBorrows stays 0 throughout.
	got, err := c.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if got != 10 {
		t.Errorf("Current() = %d, want 10", got)
	}

	// Mutating method call exercises the same borrow path with no
	// result.
	if err := c.Increment(ctx); err != nil {
		t.Fatalf("Increment: %v", err)
	}
	if err := c.Increment(ctx); err != nil {
		t.Fatalf("Increment: %v", err)
	}

	got, err = c.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if got != 12 {
		t.Errorf("Current() after 2x Increment = %d, want 12", got)
	}

	// Releasing the own handle goes through the canon Drop orchestrator.
	if err := c.Drop(ctx); err != nil {
		t.Errorf("Drop() on own handle: unexpected error %v", err)
	}
}

// borrowTestHost is a minimal Counterhost impl for the borrow tests.
// Returns a fresh *CounterHandle wrapping a borrowTestCounter on each
// NewCounter call.
type borrowTestHost struct{}

func (borrowTestHost) NewCounter(ctx context.Context, initial uint32) (*counterhost.CounterHandle, error) {
	return counterhost.NewCounterHandle(&borrowTestCounter{value: initial}), nil
}

type borrowTestCounter struct{ value uint32 }

func (c *borrowTestCounter) Current(ctx context.Context) (uint32, error) { return c.value, nil }
func (c *borrowTestCounter) Increment(ctx context.Context) error         { c.value++; return nil }
