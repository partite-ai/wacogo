package fixturetests_test

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/compounds"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/counterhost"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/echo"
)

// TestCounterhost_WrapInstance_ForeignCaller exercises WrapInstance with
// a caller built by a different witgen package than the wrapper's own.
//
// The wrap.go method bodies once threaded TR identity through a per-
// caller-package slot table — broken when the caller came from another
// package (here: echo, which declares zero resources) because the
// constant index didn't refer to anything meaningful on the caller.
// With the type now threaded into each helper, the TR for each resource
// leaf is recovered from the wrapped function's signature instead, so
// this test must succeed for any caller.
func TestCounterhost_WrapInstance_ForeignCaller(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	// Callee: a real counterhost instance — its canon table holds
	// Counter entries keyed by *its* TR for "counter".
	calleeFac, err := counterhost.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("counterhost NewFactory: %v", err)
	}
	defer calleeFac.Close(ctx)
	callee, err := calleeFac.NewInstance(ctx, wrapForeignCounterhost{}, nil)
	if err != nil {
		t.Fatalf("counterhost NewInstance: %v", err)
	}
	defer callee.Close(ctx)

	// Caller: from a foreign witgen package (echo). echo declares no
	// resources of its own; the wrap.go emitted by counterhost must
	// resolve TR identity through the wrapped func's signature so the
	// caller's package-internal layout is irrelevant.
	echoFac, err := echo.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("echo NewFactory: %v", err)
	}
	defer echoFac.Close(ctx)
	caller, err := echoFac.NewInstance(ctx, wrapForeignEcho{}, nil)
	if err != nil {
		t.Fatalf("echo NewInstance: %v", err)
	}
	defer caller.Close(ctx)

	wrapped := counterhost.WrapInstance(caller, callee.Core())

	c, err := wrapped.NewCounter(ctx, 5)
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}
	got, err := c.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if got != 5 {
		t.Errorf("Current() = %d, want 5", got)
	}
}

type wrapForeignCounterhost struct{}

func (wrapForeignCounterhost) NewCounter(_ context.Context, initial uint32) (*counterhost.CounterHandle, error) {
	return counterhost.NewCounterHandle(&wrapForeignCounter{value: initial}), nil
}

type wrapForeignCounter struct{ value uint32 }

func (c *wrapForeignCounter) Current(_ context.Context) (uint32, error) { return c.value, nil }
func (c *wrapForeignCounter) Increment(_ context.Context) error         { c.value++; return nil }

type wrapForeignEcho struct{}

func (wrapForeignEcho) Greet(_ context.Context, name string) (string, error) {
	return "hello, " + name, nil
}

// TestCompounds_WrapInstance_ForeignCaller exercises the same foreign-caller
// scenario across compound result/param types: list<own<R>> (NewCounters)
// and record { c: own<R> } (BoxCounter). These drive the lift/lower
// helpers in compounds.bind.go. With the type threaded into each
// helper, the TR for each resource leaf is recovered from the wrapped
// function's signature, so foreign callers work the same as same-package
// callers.
func TestCompounds_WrapInstance_ForeignCaller(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	calleeFac, err := compounds.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("compounds NewFactory: %v", err)
	}
	defer calleeFac.Close(ctx)
	callee, err := calleeFac.NewInstance(ctx, wrapForeignCompounds{}, nil)
	if err != nil {
		t.Fatalf("compounds NewInstance: %v", err)
	}
	defer callee.Close(ctx)

	echoFac, err := echo.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("echo NewFactory: %v", err)
	}
	defer echoFac.Close(ctx)
	caller, err := echoFac.NewInstance(ctx, wrapForeignEcho{}, nil)
	if err != nil {
		t.Fatalf("echo NewInstance: %v", err)
	}
	defer caller.Close(ctx)

	wrapped := compounds.WrapInstance(caller, callee.Core())

	// list<own<counter>> result path.
	cs, err := wrapped.NewCounters(ctx, []uint32{10, 20, 30})
	if err != nil {
		t.Fatalf("NewCounters: %v", err)
	}
	if len(cs) != 3 {
		t.Fatalf("NewCounters returned %d handles, want 3", len(cs))
	}
	for i, want := range []uint32{10, 20, 30} {
		got, err := cs[i].Get(ctx)
		if err != nil {
			t.Fatalf("cs[%d].Get: %v", i, err)
		}
		if got != want {
			t.Errorf("cs[%d].Get() = %d, want %d", i, got, want)
		}
	}

	// record { c: own<counter> } result path: BoxCounter takes a fresh
	// counter handle as own<R> param and returns it inside a record.
	c, err := wrapped.NewCounter(ctx, 42)
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}
	rec, err := wrapped.BoxCounter(ctx, c)
	if err != nil {
		t.Fatalf("BoxCounter: %v", err)
	}
	got, err := rec.C.Get(ctx)
	if err != nil {
		t.Fatalf("rec.C.Get: %v", err)
	}
	if got != 42 {
		t.Errorf("rec.C.Get() = %d, want 42", got)
	}
}

type wrapForeignCompounds struct{}

func (wrapForeignCompounds) NewCounters(_ context.Context, starts []uint32) ([]*compounds.CounterHandle, error) {
	out := make([]*compounds.CounterHandle, len(starts))
	for i, s := range starts {
		out[i] = compounds.NewCounterHandle(&wrapForeignCompoundsCounter{value: s})
	}
	return out, nil
}

func (wrapForeignCompounds) BoxCounter(_ context.Context, c *compounds.CounterHandle) (compounds.RecordWithCounter, error) {
	return compounds.RecordWithCounter{C: c}, nil
}

func (wrapForeignCompounds) NewCounter(_ context.Context, start uint32) (*compounds.CounterHandle, error) {
	return compounds.NewCounterHandle(&wrapForeignCompoundsCounter{value: start}), nil
}

type wrapForeignCompoundsCounter struct{ value uint32 }

func (c *wrapForeignCompoundsCounter) Get(_ context.Context) (uint32, error) { return c.value, nil }
