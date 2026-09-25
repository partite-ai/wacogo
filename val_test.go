package wacogo_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
)

// TestValList tests list construction, Len, and Get.
func TestValList(t *testing.T) {
	l := wacogo.NewValListOf(wacogo.ValS32(1), wacogo.ValS32(2), wacogo.ValS32(3))
	if l.Len() != 3 {
		t.Fatalf("expected Len=3, got %d", l.Len())
	}
	if l.Get(0) != wacogo.ValS32(1) {
		t.Errorf("expected Get(0)=1, got %v", l.Get(0))
	}
	if l.Get(2) != wacogo.ValS32(3) {
		t.Errorf("expected Get(2)=3, got %v", l.Get(2))
	}

	// empty list
	empty := wacogo.NewValListOf[wacogo.ValS32]()
	if empty.Len() != 0 {
		t.Errorf("expected empty list Len=0, got %d", empty.Len())
	}

	// ValList implements Val
	var _ wacogo.Val = l
}

// TestValRecord tests field access by name and index.
func TestValRecord(t *testing.T) {
	r := wacogo.NewValRecord(
		wacogo.Field{Name: "x", Val: wacogo.ValS32(10)},
		wacogo.Field{Name: "y", Val: wacogo.ValS32(20)},
	)

	// by name
	if r.Field("x") != wacogo.ValS32(10) {
		t.Errorf("expected Field(x)=10, got %v", r.Field("x"))
	}
	if r.Field("y") != wacogo.ValS32(20) {
		t.Errorf("expected Field(y)=20, got %v", r.Field("y"))
	}
	if r.Field("z") != nil {
		t.Errorf("expected Field(z)=nil, got %v", r.Field("z"))
	}

	// by index — order preserved
	if r.FieldByIndex(0) != wacogo.ValS32(10) {
		t.Errorf("expected FieldByIndex(0)=10, got %v", r.FieldByIndex(0))
	}
	if r.FieldByIndex(1) != wacogo.ValS32(20) {
		t.Errorf("expected FieldByIndex(1)=20, got %v", r.FieldByIndex(1))
	}

	// Fields slice
	fields := r.Fields()
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	if fields[0].Name != "x" || fields[1].Name != "y" {
		t.Errorf("field order not preserved: %v", fields)
	}

	// ValRecord implements Val
	var _ wacogo.Val = r
}

// TestValVariant tests discriminant and payload.
func TestValVariant(t *testing.T) {
	v := wacogo.NewValVariant(2, wacogo.ValString("hello"))
	if v.Discriminant() != 2 {
		t.Errorf("expected discriminant=2, got %d", v.Discriminant())
	}
	if v.Val() != wacogo.ValString("hello") {
		t.Errorf("expected payload=hello, got %v", v.Val())
	}

	// nil payload
	noPayload := wacogo.NewValVariant(0, nil)
	if noPayload.Val() != nil {
		t.Errorf("expected nil payload")
	}

	// ValVariant implements Val
	var _ wacogo.Val = v
}

// TestValEnum tests discriminant.
func TestValEnum(t *testing.T) {
	e := wacogo.NewValEnum(5)
	if e.Discriminant() != 5 {
		t.Errorf("expected discriminant=5, got %d", e.Discriminant())
	}
	var _ wacogo.Val = e
}

// TestValOption tests none/some behavior and panic on Val() when none.
func TestValOption(t *testing.T) {
	none := wacogo.ValOptionNone()
	if !none.IsNone() {
		t.Errorf("expected IsNone=true")
	}

	some := wacogo.ValOptionSome(wacogo.ValS32(42))
	if some.IsNone() {
		t.Errorf("expected IsNone=false")
	}
	if some.Val() != wacogo.ValS32(42) {
		t.Errorf("expected Val=42, got %v", some.Val())
	}

	// Val panics on none
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on none.Val()")
		}
	}()
	none.Val()

	// ValOption implements Val
	var _ wacogo.Val = some
}

// TestValResult tests ok/err accessors and panics.
func TestValResult(t *testing.T) {
	ok := wacogo.ValResultOk(wacogo.ValS32(1))
	if !ok.IsOk() {
		t.Errorf("expected IsOk=true")
	}
	if ok.Ok() != wacogo.ValS32(1) {
		t.Errorf("expected Ok=1")
	}

	err := wacogo.ValResultErr(wacogo.ValString("boom"))
	if err.IsOk() {
		t.Errorf("expected IsOk=false")
	}
	if err.Err() != wacogo.ValString("boom") {
		t.Errorf("expected Err=boom")
	}

	// Ok panics on err result
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic on err.Ok()")
			}
		}()
		err.Ok()
	}()

	// Err panics on ok result
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic on ok.Err()")
			}
		}()
		ok.Err()
	}()

	// ValResult implements Val
	var _ wacogo.Val = ok
}

