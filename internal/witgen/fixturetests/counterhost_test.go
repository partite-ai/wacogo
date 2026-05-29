package fixturetests_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/counterhost"
)

// counterhostCounter implements the generated Counter resource interface, plus
// an optional Drop() that the host calls on terminal drop.
type counterhostCounter struct{ v uint32 }

func (c *counterhostCounter) Increment(ctx context.Context) error         { c.v++; return nil }
func (c *counterhostCounter) Current(ctx context.Context) (uint32, error) { return c.v, nil }

// Module-level state captured by the optional Drop() — observed in the
// test to confirm the dtor fires exactly once with the right object.
var (
	dropCount   int
	lastDropped *counterhostCounter
)

func (c *counterhostCounter) Drop(_ context.Context) error {
	dropCount++
	lastDropped = c
	return nil
}

// myCounterhost implements the generated Counterhost interface, minting
// a fresh *counterhostCounter wrapped in *CounterHandle for each NewCounter call.
type myCounterhost struct{}

func (myCounterhost) NewCounter(ctx context.Context, initial uint32) (*counterhost.CounterHandle, error) {
	return counterhost.NewCounterHandle(&counterhostCounter{v: initial}), nil
}

// TestCounter_ResourceLifecycle exercises the full resource pipeline
// end-to-end through wasm: construct, increment×2, current, drop.
// Asserts the method math is right and the optional Drop() fires once.
func TestCounter_ResourceLifecycle(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := counterhost.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	hostInst, err := fac.NewInstance(ctx, myCounterhost{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer hostInst.Close(ctx)

	// Importer: imports the counter resource + ctor + methods, lowers
	// them into a core module that calls construct → inc → inc → cur →
	// drop and returns the final value.
	importerWAT := `(component
  (import "h" (instance $h
    (export "counter" (type $c (sub resource)))
    (export "[constructor]counter" (func (param "initial" u32) (result (own $c))))
    (export "[method]counter.increment" (func (param "self" (borrow $c))))
    (export "[method]counter.current" (func (param "self" (borrow $c)) (result u32)))
  ))
  (alias export $h "counter" (type $c))
  (alias export $h "[constructor]counter" (func $new))
  (alias export $h "[method]counter.increment" (func $inc))
  (alias export $h "[method]counter.current" (func $cur))

  (core func $new-lowered (canon lower (func $new)))
  (core func $inc-lowered (canon lower (func $inc)))
  (core func $cur-lowered (canon lower (func $cur)))
  (core func $drop (canon resource.drop $c))

  (core module $m
    (import "" "new"  (func $new (param i32) (result i32)))
    (import "" "inc"  (func $inc (param i32)))
    (import "" "cur"  (func $cur (param i32) (result i32)))
    (import "" "drop" (func $drop (param i32)))

    (func (export "test") (result i32)
      (local $h i32)
      (local $v i32)
      i32.const 5
      call $new
      local.set $h
      local.get $h
      call $inc
      local.get $h
      call $inc
      local.get $h
      call $cur
      local.set $v
      local.get $h
      call $drop
      local.get $v
    )
    (memory (export "memory") 1)
    (func (export "realloc") (param i32 i32 i32 i32) (result i32) i32.const 0)
  )
  (core instance $bridge
    (export "new" (func $new-lowered))
    (export "inc" (func $inc-lowered))
    (export "cur" (func $cur-lowered))
    (export "drop" (func $drop)))
  (core instance $i (instantiate $m (with "" (instance $bridge))))
  (alias core export $i "test" (core func $t))
  (alias core export $i "memory" (core memory $mem))
  (alias core export $i "realloc" (core func $r))
  (func (export "test") (result u32) (canon lift (core func $t) (memory $mem) (realloc $r)))
)`

	bin := watToBinary(t, importerWAT)
	comp, err := e.LoadComponent(ctx, bytes.NewReader(bin))
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}
	inst, err := comp.Instantiate(ctx, wacogo.WithInstanceImport("h", hostInst.Core()))
	if err != nil {
		t.Fatalf("Instantiate importer: %v", err)
	}
	defer inst.Close(ctx)

	dropCount = 0
	lastDropped = nil

	fn := inst.ExportedFunc("test")
	if fn == nil {
		t.Fatal("want exported test")
	}
	got, err := fn.Call(ctx)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if u := uint32(got[0].(wacogo.ValU32)); u != 7 {
		t.Fatalf("test result: got %d, want 7 (5 + 2 increments)", u)
	}
	if dropCount != 1 {
		t.Errorf("Drop count: got %d, want 1", dropCount)
	}
	if lastDropped == nil || lastDropped.v != 7 {
		t.Errorf("dropped counter: got %+v, want &counterhostCounter{v:7}", lastDropped)
	}
}
