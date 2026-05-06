package fixturetests_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/midpoint"
)

type myMidpoint struct{}

func (myMidpoint) Avg(ctx context.Context, a, b midpoint.Point) (midpoint.Point, error) {
	return midpoint.Point{
		X: (a.X + b.X) / 2,
		Y: (a.Y + b.Y) / 2,
	}, nil
}

func TestMidpoint_RecordRoundTrip(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := midpoint.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	hostInst, err := fac.NewInstance(ctx, myMidpoint{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer hostInst.Close(ctx)

	importerWAT := `(component
  (import "h" (instance $h
    (type $pt (record (field "x" u32) (field "y" u32)))
    (export "point" (type (eq $pt)))
    (export "avg" (func (param "a" $pt) (param "b" $pt) (result $pt)))
  ))
  (alias export $h "avg" (func $a))
  (export "avg" (func $a))
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

	fn := inst.ExportedFunc("avg")

	mkpt := func(x, y uint32) *wacogo.ValRecord {
		return wacogo.NewValRecord(
			wacogo.Field{Name: "x", Val: wacogo.ValU32(x)},
			wacogo.Field{Name: "y", Val: wacogo.ValU32(y)},
		)
	}

	got, err := fn.Call(ctx, mkpt(2, 4), mkpt(10, 20))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	rec := got[0].(*wacogo.ValRecord)
	if x := uint32(rec.Field("x").(wacogo.ValU32)); x != 6 {
		t.Errorf("x: got %d, want 6", x)
	}
	if y := uint32(rec.Field("y").(wacogo.ValU32)); y != 12 {
		t.Errorf("y: got %d, want 12", y)
	}
}
