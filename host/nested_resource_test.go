package host_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
)

// A resource declared in a nested instance scope must be resolvable
// through that instance, while the root remains its defining instance:
// a nested scope is a namespace, not a runtime of its own.
func TestNestedScopeResourceResolvesAndRootDefines(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("res-host")
	b.AddResource("root-thing", nil)
	nested := b.AddNestedInstance("nested")
	nestedRT := nested.AddResource("thing", nil)
	nested.AddFunction("make", &host.FuncType{
		Results: []host.ResultDecl{{Type: nestedRT.Own()}},
	}, func(ctx context.Context, cc *host.CallContext, h *host.ComponentInstance, stack []uint64) error {
		return nil
	})

	hostComp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer hostComp.Close(ctx)
	hostInst, err := hostComp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer hostInst.Close(ctx)

	sub := hostInst.Core().ExportedInstance("nested")
	if sub == nil {
		t.Fatal("nested instance is not exported")
	}
	nestedTR, ok := sub.ExportedType("thing").(*core.TypeResource)
	if !ok {
		t.Fatalf("nested ExportedType(thing) = %T, want *core.TypeResource", sub.ExportedType("thing"))
	}
	rootTR, ok := hostInst.Core().ExportedType("root-thing").(*core.TypeResource)
	if !ok {
		t.Fatalf("root ExportedType(root-thing) = %T, want *core.TypeResource", hostInst.Core().ExportedType("root-thing"))
	}

	// Distinct type identity per declaration...
	if nestedTR == rootTR {
		t.Error("nested and root resources share a *TypeResource; identities must be distinct")
	}
	// ...but one runtime home, the root.
	if got := nestedTR.Instance(); got != hostInst.Core() {
		t.Errorf("nested resource defining instance = %p, want root %p", got, hostInst.Core())
	}
	if got := rootTR.Instance(); got != hostInst.Core() {
		t.Errorf("root resource defining instance = %p, want root %p", got, hostInst.Core())
	}
	if nestedTR.Instance() == sub {
		t.Error("nested shell must not be the defining instance; it has no table, dtor state, or reentrance gate")
	}

	// A guest aliasing the nested resource type must instantiate.
	guestWAT := `(component
  (import "host" (instance $h
    (export "nested" (instance
      (export "thing" (type (sub resource)))
    ))
  ))
  (alias export $h "nested" (instance $n))
  (alias export $n "thing" (type $t))
  (export "re-thing" (type $t))
)`
	guestComp, err := e.LoadComponent(ctx, bytes.NewReader(watToBinary(t, guestWAT)))
	if err != nil {
		t.Fatalf("guest LoadComponent: %v", err)
	}
	guestInst, err := guestComp.Instantiate(ctx,
		wacogo.WithInstanceImport("host", hostInst.Core()))
	if err != nil {
		t.Fatalf("guest Instantiate: %v", err)
	}
	defer guestInst.Close(ctx)
}
