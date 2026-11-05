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

func TestResourceTypeImport_HostLender(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("type-import-lender")
	r := b.AddResource("r", nil)
	b.AddNestedInstance("n").AddResource("s", nil)
	if err := b.AddNestedInstance("m").AddResourceAlias("r", r); err != nil {
		t.Fatalf("AddResourceAlias: %v", err)
	}
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	l1, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer l1.Close(ctx)
	l2, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer l2.Close(ctx)

	consumer := loadGuest(t, e, `(component
  (import "r" (type $r (sub resource)))
  (import "a" (instance (export "r" (type (eq $r))))))`)

	for _, tc := range []struct {
		name     string
		lender   *core.ComponentInstance
		instance *core.ComponentInstance
		wantErr  bool
	}{
		{"root and own root", l1.Core(), l1.Core(), false},
		{"root and own nested scope", l1.Core(), l1.Core().ExportedInstance("m"), false},
		{"nested scope and root", l1.Core().ExportedInstance("m"), l1.Core(), false},
		{"another instantiation", l1.Core(), l2.Core(), true},
		{"another instantiation's nested scope", l1.Core(), l2.Core().ExportedInstance("m"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := consumer.Instantiate(ctx,
				wacogo.WithResourceTypeImport("r", tc.lender, "r"),
				wacogo.WithInstanceImport("a", tc.instance))
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got: %v", tc.wantErr, err)
			}
		})
	}

	nested := loadGuest(t, e, `(component (import "r" (type (sub resource))))`)
	if _, err := nested.Instantiate(ctx, wacogo.WithResourceTypeImport("r", l1.Core().ExportedInstance("n"), "s")); err != nil {
		t.Fatalf("want a resource defined in a nested scope accepted, got: %v", err)
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

// hostLender returns an instance of a host component exporting a resource "r".
func hostLender(t *testing.T, ctx context.Context, comp *host.Component) *core.ComponentInstance {
	t.Helper()
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate lender: %v", err)
	}
	t.Cleanup(func() { inst.Close(ctx) })
	return inst.Core()
}

func newHostLenderComponent(t *testing.T, ctx context.Context, e *wacogo.Engine) *host.Component {
	t.Helper()
	b := e.NewHostBuilder("lender")
	b.AddResource("r", nil)
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build lender: %v", err)
	}
	t.Cleanup(func() { comp.Close(ctx) })
	return comp
}

func noopHostFunc(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error {
	return nil
}

func TestNestedResourceRef(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)
	lender := newHostLenderComponent(t, ctx, e)
	l1, l2 := hostLender(t, ctx, lender), hostLender(t, ctx, lender)

	b := e.NewHostBuilder("nested-ref")
	n := b.AddNestedInstance("n")
	ref := n.AddResourceRef("r")
	n.AddFunction("f", &host.FuncType{Params: []host.Param{{Name: "x", Type: ref.Own()}}}, noopHostFunc)
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	inst, err := comp.Instantiate(ctx, host.WithResourceFrom(ref, l1, "r"))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)
	nested := inst.Core().ExportedInstance("n")

	if got := nested.ExportedType("r"); got == nil || got != l1.ExportedType("r") {
		t.Fatalf("n.r = %v, want the lender's resource", got)
	}
	if got := inst.Core().ExportedType("r"); got != nil {
		t.Fatalf("the root exports r = %v, want it only in n", got)
	}

	eq := loadGuest(t, e, `(component
  (import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $r))
  (import "b" (instance (export "n" (instance (export "r" (type (eq $r))))))))`)
	if _, err := eq.Instantiate(ctx, wacogo.WithInstanceImport("a", l1), wacogo.WithInstanceImport("b", inst.Core())); err != nil {
		t.Fatalf("want n.r to be the lender's resource, got: %v", err)
	}
	if _, err := eq.Instantiate(ctx, wacogo.WithInstanceImport("a", l2), wacogo.WithInstanceImport("b", inst.Core())); err == nil {
		t.Fatal("want n.r to differ from another lender's resource")
	}

	fn := loadGuest(t, e, `(component
  (import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $r))
  (type $o (own $r))
  (import "f" (func (param "x" $o))))`)
	f := nested.ExportedFunc("f")
	if _, err := fn.Instantiate(ctx, wacogo.WithInstanceImport("a", l1), wacogo.WithFuncImport("f", f)); err != nil {
		t.Fatalf("want n.f to name the lender's resource, got: %v", err)
	}
	if _, err := fn.Instantiate(ctx, wacogo.WithInstanceImport("a", l2), wacogo.WithFuncImport("f", f)); err == nil {
		t.Fatal("want n.f not to name another lender's resource")
	}
}

