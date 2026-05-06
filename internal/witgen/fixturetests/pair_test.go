package fixturetests_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/pair"
)

type myPair struct{}

func (myPair) Swap(ctx context.Context, p pair.TupleU32U32) (pair.TupleU32U32, error) {
	return pair.TupleU32U32{F0: p.F1, F1: p.F0}, nil
}

func TestPair_TupleSwap(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := pair.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	hostInst, err := fac.NewInstance(ctx, myPair{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer hostInst.Close(ctx)

	importerWAT := `(component
  (type $ft (func (param "p" (tuple u32 u32)) (result (tuple u32 u32))))
  (import "h" (instance $h
    (export "swap" (func (type $ft)))
  ))
  (alias export $h "swap" (func $s))
  (export "swap" (func $s))
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

	fn := inst.ExportedFunc("swap")
	// Tuples are represented as *ValRecord with positional field names "0", "1".
	arg := wacogo.NewValRecord(
		wacogo.Field{Name: "0", Val: wacogo.ValU32(7)},
		wacogo.Field{Name: "1", Val: wacogo.ValU32(35)},
	)
	got, err := fn.Call(ctx, arg)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	rec := got[0].(*wacogo.ValRecord)
	f0 := uint32(rec.Field("0").(wacogo.ValU32))
	f1 := uint32(rec.Field("1").(wacogo.ValU32))
	if f0 != 35 || f1 != 7 {
		t.Fatalf("got (%d, %d), want (35, 7)", f0, f1)
	}
}
