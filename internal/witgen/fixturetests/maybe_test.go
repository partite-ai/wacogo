package fixturetests_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/maybe"
)

type myMaybe struct{}

func (myMaybe) Find(ctx context.Context, key uint32) (maybe.OptionString, error) {
	if key == 42 {
		return maybe.SomeString("answer"), nil
	}
	return maybe.NoneString(), nil
}

func TestMaybe_OptionStringRoundTrip(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := maybe.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	hostInst, err := fac.NewInstance(ctx, myMaybe{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer hostInst.Close(ctx)

	importerWAT := `(component
  (type $ft (func (param "key" u32) (result (option string))))
  (import "h" (instance $h
    (export "find" (func (type $ft)))
  ))
  (alias export $h "find" (func $f))
  (export "find" (func $f))
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

	fn := inst.ExportedFunc("find")

	// Find(42) -> Some("answer")
	got, err := fn.Call(ctx, wacogo.ValU32(42))
	if err != nil {
		t.Fatalf("Call(42): %v", err)
	}
	opt := got[0].(*wacogo.ValOption)
	if opt.IsNone() {
		t.Fatal("Find(42): expected Some, got None")
	}
	if s := string(opt.Val().(wacogo.ValString)); s != "answer" {
		t.Errorf("Find(42).Val: got %q, want %q", s, "answer")
	}

	// Find(0) -> None
	got, err = fn.Call(ctx, wacogo.ValU32(0))
	if err != nil {
		t.Fatalf("Call(0): %v", err)
	}
	opt = got[0].(*wacogo.ValOption)
	if !opt.IsNone() {
		t.Errorf("Find(0): expected None, got %v", opt)
	}
}
