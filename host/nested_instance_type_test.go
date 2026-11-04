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
