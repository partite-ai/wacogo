package host_test

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
)

func TestAddNestedInstance_ExportsNested(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("nested-test")
	nested := b.AddNestedInstance("nested")
	nested.AddFunction("inner", &host.FuncType{
		Results: []host.ResultDecl{{Name: "", Type: host.U32}},
	}, func(_ context.Context, _ *core.CallContext, _ *host.ComponentInstance, stack []uint64) error {
		stack[0] = 42
		return nil
	})

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

	sub := inst.Core().ExportedInstance("nested")
	if sub == nil {
		t.Fatal("ExportedInstance(\"nested\") is nil")
	}
	fn := sub.ExportedFunc("inner")
	if fn == nil {
		t.Fatal("nested.inner is not exported")
	}
	results, err := fn.Call(ctx)
	if err != nil {
		t.Fatalf("inner Call: %v", err)
	}
	if len(results) != 1 || results[0] != core.ValU32(42) {
		t.Fatalf("inner returned %v, want [ValU32(42)]", results)
	}
}

func TestAddNestedInstance_NameCollisionWithinScope_ErrorsAtBuild(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("nested-collide")
	b.AddFunction("name", &host.FuncType{}, func(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error { return nil })
	_ = b.AddNestedInstance("name") // collides with the func above
	if _, err := b.Build(ctx); err == nil {
		t.Fatal("Build with name collision: want error, got nil")
	}
}

func TestAddNestedInstance_SiblingScopesMayShareNames(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("nested-siblings")
	a := b.AddNestedInstance("a")
	bSub := b.AddNestedInstance("b")
	a.AddFunction("shared", &host.FuncType{}, func(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error { return nil })
	bSub.AddFunction("shared", &host.FuncType{}, func(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error { return nil })

	if _, err := b.Build(ctx); err != nil {
		t.Fatalf("Build: %v", err)
	}
}
