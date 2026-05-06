package fixturetests_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/sumlist"
)

type mySumlist struct{}

func (mySumlist) Sum(ctx context.Context, xs []uint32) (uint32, error) {
	var total uint32
	for _, x := range xs {
		total += x
	}
	return total, nil
}

func TestSumlist_ListU32RoundTrip(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := sumlist.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	hostInst, err := fac.NewInstance(ctx, mySumlist{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer hostInst.Close(ctx)

	importerWAT := `(component
  (type $ft (func (param "xs" (list u32)) (result u32)))
  (import "h" (instance $h
    (export "sum" (func (type $ft)))
  ))
  (alias export $h "sum" (func $s))
  (export "sum" (func $s))
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

	fn := inst.ExportedFunc("sum")
	vals := wacogo.NewValListOf[wacogo.ValU32](1, 2, 3, 4, 5)
	got, err := fn.Call(ctx, vals)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if u := uint32(got[0].(wacogo.ValU32)); u != 15 {
		t.Fatalf("got %d, want 15", u)
	}
}
