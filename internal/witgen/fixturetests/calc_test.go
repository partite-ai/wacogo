package fixturetests_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/calc"
)

// myCalc implements the generated Calc interface.
type myCalc struct{}

func (myCalc) Add(ctx context.Context, a uint32, b uint32) (uint32, error) { return a + b, nil }

func TestCalc_RoundTripThroughWasm(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	// Build the host-side calc instance via the generated factory.
	fac, err := calc.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	hostInst, err := fac.NewInstance(ctx, myCalc{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer hostInst.Close(ctx)

	// Importer component: imports calc, re-exports add.
	importerWAT := `(component
  (type $ft (func (param "a" u32) (param "b" u32) (result u32)))
  (import "h" (instance $h
    (export "add" (func (type $ft)))
  ))
  (alias export $h "add" (func $add))
  (export "add" (func $add))
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

	fn := inst.ExportedFunc("add")
	if fn == nil {
		t.Fatal("want exported add")
	}
	got, err := fn.Call(ctx, wacogo.ValU32(7), wacogo.ValU32(35))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if u := uint32(got[0].(wacogo.ValU32)); u != 42 {
		t.Fatalf("want 42, got %d", u)
	}
}
