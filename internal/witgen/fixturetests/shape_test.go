package fixturetests_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/shape"
)

type myShape struct{}

func (myShape) Describe(ctx context.Context, s shape.ShapeKind) (string, error) {
	switch x := s.(type) {
	case shape.ShapeKindCircle:
		return fmt.Sprintf("circle r=%d", x.Value), nil
	case shape.ShapeKindRect:
		return fmt.Sprintf("rect at (%d,%d)", x.Value.X, x.Value.Y), nil
	case shape.ShapeKindNone:
		return "none", nil
	}
	return "unknown", nil
}

func TestShape_VariantRoundTrip(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := shape.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	hostInst, err := fac.NewInstance(ctx, myShape{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer hostInst.Close(ctx)

	importerWAT := `(component
  (import "h" (instance $h
    (type $pt (record (field "x" u32) (field "y" u32)))
    (export "point" (type (eq $pt)))
    (type $sk (variant
      (case "circle" u32)
      (case "rect" $pt)
      (case "none")
    ))
    (export "shape-kind" (type (eq $sk)))
    (export "describe" (func (param "s" $sk) (result string)))
  ))
  (alias export $h "describe" (func $d))
  (export "describe" (func $d))
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

	fn := inst.ExportedFunc("describe")

	// circle(5)
	got, err := fn.Call(ctx, wacogo.NewValVariant(0, wacogo.ValU32(5)))
	if err != nil {
		t.Fatalf("Call(circle): %v", err)
	}
	if s := string(got[0].(wacogo.ValString)); s != "circle r=5" {
		t.Errorf("Describe(circle 5): got %q, want %q", s, "circle r=5")
	}

	// rect((10,20))
	pt := wacogo.NewValRecord(
		wacogo.Field{Name: "x", Val: wacogo.ValU32(10)},
		wacogo.Field{Name: "y", Val: wacogo.ValU32(20)},
	)
	got, err = fn.Call(ctx, wacogo.NewValVariant(1, pt))
	if err != nil {
		t.Fatalf("Call(rect): %v", err)
	}
	if s := string(got[0].(wacogo.ValString)); s != "rect at (10,20)" {
		t.Errorf("Describe(rect): got %q, want %q", s, "rect at (10,20)")
	}

	// none
	got, err = fn.Call(ctx, wacogo.NewValVariant(2, nil))
	if err != nil {
		t.Fatalf("Call(none): %v", err)
	}
	if s := string(got[0].(wacogo.ValString)); s != "none" {
		t.Errorf("Describe(none): got %q, want %q", s, "none")
	}
}
