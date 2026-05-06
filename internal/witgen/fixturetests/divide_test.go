package fixturetests_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/divide"
)

type myDivide struct{}

func (myDivide) Divide(ctx context.Context, a, b uint32) (divide.ResultU32String, error) {
	if b == 0 {
		return divide.ResultU32StringErr{Value: "division by zero"}, nil
	}
	return divide.ResultU32StringOk{Value: a / b}, nil
}

func TestDivide_ResultRoundTrip(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := divide.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	hostInst, err := fac.NewInstance(ctx, myDivide{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer hostInst.Close(ctx)

	importerWAT := `(component
  (type $ft (func (param "a" u32) (param "b" u32) (result (result u32 (error string)))))
  (import "h" (instance $h
    (export "divide" (func (type $ft)))
  ))
  (alias export $h "divide" (func $d))
  (export "divide" (func $d))
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

	fn := inst.ExportedFunc("divide")

	// Divide(20, 5) -> Ok(4)
	got, err := fn.Call(ctx, wacogo.ValU32(20), wacogo.ValU32(5))
	if err != nil {
		t.Fatalf("Call(20,5): %v", err)
	}
	r := got[0].(*wacogo.ValResult)
	if !r.IsOk() {
		t.Fatalf("Divide(20,5): expected Ok, got Err")
	}
	if u := uint32(r.Ok().(wacogo.ValU32)); u != 4 {
		t.Errorf("Divide(20,5): got %d, want 4", u)
	}

	// Divide(20, 0) -> Err("division by zero")
	got, err = fn.Call(ctx, wacogo.ValU32(20), wacogo.ValU32(0))
	if err != nil {
		t.Fatalf("Call(20,0): %v", err)
	}
	r = got[0].(*wacogo.ValResult)
	if r.IsOk() {
		t.Fatalf("Divide(20,0): expected Err, got Ok")
	}
	if s := string(r.Err().(wacogo.ValString)); s != "division by zero" {
		t.Errorf("Divide(20,0).Err: got %q, want %q", s, "division by zero")
	}
}
