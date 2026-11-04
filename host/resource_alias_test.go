package host_test

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
)

func TestResourceAlias_SharesIdentity(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("aliastest")
	primary := b.AddResource("primary", nil)
	if err := b.AddResourceAlias("alias", primary); err != nil {
		t.Fatalf("AddResourceAlias: %v", err)
	}

	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)

	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	tPrimary, ok := inst.Core().ExportedType("primary").(*core.TypeResource)
	if !ok {
		t.Fatalf("primary export: got %T", inst.Core().ExportedType("primary"))
	}
	tAlias, ok := inst.Core().ExportedType("alias").(*core.TypeResource)
	if !ok {
		t.Fatalf("alias export: got %T", inst.Core().ExportedType("alias"))
	}
	if tPrimary != tAlias {
		t.Fatalf("alias identity mismatch: primary=%p alias=%p", tPrimary, tAlias)
	}

	// The checker must see one resource under both names.
	consumer := loadGuest(t, e, `(component
  (import "a" (instance $a (export "primary" (type (sub resource)))))
  (alias export $a "primary" (type $r))
  (import "b" (instance (export "alias" (type (eq $r))))))`)
	if _, err := consumer.Instantiate(ctx,
		wacogo.WithInstanceImport("a", inst.Core()),
		wacogo.WithInstanceImport("b", inst.Core())); err != nil {
		t.Fatalf("want alias and target to be one resource, got: %v", err)
	}
}

func TestResourceAlias_TargetFromOtherBuilder_Errors(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	otherB := e.NewHostBuilder("other")
	other := otherB.AddResource("other-r", nil)

	b := e.NewHostBuilder("aliastest_err")
	if err := b.AddResourceAlias("alias", other); err == nil {
		t.Fatal("AddResourceAlias accepting a foreign *ResourceType: want error, got nil")
	}
}

func TestResourceAlias_DuplicateName_RejectedAtBuild(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("aliastest_dup")
	r := b.AddResource("r", nil)
	if err := b.AddResourceAlias("r", r); err != nil {
		t.Fatalf("AddResourceAlias returned an immediate error; expected the duplicate-name check at Build: %v", err)
	}
	if _, err := b.Build(ctx); err == nil {
		t.Fatal("Build with alias colliding on the original name: want error, got nil")
	}
}

// An alias can target a resource in any scope, whatever the order they are
// declared and visited in.
func TestResourceAlias_AnyScope(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("alias-any-scope")
	m := b.AddNestedInstance("m")
	n := b.AddNestedInstance("n")
	r := n.AddResource("r", nil)
	if err := m.AddResourceAlias("later-sibling", r); err != nil {
		t.Fatalf("alias in an earlier sibling: %v", err)
	}
	if err := b.AddResourceAlias("root", r); err != nil {
		t.Fatalf("alias at the root: %v", err)
	}
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	newInst := func() *host.ComponentInstance {
		inst, err := comp.Instantiate(ctx)
		if err != nil {
			t.Fatalf("Instantiate: %v", err)
		}
		t.Cleanup(func() { inst.Close(ctx) })
		return inst
	}
	inst, other := newInst(), newInst()

	want := inst.Core().ExportedInstance("n").ExportedType("r")
	if want == nil {
		t.Fatal("want n.r exported")
	}
	if got := inst.Core().ExportedType("root"); got != want {
		t.Errorf("root alias = %v, want n.r", got)
	}
	if got := inst.Core().ExportedInstance("m").ExportedType("later-sibling"); got != want {
		t.Errorf("sibling alias = %v, want n.r", got)
	}

	consumer := loadGuest(t, e, `(component
  (import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $r))
  (import "b" (instance (export "root" (type (eq $r))))))`)
	n1 := inst.Core().ExportedInstance("n")
	if _, err := consumer.Instantiate(ctx, wacogo.WithInstanceImport("a", n1), wacogo.WithInstanceImport("b", inst.Core())); err != nil {
		t.Fatalf("want the root alias checked as n.r, got: %v", err)
	}
	if _, err := consumer.Instantiate(ctx, wacogo.WithInstanceImport("a", other.Core().ExportedInstance("n")), wacogo.WithInstanceImport("b", inst.Core())); err == nil {
		t.Fatal("want another instantiation's resource to differ")
	}
}