// Refs with one name in different scopes are separate placeholders.
func TestNestedResourceRef_SameNameInTwoScopes(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)
	lender := newHostLenderComponent(t, ctx, e)
	l1, l2 := hostLender(t, ctx, lender), hostLender(t, ctx, lender)

	b := e.NewHostBuilder("same-name")
	rootRef := b.AddResourceRef("r")
	nRef := b.AddNestedInstance("n").AddResourceRef("r")
	mRef := b.AddNestedInstance("m").AddResourceRef("r")
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	inst, err := comp.Instantiate(ctx,
		host.WithResourceFrom(rootRef, l2, "r"),
		host.WithResourceFrom(nRef, l1, "r"),
		host.WithResourceFrom(mRef, l2, "r"))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	consumer := loadGuest(t, e, `(component
  (import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $r))
  (import "b" (instance (export "r" (type (eq $r))))))`)
	for _, c := range []struct {
		name    string
		lender  *core.ComponentInstance
		inst    *core.ComponentInstance
		wantErr bool
	}{
		{"root is its lender's", l2, inst.Core(), false},
		{"root is not another's", l1, inst.Core(), true},
		{"n is its lender's", l1, inst.Core().ExportedInstance("n"), false},
		{"n is not another's", l2, inst.Core().ExportedInstance("n"), true},
		{"m is its lender's", l2, inst.Core().ExportedInstance("m"), false},
		{"m is not another's", l1, inst.Core().ExportedInstance("m"), true},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := consumer.Instantiate(ctx, wacogo.WithInstanceImport("a", c.lender), wacogo.WithInstanceImport("b", c.inst))
			if (err != nil) != c.wantErr {
				t.Fatalf("wantErr=%v, got: %v", c.wantErr, err)
			}
		})
	}
}

func TestNestedResourceRef_Deeper(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)
	lender := newHostLenderComponent(t, ctx, e)
	l1 := hostLender(t, ctx, lender)

	b := e.NewHostBuilder("deeper")
	ref := b.AddNestedInstance("n").AddNestedInstance("m").AddResourceRef("r")
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	inst, err := comp.Instantiate(ctx, host.WithResourceFrom(ref, l1, "r"))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)
	if got := inst.Core().ExportedInstance("n").ExportedInstance("m").ExportedType("r"); got != l1.ExportedType("r") {
		t.Fatalf("n.m.r = %v, want the lender's resource", got)
	}
}

func TestNestedResourceRef_Unbound(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	b := e.NewHostBuilder("unbound")
	b.AddNestedInstance("n").AddResourceRef("r")
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	if _, err := comp.Instantiate(ctx); err == nil || !strings.Contains(err.Error(), "no WithResourceFrom binding") {
		t.Fatalf("want a missing binding reported, got: %v", err)
	}
}

// A ref can be used from any scope, whichever declares it.
func TestResourceRef_UsedAcrossScopes(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)
	lender := newHostLenderComponent(t, ctx, e)
	l1, l2 := hostLender(t, ctx, lender), hostLender(t, ctx, lender)

	b := e.NewHostBuilder("across-scopes")
	n := b.AddNestedInstance("n")
	nestedRef := n.AddResourceRef("r")
	rootRef := b.AddResourceRef("s")
	b.AddFunction("root-uses-nested", &host.FuncType{Params: []host.Param{{Name: "x", Type: nestedRef.Own()}}}, noopHostFunc)
	n.AddFunction("nested-uses-root", &host.FuncType{Params: []host.Param{{Name: "x", Type: rootRef.Own()}}}, noopHostFunc)
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	inst, err := comp.Instantiate(ctx, host.WithResourceFrom(nestedRef, l1, "r"), host.WithResourceFrom(rootRef, l2, "r"))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	consumer := loadGuest(t, e, `(component
  (import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $r))
  (type $o (own $r))
  (import "f" (func (param "x" $o))))`)
	rootUsesNested := inst.Core().ExportedFunc("root-uses-nested")
	nestedUsesRoot := inst.Core().ExportedInstance("n").ExportedFunc("nested-uses-root")
	for _, c := range []struct {
		name    string
		lender  *core.ComponentInstance
		f       *core.ExportedFunc
		wantErr bool
	}{
		{"root func names the nested ref's lender", l1, rootUsesNested, false},
		{"root func names no other", l2, rootUsesNested, true},
		{"nested func names the root ref's lender", l2, nestedUsesRoot, false},
		{"nested func names no other", l1, nestedUsesRoot, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := consumer.Instantiate(ctx, wacogo.WithInstanceImport("a", c.lender), wacogo.WithFuncImport("f", c.f))
			if (err != nil) != c.wantErr {
				t.Fatalf("wantErr=%v, got: %v", c.wantErr, err)
			}
		})
	}
}

// Resources and refs share an ID space, whichever scopes declare them.
func TestNestedResourceRef_DeclaredBeforeResource(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)
	lender := newHostLenderComponent(t, ctx, e)
	l1 := hostLender(t, ctx, lender)

	b := e.NewHostBuilder("ref-before-resource")
	ref := b.AddNestedInstance("n").AddResourceRef("r")
	b.AddResource("own", nil)
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	inst, err := comp.Instantiate(ctx, host.WithResourceFrom(ref, l1, "r"))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)
	if inst.Core().ExportedType("own") == nil {
		t.Fatal("want the component's own resource exported")
	}
}

func TestResourceRef_LenderExportIsNotAResource(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	lender, err := loadGuest(t, e, `(component (type $u u32) (export "u" (type $u)))`).Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate lender: %v", err)
	}
	b := e.NewHostBuilder("non-resource")
	ref := b.AddResourceRef("r")
	comp, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer comp.Close(ctx)
	if _, err := comp.Instantiate(ctx, host.WithResourceFrom(ref, lender, "u")); err == nil || !strings.Contains(err.Error(), "want *TypeResource") {
		t.Fatalf("want a non-resource export rejected, got: %v", err)
	}
}
