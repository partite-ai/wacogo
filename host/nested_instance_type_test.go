package host_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
)

func loadGuest(t *testing.T, e *wacogo.Engine, wat string) *wacogo.Component {
	t.Helper()
	comp, err := e.LoadComponent(context.Background(), bytes.NewReader(watToBinary(t, wat)))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return comp
}

func TestNestedHostInstance_CheckedAgainstImport(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("nested-link")
	b.AddNestedInstance("n").AddFunction("f", &host.FuncType{
		Params: []host.Param{{Name: "x", Type: host.S32}},
	}, func(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error { return nil })
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
	n := inst.Core().ExportedInstance("n")

	mismatch := loadGuest(t, e, `(component (import "n" (instance (export "f" (func (param "x" string))))))`)
	if _, err := mismatch.Instantiate(ctx, wacogo.WithInstanceImport("n", n)); err == nil {
		t.Fatal("want mismatched nested host instance rejected")
	}
	match := loadGuest(t, e, `(component (import "n" (instance (export "f" (func (param "x" s32))))))`)
	if _, err := match.Instantiate(ctx, wacogo.WithInstanceImport("n", n)); err != nil {
		t.Fatalf("want matching nested host instance accepted, got: %v", err)
	}
}

func TestNestedHostInstance_ResourceIdentity(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("nested-link-resource")
	r := b.AddResource("r", nil)
	if err := b.AddNestedInstance("n").AddResourceAlias("r", r); err != nil {
		t.Fatalf("AddResourceAlias: %v", err)
	}
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	inst1, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst1.Close(ctx)
	inst2, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst2.Close(ctx)

	consumer := loadGuest(t, e, `(component
  (import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $r))
  (import "b" (instance (export "r" (type (eq $r))))))`)

	if _, err := consumer.Instantiate(ctx,
		wacogo.WithInstanceImport("a", inst1.Core()),
		wacogo.WithInstanceImport("b", inst1.Core().ExportedInstance("n"))); err != nil {
		t.Fatalf("want root and nested scope to share a resource, got: %v", err)
	}
	if _, err := consumer.Instantiate(ctx,
		wacogo.WithInstanceImport("a", inst1.Core()),
		wacogo.WithInstanceImport("b", inst2.Core().ExportedInstance("n"))); err == nil {
		t.Fatal("want resources of two host instantiations to differ")
	}
}

func TestResourceRef_LentByNestedInstance(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	lenderB := e.NewHostBuilder("nested-lender")
	lenderB.AddNestedInstance("n").AddResource("r", nil)
	lender, err := lenderB.Build(ctx)
	if err != nil {
		t.Fatalf("Build lender: %v", err)
	}
	defer lender.Close(ctx)
	l1, err := lender.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate lender: %v", err)
	}
	defer l1.Close(ctx)
	l2, err := lender.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate lender: %v", err)
	}
	defer l2.Close(ctx)

	borrowerB := e.NewHostBuilder("nested-borrower")
	ref := borrowerB.AddResourceRef("r")
	borrower, err := borrowerB.Build(ctx)
	if err != nil {
		t.Fatalf("Build borrower: %v", err)
	}
	defer borrower.Close(ctx)
	inst, err := borrower.Instantiate(ctx, host.WithResourceFrom(ref, l1.Core().ExportedInstance("n"), "r"))
	if err != nil {
		t.Fatalf("Instantiate borrower: %v", err)
	}
	defer inst.Close(ctx)

	consumer := loadGuest(t, e, `(component
  (import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $r))
  (import "b" (instance (export "r" (type (eq $r))))))`)

	if _, err := consumer.Instantiate(ctx,
		wacogo.WithInstanceImport("a", l1.Core().ExportedInstance("n")),
		wacogo.WithInstanceImport("b", inst.Core())); err != nil {
		t.Fatalf("want borrower to share the lender's resource, got: %v", err)
	}
	if _, err := consumer.Instantiate(ctx,
		wacogo.WithInstanceImport("a", l2.Core().ExportedInstance("n")),
		wacogo.WithInstanceImport("b", inst.Core())); err == nil {
		t.Fatal("want another lender's resource to differ")
	}
}

