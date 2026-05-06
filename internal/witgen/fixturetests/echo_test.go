package fixturetests_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/echo"
)

type myEcho struct{}

func (myEcho) Greet(ctx context.Context, name string) (string, error) { return "hello, " + name, nil }

func TestEcho_StringRoundTrip(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	fac, err := echo.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	hostInst, err := fac.NewInstance(ctx, myEcho{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer hostInst.Close(ctx)

	importerWAT := `(component
  (type $ft (func (param "name" string) (result string)))
  (import "h" (instance $h
    (export "greet" (func (type $ft)))
  ))
  (alias export $h "greet" (func $g))
  (export "greet" (func $g))
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

	fn := inst.ExportedFunc("greet")
	got, err := fn.Call(ctx, wacogo.ValString("world"))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if s := string(got[0].(wacogo.ValString)); s != "hello, world" {
		t.Fatalf("got %q, want %q", s, "hello, world")
	}
}
