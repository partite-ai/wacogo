package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/examples/calculator/gen/example/demo/calc"
)

func TestCalculator_Add(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

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

	importerWAT := `(component
  (type $ft (func (param "a" u32) (param "b" u32) (result u32)))
  (import "h" (instance $h
    (export "add" (func (type $ft)))
  ))
  (alias export $h "add" (func $a))
  (export "add" (func $a))
)`
	bin := watToBinary(ctx, importerWAT)
	comp, err := e.LoadComponent(ctx, bytes.NewReader(bin))
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}
	inst, err := comp.Instantiate(ctx, wacogo.WithInstanceImport("h", hostInst.Core()))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	fn := inst.ExportedFunc("add")
	got, err := fn.Call(ctx, wacogo.ValU32(7), wacogo.ValU32(35))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if u := uint32(got[0].(wacogo.ValU32)); u != 42 {
		t.Fatalf("got %d, want 42", u)
	}
}
