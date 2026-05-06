package host

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo/internal/core"
)

func newTestBuilder(t *testing.T) (*Builder, *core.Engine) {
	t.Helper()
	e := core.NewEngine(context.Background())
	t.Cleanup(func() { _ = e.Close(context.Background()) })
	return NewBuilder(e, "test"), e
}

func TestAddFunction_PrimitiveSig_Accumulates(t *testing.T) {
	b, _ := newTestBuilder(t)
	fn := func(_ context.Context, _ *core.CallContext, _ *ComponentInstance, _ []uint64) error { return nil }
	ty := &FuncType{
		Params:  []Param{{"a", U32}},
		Results: []ResultDecl{{"", U32}},
	}
	b.AddFunction("double", ty, fn)
	if len(b.root.funcs) != 1 {
		t.Fatalf("want 1 func decl, got %d", len(b.root.funcs))
	}
	if b.root.funcs[0].name != "double" || b.root.funcs[0].ty != ty {
		t.Fatal("func decl fields mismatched")
	}
}

func TestBuild_PrimitiveFunc_ProducesComponent(t *testing.T) {
	b, _ := newTestBuilder(t)
	ty := &FuncType{
		Params:  []Param{{"x", U32}},
		Results: []ResultDecl{{"", U32}},
	}
	b.AddFunction("double", ty, func(_ context.Context, _ *core.CallContext, _ *ComponentInstance, stack []uint64) error {
		stack[0] = stack[0] * 2
		return nil
	})

	comp, err := b.Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(context.Background())
	if comp == nil {
		t.Fatal("want non-nil *Component")
	}
	if comp.compiledStub == nil {
		t.Fatal("want non-nil compiledStub")
	}
}

func TestBuild_NominalInline_Rejected(t *testing.T) {
	b, _ := newTestBuilder(t)
	b.AddFunction("bad", &FuncType{
		Params: []Param{{"p", Record{Fields: []Field{{"x", U32}}}}},
	}, func(context.Context, *core.CallContext, *ComponentInstance, []uint64) error { return nil })
	_, err := b.Build(context.Background())
	if err == nil {
		t.Fatal("want inline-nominal error")
	}
}

func TestBuild_StructuralInline_Accepted(t *testing.T) {
	ctx := context.Background()
	b, _ := newTestBuilder(t)
	b.AddFunction("sum", &FuncType{
		Params:  []Param{{"xs", List{Elem: U32}}},
		Results: []ResultDecl{{"", U32}},
	}, func(context.Context, *core.CallContext, *ComponentInstance, []uint64) error { return nil })
	_, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
}

func TestAddType_ListHandle_Reusable(t *testing.T) {
	ctx := context.Background()
	b, _ := newTestBuilder(t)
	listU32 := b.AddType("list-u32", List{Elem: U32})
	b.AddFunction("sum", &FuncType{
		Params:  []Param{{"xs", listU32}},
		Results: []ResultDecl{{"", U32}},
	}, func(context.Context, *core.CallContext, *ComponentInstance, []uint64) error { return nil })
	if _, err := b.Build(ctx); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if listU32.declExpr == nil {
		t.Fatal("want *TypeRef carrying its declExpr after AddType")
	}
}

func TestBuild_DuplicateFuncName_Errors(t *testing.T) {
	b, _ := newTestBuilder(t)
	ty := &FuncType{}
	fn := func(_ context.Context, _ *core.CallContext, _ *ComponentInstance, _ []uint64) error { return nil }
	b.AddFunction("hi", ty, fn)
	b.AddFunction("hi", ty, fn)
	if _, err := b.Build(context.Background()); err == nil {
		t.Fatal("want duplicate-name error")
	}
}

func TestBuilder_AddResourceRef_ReturnsDistinctRefs(t *testing.T) {
	ctx := context.Background()
	e := core.NewEngine(ctx)
	defer e.Close(ctx)

	b := NewBuilder(e, "rrtest")
	a := b.AddResourceRef("a")
	bRef := b.AddResourceRef("b")
	if a == bRef {
		t.Fatal("AddResourceRef should return distinct refs for distinct names")
	}
}

func TestBuilder_AddResourceRef_DuplicateNameRejectedAtBuild(t *testing.T) {
	ctx := context.Background()
	e := core.NewEngine(ctx)
	defer e.Close(ctx)

	b := NewBuilder(e, "rrtest_dup")
	b.AddResourceRef("dup")
	b.AddResourceRef("dup")
	if _, err := b.Build(ctx); err == nil {
		t.Fatal("Build with duplicate AddResourceRef name: want error, got nil")
	}
}

func TestAddResourceAllocatesComponentResourceTypeID(t *testing.T) {
	b, _ := newTestBuilder(t)

	a := b.AddResource("a", nil)
	bRT := b.AddResource("b", nil)

	if a.componentResourceTypeID != 0 {
		t.Errorf("first AddResource: componentResourceTypeID = %d, want 0", a.componentResourceTypeID)
	}
	if bRT.componentResourceTypeID != 1 {
		t.Errorf("second AddResource: componentResourceTypeID = %d, want 1", bRT.componentResourceTypeID)
	}
}

func TestAddResourceRefAllocatesComponentResourceTypeIDAfterResources(t *testing.T) {
	b, _ := newTestBuilder(t)

	rt := b.AddResource("r", nil)
	ref1 := b.AddResourceRef("ref1")
	ref2 := b.AddResourceRef("ref2")

	if rt.componentResourceTypeID != 0 {
		t.Errorf("AddResource: componentResourceTypeID = %d, want 0", rt.componentResourceTypeID)
	}
	if ref1.componentResourceTypeID != 1 {
		t.Errorf("first AddResourceRef: componentResourceTypeID = %d, want 1", ref1.componentResourceTypeID)
	}
	if ref2.componentResourceTypeID != 2 {
		t.Errorf("second AddResourceRef: componentResourceTypeID = %d, want 2", ref2.componentResourceTypeID)
	}
}
