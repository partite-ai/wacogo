package host_test

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
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