// TestValFlags tests Has/Set/Clear.
func TestValFlags(t *testing.T) {
	f := wacogo.NewValFlags([]string{"read", "write", "exec"}, "read", "exec")
	if !f.Has("read") {
		t.Errorf("expected read flag set")
	}
	if f.Has("write") {
		t.Errorf("expected write flag not set")
	}
	if !f.Has("exec") {
		t.Errorf("expected exec flag set")
	}

	f.Set("write")
	if !f.Has("write") {
		t.Errorf("expected write flag set after Set")
	}

	f.Clear("read")
	if f.Has("read") {
		t.Errorf("expected read flag cleared")
	}

	// unknown name: no panic
	f.Set("unknown")
	f.Clear("unknown")
	if f.Has("unknown") {
		t.Errorf("expected Has(unknown)=false")
	}

	// Bits returns something
	bits := f.Bits()
	if bits == nil {
		t.Errorf("expected non-nil bits")
	}

	var _ wacogo.Val = f
}

// TestValOwnHandleDrop verifies the public Val handle surface: a
// *ValOwnHandle exposes Drop and satisfies Val. Construction is
// internal (lift visitors mint), so the test only checks the Val
// interface conformance and a no-op Drop on a zero-value handle.
func TestValOwnHandleDrop(t *testing.T) {
	var v wacogo.ValOwnHandle
	if err := v.Drop(context.Background()); err != nil {
		t.Errorf("zero-value Drop returned %v, want nil", err)
	}
	var _ wacogo.Val = &v
}

func TestNewValOwnHandleTransfersThroughFuncCall(t *testing.T) {
	ctx := context.Background()
	engine := wacogo.NewEngine(ctx)
	t.Cleanup(func() { _ = engine.Close(ctx) })

	var drops int
	builder := engine.NewHostBuilder("own-value-test")
	item := builder.AddResource("item", func(_ context.Context, _ *host.ComponentInstance, obj any) error {
		if obj != "payload" {
			return fmt.Errorf("dropped object = %v, want payload", obj)
		}
		drops++
		return nil
	})
	builder.AddFunction("consume", &host.FuncType{
		Params: []host.Param{{Name: "item", Type: item.Own()}},
	}, func(ctx context.Context, cc *host.CallContext, instance *host.ComponentInstance, stack []uint64) error {
		rt, ok := cc.Instance().ExportedType("item").(*wacogo.TypeResource)
		if !ok {
			return fmt.Errorf("item export = %T, want *wacogo.TypeResource", cc.Instance().ExportedType("item"))
		}
		handle, err := cc.LookupOwn(rt, uint32(stack[0]))
		if err != nil {
			return fmt.Errorf("lookup transferred own: %w", err)
		}
		if obj, ok := instance.LookupResource(host.ExternHandle(handle.Rep())); !ok || obj != "payload" {
			return fmt.Errorf("registered object = (%v, %v), want (payload, true)", obj, ok)
		}
		return handle.Drop(ctx)
	})

	component, err := builder.Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = component.Close(ctx) })
	instance, err := component.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close(ctx) })

	rt, ok := instance.Core().ExportedType("item").(*wacogo.TypeResource)
	if !ok {
		t.Fatalf("item export = %T, want *wacogo.TypeResource", instance.Core().ExportedType("item"))
	}
	extern := instance.RegisterResource("payload")
	own := wacogo.NewValOwnHandle(rt, uint32(extern))
	if _, err := instance.Core().ExportedFunc("consume").Call(ctx, own); err != nil {
		t.Fatalf("consume host-created own: %v", err)
	}
	if drops != 1 {
		t.Fatalf("resource drops = %d, want 1", drops)
	}
	if _, live := instance.LookupResource(extern); live {
		t.Fatal("transferred resource remained registered after callee drop")
	}
	if _, err := instance.Core().ExportedFunc("consume").Call(ctx, own); err == nil || !strings.Contains(err.Error(), "transferred") {
		t.Fatalf("second consume error = %v, want transferred ownership error", err)
	}
}

func TestNewValOwnHandleRejectsNilType(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewValOwnHandle(nil, ...) did not panic")
		}
	}()
	_ = wacogo.NewValOwnHandle(nil, 1)
}