func TestResourceRef_LenderMissingExport(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	lender, err := e.NewHostBuilder("empty-lender").Build(ctx)
	if err != nil {
		t.Fatalf("Build lender: %v", err)
	}
	defer lender.Close(ctx)
	l, err := lender.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate lender: %v", err)
	}
	defer l.Close(ctx)

	b := e.NewHostBuilder("borrower")
	ref := b.AddResourceRef("r")
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build borrower: %v", err)
	}
	defer comp.Close(ctx)
	_, err = comp.Instantiate(ctx, host.WithResourceFrom(ref, l.Core(), "r"))
	if err == nil || !strings.Contains(err.Error(), `does not export type "r"`) {
		t.Fatalf("want a missing export reported, got: %v", err)
	}
}

func noopFunc(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error {
	return nil
}

const resourceFuncGuest = `(component
  (import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $r))
  (type $o (own $r))
  (import "f" (func (param "x" $o))))`

func TestHostFunc_ResourceIdentity(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("func-resource")
	r := b.AddResource("r", nil)
	own := &host.FuncType{Params: []host.Param{{Name: "x", Type: r.Own()}}}
	b.AddFunction("f", own, noopFunc)
	b.AddNestedInstance("n").AddFunction("g", own, noopFunc)
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	i1, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer i1.Close(ctx)
	i2, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer i2.Close(ctx)

	guest := loadGuest(t, e, resourceFuncGuest)
	for name, tc := range map[string]struct {
		fn      *wacogo.ExportedFunc
		wantErr bool
	}{
		"root func, same instance":    {fn: i1.Core().ExportedFunc("f")},
		"nested func, same instance":  {fn: i1.Core().ExportedInstance("n").ExportedFunc("g")},
		"root func, other instance":   {fn: i2.Core().ExportedFunc("f"), wantErr: true},
		"nested func, other instance": {fn: i2.Core().ExportedInstance("n").ExportedFunc("g"), wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := guest.Instantiate(ctx,
				wacogo.WithInstanceImport("a", i1.Core()),
				wacogo.WithFuncImport("f", tc.fn))
			if tc.wantErr != (err != nil) {
				t.Fatalf("wantErr=%v, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestHostFunc_ResourceRefIdentity(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	lenderB := e.NewHostBuilder("func-lender")
	lenderB.AddResource("r", nil)
	lender, err := lenderB.Build(ctx)
	if err != nil {
		t.Fatalf("Build lender: %v", err)
	}
	defer lender.Close(ctx)
	l1, err := lender.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate lender: %v", err)
	}
	defer l1.Close(ctx)
	l2, err := lender.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate lender: %v", err)
	}
	defer l2.Close(ctx)

	b := e.NewHostBuilder("func-borrower")
	ref := b.AddResourceRef("r")
	b.AddFunction("f", &host.FuncType{Params: []host.Param{{Name: "x", Type: ref.Own()}}}, noopFunc)
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build borrower: %v", err)
	}
	defer comp.Close(ctx)
	inst, err := comp.Instantiate(ctx, host.WithResourceFrom(ref, l1.Core(), "r"))
	if err != nil {
		t.Fatalf("Instantiate borrower: %v", err)
	}
	defer inst.Close(ctx)

	guest := loadGuest(t, e, resourceFuncGuest)
	if _, err := guest.Instantiate(ctx,
		wacogo.WithInstanceImport("a", l1.Core()),
		wacogo.WithFuncImport("f", inst.Core().ExportedFunc("f"))); err != nil {
		t.Fatalf("want the lender's resource to match, got: %v", err)
	}
	if _, err := guest.Instantiate(ctx,
		wacogo.WithInstanceImport("a", l2.Core()),
		wacogo.WithFuncImport("f", inst.Core().ExportedFunc("f"))); err == nil {
		t.Fatal("want another lender's resource to differ")
	}
}
