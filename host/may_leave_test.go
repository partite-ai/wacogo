package host_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
	"github.com/partite-ai/wacogo/wasmtools"
)

// TestMayLeave_HostImport checks a host function can't be reached from a realloc,
// while the component is being lowered into, but can be otherwise.
func TestMayLeave_HostImport(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	calls := 0
	hb := e.NewHostBuilder("provider-host")
	hb.AddFunction("f", &host.FuncType{}, func(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error {
		calls++
		return nil
	})
	hComp, err := hb.Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer hComp.Close(ctx)
	hInst, err := hComp.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer hInst.Close(ctx)

	tool, err := wasmtools.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bin, err := tool.Parse(ctx, []byte(`(component
  (import "f" (func $f))
  (core func $lowered (canon lower (func $f)))
  (core instance $env (export "f" (func $lowered)))
  (core module $m
    (import "env" "f" (func $f))
    (memory (export "m") 1)
    (func (export "r") (param i32 i32 i32 i32) (result i32) (call $f) i32.const 1024)
    (func (export "outside") (call $f))
    (func (export "lowered-into") (param i32 i32)))
  (core instance $i (instantiate $m (with "env" (instance $env))))
  (func (export "outside") (canon lift (core func $i "outside")))
  (func (export "lowered-into") (param "s" string)
    (canon lift (core func $i "lowered-into") (memory $i "m") (realloc (func $i "r")))))`))
	if err != nil {
		t.Fatal(err)
	}
	comp, err := e.LoadComponent(ctx, bytes.NewReader(bin))
	if err != nil {
		t.Fatal(err)
	}
	inst, err := comp.Instantiate(ctx, wacogo.WithFuncImport("f", hInst.Core().ExportedFunc("f")))
	if err != nil {
		t.Fatal(err)
	}
	defer inst.Close(ctx)

	if _, err := inst.ExportedFunc("outside").Call(ctx); err != nil {
		t.Fatalf("outside: %v", err)
	}
	if calls != 1 {
		t.Fatalf("outside: host function called %d times, want 1", calls)
	}

	_, err = inst.ExportedFunc("lowered-into").Call(ctx, core.ValString("hello"))
	if err == nil || !strings.Contains(err.Error(), "cannot leave component instance") {
		t.Fatalf("lowered-into: got %v", err)
	}
	if calls != 1 {
		t.Fatalf("lowered-into: host function was reached")
	}
}
