package fixturetests_test

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/counterhost"
)

// TestCounterhost_WrapInstanceRoundTrip verifies that WrapInstance returns a
// Counterhost whose methods actually call through the canon ABI into the
// host-component's wasm stub and back. It exercises the full
// Go→wasm (wrapper) direction: constructor, method calls, and the
// returned *CounterHandle's remote-dispatch wrapper.
//
// After the handle-management refactor, WrapInstance takes (caller,
// callee). For this round-trip exercise the same instance plays both
// roles — the test exercises the same-component code path.
func TestCounterhost_WrapInstanceRoundTrip(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := counterhost.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	inst, err := fac.NewInstance(ctx, &wrapTestImpl{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer inst.Close(ctx)

	// Same-instance caller==callee for the simple round-trip test.
	wrapped := counterhost.WrapInstance(inst, inst.Core())

	// NewCounter → *CounterHandle bound to the callee's canon table;
	// each method call goes through CallRaw into the stub module and
	// back to the Go trampoline.
	c, err := wrapped.NewCounter(ctx, 7)
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}

	if err := c.Increment(ctx); err != nil {
		t.Fatalf("Increment: %v", err)
	}
	if err := c.Increment(ctx); err != nil {
		t.Fatalf("Increment: %v", err)
	}

	got, err := c.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if got != 9 {
		t.Errorf("Current() = %d, want 9 (7 + 2 increments)", got)
	}
}

// wrapTestImpl is the host implementation used in the wrap round-trip test.
// It is a separate type from myCounterhost in the runtime test to avoid
// test-file conflicts (both files are in the same test package).
type wrapTestImpl struct{}

func (wrapTestImpl) NewCounter(ctx context.Context, initial uint32) (*counterhost.CounterHandle, error) {
	return counterhost.NewCounterHandle(&wrapCounter{value: initial}), nil
}

type wrapCounter struct{ value uint32 }

func (c *wrapCounter) Current(ctx context.Context) (uint32, error) { return c.value, nil }
func (c *wrapCounter) Increment(ctx context.Context) error         { c.value++; return nil }
