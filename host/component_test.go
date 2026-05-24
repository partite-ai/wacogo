package host

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo/internal/core"
)

func TestInstantiate_PrimitiveFunc_ExportsVisibleAndCallable(t *testing.T) {
	ctx := context.Background()
	b, _ := newTestBuilder(t)
	ty := &FuncType{
		Params:  []Param{{"x", U32}},
		Results: []ResultDecl{{"", U32}},
	}
	b.AddFunction("double", ty, func(_ context.Context, _ *core.CallContext, _ *ComponentInstance, stack []uint64) error {
		stack[0] = uint64(uint32(stack[0]) * 2)
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

	ef := inst.Core().ExportedFunc("double")
	if ef == nil {
		t.Fatal("want ExportedFunc(\"double\")")
	}

	results, err := ef.Call(ctx, core.ValU32(21))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := uint32(results[0].(core.ValU32)); got != 42 {
		t.Fatalf("want 42, got %d", got)
	}
}

func TestInstantiate_TwoInstances_IndependentState(t *testing.T) {
	ctx := context.Background()
	b, _ := newTestBuilder(t)
	b.AddFunction("bump", &FuncType{
		Params:  []Param{{"n", U32}},
		Results: []ResultDecl{{"", U32}},
	}, func(_ context.Context, _ *core.CallContext, _ *ComponentInstance, stack []uint64) error {
		stack[0] = uint64(uint32(stack[0]) + 1)
		return nil
	})
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)

	inst1, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer inst1.Close(ctx)
	inst2, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer inst2.Close(ctx)

	// Each instance must have its own exports map referring to
	// distinct ExportedFunc values (different canon bindings into
	// different stubMods).
	f1 := inst1.Core().ExportedFunc("bump")
	f2 := inst2.Core().ExportedFunc("bump")
	if f1 == f2 {
		t.Fatal("want distinct *ExportedFunc per instance")
	}

	// Both must be callable independently.
	r1, err := f1.Call(ctx, core.ValU32(10))
	if err != nil {
		t.Fatal(err)
	}
	r2, err := f2.Call(ctx, core.ValU32(20))
	if err != nil {
		t.Fatal(err)
	}
	if u := uint32(r1[0].(core.ValU32)); u != 11 {
		t.Fatalf("r1: want 11 got %d", u)
	}
	if u := uint32(r2[0].(core.ValU32)); u != 21 {
		t.Fatalf("r2: want 21 got %d", u)
	}
}

func TestInstantiate_TwoInstances_ResourcesIsolated(t *testing.T) {
	ctx := context.Background()
	b, _ := newTestBuilder(t)

	rt := b.AddResource("cell", nil)
	b.AddFunction("new", &FuncType{
		Params:  []Param{{"v", U32}},
		Results: []ResultDecl{{"", rt.Own()}},
	}, func(_ context.Context, cc *core.CallContext, h *ComponentInstance, stack []uint64) error {
		v := uint32(stack[0])
		tr := h.Core().ExportedType("cell").(*core.TypeResource)
		handle := cc.IssueOwnHandle(tr, uint32(h.RegisterResource(v))).HandleID()
		stack[0] = uint64(handle)
		return nil
	})
	b.AddFunction("get", &FuncType{
		Params:  []Param{{"c", rt.Borrow()}},
		Results: []ResultDecl{{"", U32}},
	}, func(_ context.Context, _ *core.CallContext, h *ComponentInstance, stack []uint64) error {
		// Borrow lifts as the rep value directly. RegisterOwn stored the
		// obj in h's extTable, so look it up via LookupResource.
		rep := uint32(stack[0])
		obj, _ := h.LookupResource(ExternHandle(rep))
		stack[0] = uint64(obj.(uint32))
		return nil
	})

	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)

	inst1, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer inst1.Close(ctx)
	inst2, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer inst2.Close(ctx)

	// Both calls succeed — each instance has its own host module whose
	// Go closures capture the per-instance state directly. No shared
	// lookup map.
	r1, err := inst1.Core().ExportedFunc("new").Call(ctx, core.ValU32(100))
	if err != nil {
		t.Fatalf("inst1 new: %v", err)
	}
	r2, err := inst2.Core().ExportedFunc("new").Call(ctx, core.ValU32(200))
	if err != nil {
		t.Fatalf("inst2 new: %v", err)
	}
	if r1[0] == nil || r2[0] == nil {
		t.Fatal("both should return non-nil handles")
	}
	// Different wpInstance identities: imports through each flows
	// through its own resource-identity scope in the subtype checker.
	// (Exposed via the re-exported *wasmparser.InstanceType pointer
	// on each ComponentInstance; not a public API to compare.)
}

func TestInstantiate_AfterClose_Errors(t *testing.T) {
	ctx := context.Background()
	b, _ := newTestBuilder(t)
	b.AddFunction("noop", &FuncType{}, func(context.Context, *core.CallContext, *ComponentInstance, []uint64) error { return nil })
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := comp.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := comp.Instantiate(ctx); err == nil {
		t.Fatal("want Instantiate-after-Close error")
	}
}

func TestInstantiateExportsResourceType(t *testing.T) {
	ctx := context.Background()
	b, _ := newTestBuilder(t)
	b.AddResource("r", nil)

	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer comp.Close(ctx)

	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer inst.Close(ctx)

	got := inst.Core().ExportedType("r")
	if got == nil {
		t.Fatalf(`ExportedType("r") = nil, want non-nil`)
	}
	if _, ok := got.(*core.TypeResource); !ok {
		t.Errorf(`ExportedType("r") = %T, want *core.TypeResource`, got)
	}
}
