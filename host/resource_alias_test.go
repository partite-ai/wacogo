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

	wp := inst.Core().ParserInstanceType()
	rPrimary, ok := wp.ExportedResourceID("primary")
	if !ok {
		t.Fatalf("primary not in parser instance type")
	}
	rAlias, ok := wp.ExportedResourceID("alias")
	if !ok {
		t.Fatalf("alias not in parser instance type")
	}
	if rPrimary != rAlias {
		t.Fatalf("alias ResourceID mismatch: primary=%v alias=%v", rPrimary, rAlias)
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
