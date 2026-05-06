package fixturetests_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/compass"
)

type myCompass struct{}

func (myCompass) Reverse(ctx context.Context, d compass.Direction) (compass.Direction, error) {
	switch d {
	case compass.DirectionNorth:
		return compass.DirectionSouth, nil
	case compass.DirectionSouth:
		return compass.DirectionNorth, nil
	case compass.DirectionEast:
		return compass.DirectionWest, nil
	case compass.DirectionWest:
		return compass.DirectionEast, nil
	}
	return d, nil
}

func TestCompass_EnumRoundTrip(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := compass.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	hostInst, err := fac.NewInstance(ctx, myCompass{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer hostInst.Close(ctx)

	importerWAT := `(component
  (import "h" (instance $h
    (type $dir (enum "north" "south" "east" "west"))
    (export "direction" (type (eq $dir)))
    (export "reverse" (func (param "d" $dir) (result $dir)))
  ))
  (alias export $h "reverse" (func $r))
  (export "reverse" (func $r))
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

	fn := inst.ExportedFunc("reverse")

	cases := []struct {
		in, want uint32 // discriminant values
	}{
		{0, 1}, // north → south
		{1, 0}, // south → north
		{2, 3}, // east → west
		{3, 2}, // west → east
	}
	for _, c := range cases {
		got, err := fn.Call(ctx, wacogo.NewValEnum(c.in))
		if err != nil {
			t.Fatalf("Call(%d): %v", c.in, err)
		}
		e := got[0].(*wacogo.ValEnum)
		if e.Discriminant() != c.want {
			t.Errorf("Reverse(%d) = %d, want %d", c.in, e.Discriminant(), c.want)
		}
	}
}
