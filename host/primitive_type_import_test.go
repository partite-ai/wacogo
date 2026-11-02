package host_test

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
)

// TestPrimitiveTypeExportResolvesFromImportDeclaration verifies that a guest
// can alias a type export that its import declaration fixes to a primitive,
// such as wasi:clocks' `type duration = u64`, from a host instance. Host
// components cannot export primitive types, so the type must come from the
// guest's declaration. The guest aliases an instance before the import, so
// the import's instance index is past an aliased instance.
func TestPrimitiveTypeExportResolvesFromImportDeclaration(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	ob := e.NewHostBuilder("other-host")
	ob.AddNestedInstance("nested").AddType("code", host.Record{Fields: []host.Field{
		{Name: "code", Type: host.U32},
	}})
	otherComp, err := ob.Build(ctx)
	if err != nil {
		t.Fatalf("other Build: %v", err)
	}
	defer otherComp.Close(ctx)
	other, err := otherComp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("other Instantiate: %v", err)
	}
	defer other.Close(ctx)

	cb := e.NewHostBuilder("clock-host")
	cb.AddFunction("now", &host.FuncType{
		Results: []host.ResultDecl{{Name: "", Type: host.U64}},
	}, func(_ context.Context, _ *core.CallContext, _ *host.ComponentInstance, stack []uint64) error {
		stack[0] = 42
		return nil
	})
	clockComp, err := cb.Build(ctx)
	if err != nil {
		t.Fatalf("clock Build: %v", err)
	}
	defer clockComp.Close(ctx)
	clock, err := clockComp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("clock Instantiate: %v", err)
	}
	defer clock.Close(ctx)

	guestWAT := `(component
  (import "other" (instance $o
    (export "nested" (instance
      (type $code (record (field "code" u32)))
      (export "code" (type (eq $code)))
    ))
  ))
  (alias export $o "nested" (instance $n))
  (import "clock" (instance $c
    (type $d u64)
    (export "duration" (type (eq $d)))
    (export "now" (func (result u64)))
  ))
  (alias export $c "duration" (type $dur))
  (export "reexported-duration" (type $dur))
)`
	guestComp, err := e.LoadComponent(ctx, bytes.NewReader(watToBinary(t, guestWAT)))
	if err != nil {
		t.Fatalf("guest LoadComponent: %v", err)
	}
	guest, err := guestComp.Instantiate(ctx,
		wacogo.WithInstanceImport("other", other.Core()),
		wacogo.WithInstanceImport("clock", clock.Core()))
	if err != nil {
		t.Fatalf("guest Instantiate: %v", err)
	}
	defer guest.Close(ctx)

	if got := guest.ExportedType("reexported-duration"); !reflect.DeepEqual(got, core.TypeU64{}) {
		t.Errorf("ExportedType(reexported-duration) = %#v, want core.TypeU64{}", got)
	}
}
