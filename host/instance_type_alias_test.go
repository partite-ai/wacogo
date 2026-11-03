package host_test

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
)

// instantiateGuest loads guestWAT and instantiates it with the given import
// options, returning the instance and the instantiation error.
func instantiateGuest(t *testing.T, e *wacogo.Engine, guestWAT string, opts ...wacogo.InstantiateOption) (*wacogo.ComponentInstance, error) {
	t.Helper()
	ctx := context.Background()
	comp, err := e.LoadComponent(ctx, bytes.NewReader(watToBinary(t, guestWAT)))
	if err != nil {
		t.Fatalf("guest LoadComponent: %v", err)
	}
	inst, err := comp.Instantiate(ctx, opts...)
	if err == nil {
		t.Cleanup(func() { _ = inst.Close(ctx) })
	}
	return inst, err
}

// buildProvider builds and instantiates a host instance configured by fn.
func buildProvider(t *testing.T, e *wacogo.Engine, fn func(b *host.Builder)) *host.ComponentInstance {
	t.Helper()
	ctx := context.Background()
	b := e.NewHostBuilder("provider")
	fn(b)
	c, err := b.Build(ctx)
	if err != nil {
		t.Fatalf("provider Build: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(ctx) })
	inst, err := c.Instantiate(ctx)
	if err != nil {
		t.Fatalf("provider Instantiate: %v", err)
	}
	t.Cleanup(func() { _ = inst.Close(ctx) })
	return inst
}

// sameType reports whether two types are equal, comparing resources by identity.
// reflect.DeepEqual looks inside a *core.TypeResource instead, and so can't tell
// apart two resources of one guest instance that have no destructors.
func sameType(a, b core.Type) bool {
	switch a := a.(type) {
	case *core.TypeResource:
		b, ok := b.(*core.TypeResource)
		return ok && a == b
	case core.TypeOwn:
		b, ok := b.(core.TypeOwn)
		return ok && a.ResourceType == b.ResourceType
	case core.TypeBorrow:
		b, ok := b.(core.TypeBorrow)
		return ok && a.ResourceType == b.ResourceType
	case core.TypeList:
		b, ok := b.(core.TypeList)
		return ok && sameType(a.Elem, b.Elem)
	case core.TypeOption:
		b, ok := b.(core.TypeOption)
		return ok && sameType(a.Inner, b.Inner)
	case core.TypeResult:
		b, ok := b.(core.TypeResult)
		return ok && sameType(a.Ok, b.Ok) && sameType(a.Err, b.Err)
	case core.TypeTuple:
		b, ok := b.(core.TypeTuple)
		return ok && slices.EqualFunc(a.Types, b.Types, sameType)
	case core.TypeRecord:
		b, ok := b.(core.TypeRecord)
		return ok && slices.EqualFunc(a.Fields, b.Fields, func(x, y core.FieldType) bool {
			return x.Name == y.Name && sameType(x.Type, y.Type)
		})
	case core.TypeVariant:
		b, ok := b.(core.TypeVariant)
		return ok && slices.EqualFunc(a.Cases, b.Cases, func(x, y core.CaseType) bool {
			return x.Name == y.Name && sameType(x.Payload, y.Payload)
		})
	}
	return reflect.DeepEqual(a, b)
}

func addNoopFunc(b *host.Builder) {
	b.AddFunction("f", &host.FuncType{}, func(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error { return nil })
}

// A runtime-empty import needs no provider; the alias takes its type from the
// import's declaration.
func TestInstanceTypeAlias_NoProvider(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	inst, err := instantiateGuest(t, e, `(component
  (import "foo" (instance $i
    (type $t' u64)
    (export "t" (type (eq $t')))
  ))
  (alias export $i "t" (type $t))
  (export "out" (type $t))
)`)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	if got := inst.ExportedType("out"); !reflect.DeepEqual(got, core.TypeU64{}) {
		t.Errorf("out = %#v, want u64", got)
	}
}

// Every value-type shape resolves from the declaration when the provider
// supplies only the import's functions.
func TestInstanceTypeAlias_ProviderOmitsTypes(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	prov := buildProvider(t, e, addNoopFunc)
	inst, err := instantiateGuest(t, e, `(component
  (import "foo" (instance $i
    (type $u u64)
    (export "duration" (type (eq $u)))
    (type $e (enum "a" "b"))
    (export "e" (type (eq $e)))
    (type $fl (flags "x" "y"))
    (export "fl" (type (eq $fl)))
    (type $r (record (field "d" u64) (field "e" $e)))
    (export "r" (type (eq $r)))
    (type $v (variant (case "none") (case "some" $r)))
    (export "v" (type (eq $v)))
    (type $l (list string))
    (export "l" (type (eq $l)))
    (type $tu (tuple u8 $e))
    (export "tu" (type (eq $tu)))
    (type $o (option u32))
    (export "o" (type (eq $o)))
    (type $res (result $e (error string)))
    (export "res" (type (eq $res)))
    (type $fn (func (param "a" $e) (result u32)))
    (export "fn" (type (eq $fn)))
    (export "f" (func))
  ))
  (alias export $i "duration" (type $duration))
  (alias export $i "r" (type $rec))
  (alias export $i "v" (type $var))
  (alias export $i "l" (type $list))
  (alias export $i "tu" (type $tup))
  (alias export $i "o" (type $opt))
  (alias export $i "res" (type $res))
  (alias export $i "fl" (type $fl))
  (alias export $i "fn" (type $fn))
  (export "duration-out" (type $duration))
  (export "r-out" (type $rec))
  (export "v-out" (type $var))
  (export "l-out" (type $list))
  (export "tu-out" (type $tup))
  (export "o-out" (type $opt))
  (export "res-out" (type $res))
  (export "fl-out" (type $fl))
)`, wacogo.WithInstanceImport("foo", prov.Core()))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}

	enum := core.TypeEnum{Cases: []string{"a", "b"}}
	rec := core.TypeRecord{Fields: []core.FieldType{
		{Name: "d", Type: core.TypeU64{}},
		{Name: "e", Type: enum},
	}}
	want := map[string]core.Type{
		"duration-out": core.TypeU64{},
		"r-out":        rec,
		"v-out":        core.TypeVariant{Cases: []core.CaseType{{Name: "none"}, {Name: "some", Payload: rec}}},
		"l-out":        core.TypeList{Elem: core.TypeString{}},
		"tu-out":       core.TypeTuple{Types: []core.Type{core.TypeU8{}, enum}},
		"o-out":        core.TypeOption{Inner: core.TypeU32{}},
		"res-out":      core.TypeResult{Ok: enum, Err: core.TypeString{}},
		"fl-out":       core.TypeFlags{Names: []string{"x", "y"}},
	}
	for name, w := range want {
		if got := inst.ExportedType(name); !reflect.DeepEqual(got, w) {
			t.Errorf("%s = %#v, want %#v", name, got, w)
		}
	}
}

// A provider exporting the same type still links; one exporting a conflicting
// type is rejected rather than silently overridden by the declaration.
func TestInstanceTypeAlias_ProviderExportsType(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	guest := `(component
  (import "foo" (instance $i
    (type $r (record (field "d" u64)))
    (export "r" (type (eq $r)))
  ))
  (alias export $i "r" (type $rec))
)`
	same := buildProvider(t, e, func(b *host.Builder) {
		b.AddType("r", host.Record{Fields: []host.Field{{Name: "d", Type: host.U64}}})
	})
	if _, err := instantiateGuest(t, e, guest, wacogo.WithInstanceImport("foo", same.Core())); err != nil {
		t.Errorf("matching provider: %v", err)
	}

	conflict := buildProvider(t, e, func(b *host.Builder) {
		b.AddType("r", host.Record{Fields: []host.Field{{Name: "d", Type: host.U32}}})
	})
	if _, err := instantiateGuest(t, e, guest, wacogo.WithInstanceImport("foo", conflict.Core())); err == nil {
		t.Error("conflicting provider was accepted")
	}
}

// A required resource still has to come from the provider.
func TestInstanceTypeAlias_ProviderOmitsResource(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	prov := buildProvider(t, e, addNoopFunc)
	_, err := instantiateGuest(t, e, `(component
  (import "foo" (instance $i
    (export "res" (type (sub resource)))
    (export "f" (func))
  ))
  (alias export $i "res" (type $r))
)`, wacogo.WithInstanceImport("foo", prov.Core()))
	if err == nil || !strings.Contains(err.Error(), "res") {
		t.Fatalf("err = %v, want missing export res", err)
	}
}

// An alias through a nested instance sees the nested declaration, and instance
// aliases keep later instance indices aligned with the runtime index space.
func TestInstanceTypeAlias_NestedInstance(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	prov := buildProvider(t, e, func(b *host.Builder) {
		b.AddNestedInstance("nested")
	})
	inst, err := instantiateGuest(t, e, `(component
  (import "a" (instance $a
    (export "nested" (instance
      (type $t (record (field "x" u32)))
      (export "t" (type (eq $t)))
    ))
  ))
  (import "b" (instance $b
    (type $t string)
    (export "t" (type (eq $t)))
  ))
  (alias export $a "nested" (instance $n))
  (import "c" (instance $c
    (type $t u8)
    (export "t" (type (eq $t)))
  ))
  (alias export $n "t" (type $nt))
  (alias export $c "t" (type $ct))
  (export "nt" (type $nt))
  (export "ct" (type $ct))
)`, wacogo.WithInstanceImport("a", prov.Core()))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	wantNT := core.TypeRecord{Fields: []core.FieldType{{Name: "x", Type: core.TypeU32{}}}}
	if got := inst.ExportedType("nt"); !reflect.DeepEqual(got, wantNT) {
		t.Errorf("nt = %#v, want %#v", got, wantNT)
	}
	if got := inst.ExportedType("ct"); !reflect.DeepEqual(got, core.TypeU8{}) {
		t.Errorf("ct = %#v, want u8", got)
	}
}

// Compound types over a resource take their structure from the declaration and
// the resource's identity from the provider.
func TestInstanceTypeAlias_CompoundOverResource(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	prov := buildProvider(t, e, func(b *host.Builder) {
		b.AddResource("res", nil)
	})
	inst, err := instantiateGuest(t, e, `(component
  (import "foo" (instance $i
    (export "res" (type $r (sub resource)))
    (type $o (own $r))
    (type $l (list $o))
    (export "owned" (type (eq $l)))
  ))
  (alias export $i "owned" (type $owned))
  (alias export $i "res" (type $res))
  (export "owned-out" (type $owned))
  (export "res-out" (type $res))
)`, wacogo.WithInstanceImport("foo", prov.Core()))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	res, ok := prov.Core().ExportedType("res").(*core.TypeResource)
	if !ok {
		t.Fatalf("provider res = %T", prov.Core().ExportedType("res"))
	}
	if got := inst.ExportedType("res-out"); got != core.Type(res) {
		t.Errorf("res-out = %#v, want provider resource", got)
	}
	want := core.TypeList{Elem: core.TypeOwn{ResourceType: res}}
	if got := inst.ExportedType("owned-out"); !sameType(got, want) {
		t.Errorf("owned-out = %#v, want %#v", got, want)
	}
}

// `type headers = fields`: the provider has to export the defining name, and the
// alias resolves through it.
func TestInstanceTypeAlias_ResourceAlias(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	guest := `(component
  (import "foo" (instance $i
    (export "fields" (type $f (sub resource)))
    (export "headers" (type (eq $f)))
  ))
  (alias export $i "headers" (type $h))
  (alias export $i "fields" (type $f))
  (export "h" (type $h))
  (export "f" (type $f))
)`
	prov := buildProvider(t, e, func(b *host.Builder) { b.AddResource("fields", nil) })
	inst, err := instantiateGuest(t, e, guest, wacogo.WithInstanceImport("foo", prov.Core()))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	res := prov.Core().ExportedType("fields")
	if got := inst.ExportedType("h"); got != res {
		t.Errorf("h = %#v, want provider resource %#v", got, res)
	}
	if got := inst.ExportedType("f"); got != res {
		t.Errorf("f = %#v, want provider resource %#v", got, res)
	}

	aliasOnly := buildProvider(t, e, func(b *host.Builder) { b.AddResource("headers", nil) })
	_, err = instantiateGuest(t, e, guest, wacogo.WithInstanceImport("foo", aliasOnly.Core()))
	if err == nil || !strings.Contains(err.Error(), "fields") {
		t.Errorf("err = %v, want missing export fields", err)
	}
}

// A resource defined inside an instantiated subcomponent doesn't exist until that
// instance does, so a type mentioning it has to be read from the instance. Each
// instantiation has its own resource.
func TestInstanceTypeAlias_SubcomponentResource(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	inst, err := instantiateGuest(t, e, `(component
  (component $sub
    (type $r (resource (rep i32)))
    (export $e "r" (type $r))
    (type $o (own $e))
    (type $l (list $o))
    (export "l" (type $l))
  )
  (instance $x1 (instantiate $sub))
  (instance $x2 (instantiate $sub))
  (export "i1" (instance $x1))
  (export "i2" (instance $x2))
  (alias export $x1 "r" (type $r1))
  (alias export $x2 "r" (type $r2))
  (alias export $x1 "l" (type $l1))
  (alias export $x2 "l" (type $l2))
  (export "r1" (type $r1))
  (export "r2" (type $r2))
  (export "l1" (type $l1))
  (export "l2" (type $l2))
)`)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	r1 := inst.ExportedType("r1")
	r2 := inst.ExportedType("r2")
	if r1 == nil || r2 == nil || r1 == r2 {
		t.Fatalf("r1 = %p, r2 = %p, want distinct resources", r1, r2)
	}
	for _, tc := range []struct {
		list string
		want core.Type
	}{{"l1", r1}, {"l2", r2}} {
		list, ok := inst.ExportedType(tc.list).(core.TypeList)
		if !ok {
			t.Fatalf("%s = %#v, want a list", tc.list, inst.ExportedType(tc.list))
		}
		own, ok := list.Elem.(core.TypeOwn)
		if !ok {
			t.Fatalf("%s element = %#v, want own", tc.list, list.Elem)
		}
		if core.Type(own.ResourceType) != tc.want {
			t.Errorf("%s element resource = %p, want %p", tc.list, own.ResourceType, tc.want)
		}
	}
}

// Exporting a resource gives the exported name a new ID in the validator, which
// must still find the imported resource when a compound is built over it.
func TestInstanceTypeAlias_ReexportedResource(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	prov := buildProvider(t, e, func(b *host.Builder) { b.AddResource("r", nil) })
	empty := buildProvider(t, e, func(b *host.Builder) {})
	inst, err := instantiateGuest(t, e, `(component
  (import "foo" (instance $i (export "r" (type (sub resource)))))
  (alias export $i "r" (type $r))
  (export $e "e" (type $r))
  (import "bar" (instance $b
    (alias outer 1 $e (type $e2))
    (export "x" (type $x (eq $e2)))
    (type $o (own $x))
    (type $l (list $o))
    (export "l" (type (eq $l)))
  ))
  (alias export $b "l" (type $l))
  (export "out" (type $l))
)`, wacogo.WithInstanceImport("foo", prov.Core()), wacogo.WithInstanceImport("bar", empty.Core()))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	res := prov.Core().ExportedType("r").(*core.TypeResource)
	want := core.TypeList{Elem: core.TypeOwn{ResourceType: res}}
	if got := inst.ExportedType("out"); !sameType(got, want) {
		t.Errorf("out = %#v, want %#v", got, want)
	}
}

// A `use`d resource is exported with an eq bound by the using interface. The
// provider of that interface may omit it, leaving the defining import as the
// only source, and it must still be possible to build compounds over it.
func TestInstanceTypeAlias_UsedResource(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	guest := `(component
  (import "poll" (instance $poll
    (export "pollable" (type (sub resource)))
  ))
  (alias export $poll "pollable" (type $p))
  (import "clock" (instance $clock
    (alias outer 1 $p (type $p'))
    (export "pollable" (type $pe (eq $p')))
    (type $o (own $pe))
    (type $l (list $o))
    (export "pollables" (type (eq $l)))
  ))
  (alias export $clock "pollable" (type $cp))
  (alias export $clock "pollables" (type $cl))
  (export "cp" (type $cp))
  (export "cl" (type $cl))
)`
	poll := buildProvider(t, e, func(b *host.Builder) { b.AddResource("pollable", nil) })
	clock := buildProvider(t, e, func(b *host.Builder) {})
	res := poll.Core().ExportedType("pollable")

	inst, err := instantiateGuest(t, e, guest,
		wacogo.WithInstanceImport("poll", poll.Core()),
		wacogo.WithInstanceImport("clock", clock.Core()))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	if got := inst.ExportedType("cp"); got != res {
		t.Errorf("cp = %#v, want %#v", got, res)
	}
	want := core.TypeList{Elem: core.TypeOwn{ResourceType: res.(*core.TypeResource)}}
	if got := inst.ExportedType("cl"); !sameType(got, want) {
		t.Errorf("cl = %#v, want %#v", got, want)
	}

	// A provider that does export the name has to agree on its identity.
	other := buildProvider(t, e, func(b *host.Builder) { b.AddResource("pollable", nil) })
	_, err = instantiateGuest(t, e, guest,
		wacogo.WithInstanceImport("poll", poll.Core()),
		wacogo.WithInstanceImport("clock", other.Core()))
	if err == nil || !strings.Contains(err.Error(), "mismatched resource types") {
		t.Errorf("err = %v, want mismatched resource types", err)
	}
}

// A declaration deep enough to be exponential if expanded must stay linear,
// both to load and to instantiate.
func TestInstanceTypeAlias_SharedTypesStayShared(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	var decl strings.Builder
	decl.WriteString("(type $t0 u8)\n")
	const depth = 15
	for i := 1; i <= depth; i++ {
		fmt.Fprintf(&decl, "(type $t%d (tuple $t%d $t%d))\n", i, i-1, i-1)
	}
	fmt.Fprintf(&decl, `(export "t" (type (eq $t%d)))`, depth)

	inst, err := instantiateGuest(t, e, `(component
  (import "foo" (instance $i `+decl.String()+`))
  (alias export $i "t" (type $t))
  (export "out" (type $t))
)`)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	got := inst.ExportedType("out")
	for i := 0; i < depth; i++ {
		tup, ok := got.(core.TypeTuple)
		if !ok || len(tup.Types) != 2 {
			t.Fatalf("depth %d: %#v", i, got)
		}
		got = tup.Types[0]
	}
	if !reflect.DeepEqual(got, core.TypeU8{}) {
		t.Errorf("leaf = %#v, want u8", got)
	}
}

// Encoders may place consecutive imports in one section, unlike the text
// format, so this is encoded by hand: an import following an eq-bound type
// import in the same section must not be dropped.
func TestInstanceTypeAlias_EqTypeImportSharesSection(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	bin := []byte{
		0x00, 0x61, 0x73, 0x6d, 0x0d, 0x00, 0x01, 0x00,
		// (type u32)
		0x07, 0x02, 0x01, 0x79,
		// (import "x" (type (eq 0))) (import "y" (type (eq 0)))
		0x0a, 0x0d, 0x02,
		0x00, 0x01, 'x', 0x03, 0x00, 0x00,
		0x00, 0x01, 'y', 0x03, 0x00, 0x00,
		// (export "out" (type 2))
		0x0b, 0x09, 0x01, 0x00, 0x03, 'o', 'u', 't', 0x03, 0x02, 0x00,
	}
	comp, err := e.LoadComponent(ctx, bytes.NewReader(bin))
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)
	if got := inst.ExportedType("out"); !reflect.DeepEqual(got, core.TypeU32{}) {
		t.Errorf("out = %#v, want u32", got)
	}
}

// The alias's type survives a sub-component re-exporting the imported instance.
func TestInstanceTypeAlias_ReexportedThroughSubComponent(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)
	prov := buildProvider(t, e, addNoopFunc)
	inst, err := instantiateGuest(t, e, `(component
  (import "foo" (instance $i (type $u u64) (export "t" (type (eq $u))) (export "f" (func))))
  (component $sub
    (import "foo" (instance $i (type $u u64) (export "t" (type (eq $u))) (export "f" (func))))
    (export "foo2" (instance $i))
  )
  (instance $x (instantiate $sub (with "foo" (instance $i))))
  (alias export $x "foo2" (instance $n))
  (alias export $n "t" (type $t))
  (export "out" (type $t))
)`, wacogo.WithInstanceImport("foo", prov.Core()))
	if err != nil {
		t.Fatalf("pass-through value: %v", err)
	}
	if got := inst.ExportedType("out"); !reflect.DeepEqual(got, core.TypeU64{}) {
		t.Errorf("out = %#v", got)
	}
}

// As above, for a compound type whose resource comes from the provider.
func TestInstanceTypeAlias_ReexportedCompoundOverResource(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	prov := buildProvider(t, e, func(b *host.Builder) { b.AddResource("res", nil) })
	inst, err := instantiateGuest(t, e, `(component
  (import "foo" (instance $i
    (export "res" (type $r (sub resource)))
    (type $o (own $r))
    (type $l (list $o))
    (export "owned" (type (eq $l)))
  ))
  (component $sub
    (import "foo" (instance $i
      (export "res" (type $r (sub resource)))
      (type $o (own $r))
      (type $l (list $o))
      (export "owned" (type (eq $l)))
    ))
    (export "foo2" (instance $i))
  )
  (instance $x (instantiate $sub (with "foo" (instance $i))))
  (alias export $x "foo2" (instance $n))
  (alias export $n "owned" (type $t))
  (export "out" (type $t))
)`, wacogo.WithInstanceImport("foo", prov.Core()))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	res := prov.Core().ExportedType("res").(*core.TypeResource)
	want := core.TypeList{Elem: core.TypeOwn{ResourceType: res}}
	if got := inst.ExportedType("out"); !sameType(got, want) {
		t.Errorf("out = %#v, want %#v", got, want)
	}
}

// An instance alias takes an index, so a later import's resources must be
// recorded against that import's real index.
func TestInstanceTypeAlias_ResourceImportAfterInstanceAlias(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	nested := buildProvider(t, e, func(b *host.Builder) { b.AddNestedInstance("nested") })
	prov := buildProvider(t, e, func(b *host.Builder) { b.AddResource("res", nil) })
	inst, err := instantiateGuest(t, e, `(component
  (import "a" (instance $a (export "nested" (instance))))
  (alias export $a "nested" (instance $n))
  (import "b" (instance $b
    (export "res" (type $r (sub resource)))
    (type $o (own $r))
    (type $l (list $o))
    (export "owned" (type (eq $l)))
  ))
  (alias export $b "owned" (type $t))
  (export "out" (type $t))
)`, wacogo.WithInstanceImport("a", nested.Core()), wacogo.WithInstanceImport("b", prov.Core()))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	res := prov.Core().ExportedType("res").(*core.TypeResource)
	want := core.TypeList{Elem: core.TypeOwn{ResourceType: res}}
	if got := inst.ExportedType("out"); !sameType(got, want) {
		t.Errorf("out = %#v, want %#v", got, want)
	}
}

// A nested instance needing nothing at runtime may be aliased even though nothing
// supplies it, and types read through it come from the declaration.
func TestInstanceTypeAlias_NestedInstanceWithoutProvider(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	inst, err := instantiateGuest(t, e, `(component
  (import "a" (instance $a
    (export "nested" (instance
      (type $u u64)
      (export "t" (type (eq $u)))
    ))
  ))
  (alias export $a "nested" (instance $n))
  (alias export $n "t" (type $t))
  (export "out" (type $t))
)`)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	if got := inst.ExportedType("out"); !reflect.DeepEqual(got, core.TypeU64{}) {
		t.Errorf("out = %#v, want u64", got)
	}
}

// A supplied instance may omit, at any depth, whatever needs no runtime
// definition. Expectations were confirmed against wasmtime 50.0.1.
func TestInstanceTypeAlias_NestedOmissions(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	prov := buildProvider(t, e, func(b *host.Builder) {
		b.AddResource("res", nil)
		b.AddNestedInstance("nested").AddFunction("f", &host.FuncType{}, func(context.Context, *core.CallContext, *host.ComponentInstance, []uint64) error { return nil })
	})

	for _, tc := range []struct {
		name    string
		guest   string
		wantErr string
	}{
		{name: "primitive in nested instance", guest: `(component (import "host" (instance
  (export "nested" (instance (type $u u64) (export "t" (type (eq $u))) (export "f" (func))))
)))`},
		{name: "record in nested instance", guest: `(component (import "host" (instance
  (export "nested" (instance (type $u (record (field "x" u32))) (export "t" (type (eq $u))) (export "f" (func))))
)))`},
		{name: "type-only nested instance", guest: `(component (import "host" (instance
  (export "absent" (instance (type $u u64) (export "t" (type (eq $u)))))
)))`},
		{name: "type-only instance nested in an omitted one", guest: `(component (import "host" (instance
  (export "absent" (instance (export "deeper" (instance (type $u u64) (export "t" (type (eq $u)))))))
)))`},
		{name: "eq of a resource in nested instance", guest: `(component (import "host" (instance
  (export "res" (type $r (sub resource)))
  (export "absent" (instance (alias outer 1 $r (type $r2)) (export "x" (type (eq $r2)))))
)))`},
		{
			name: "new resource in omitted nested instance",
			guest: `(component (import "host" (instance
  (export "absent" (instance (export "r" (type (sub resource)))))
)))`,
			wantErr: "`absent` was not found",
		},
		{
			name: "new resource in supplied nested instance",
			guest: `(component (import "host" (instance
  (export "nested" (instance (export "r" (type (sub resource)))))
)))`,
			wantErr: "`r` was not found",
		},
		{
			name: "func in omitted nested instance",
			guest: `(component (import "host" (instance
  (export "absent" (instance (export "f" (func))))
)))`,
			wantErr: "`absent` was not found",
		},
		{
			name: "func in supplied nested instance",
			guest: `(component (import "host" (instance
  (export "nested" (instance (export "g" (func))))
)))`,
			wantErr: "`g` was not found",
		},
		{
			name: "wrong type in supplied nested instance",
			guest: `(component (import "host" (instance
  (export "nested" (instance (export "f" (func (result u32)))))
)))`,
			wantErr: "type mismatch",
		},
		{
			name: "non-instance under a type-only instance's name",
			guest: `(component (import "host" (instance
  (export "res" (instance (type $u u64) (export "t" (type (eq $u)))))
)))`,
			wantErr: "type mismatch",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := instantiateGuest(t, e, tc.guest, wacogo.WithInstanceImport("host", prov.Core()))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Instantiate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}

	t.Run("alias through an omitted nested instance", func(t *testing.T) {
		inst, err := instantiateGuest(t, e, `(component
  (import "host" (instance $h
    (export "absent" (instance (type $u u64) (export "t" (type (eq $u)))))
  ))
  (alias export $h "absent" (instance $n))
  (alias export $n "t" (type $t))
  (export "out" (type $t))
)`, wacogo.WithInstanceImport("host", prov.Core()))
		if err != nil {
			t.Fatalf("Instantiate: %v", err)
		}
		if got := inst.ExportedType("out"); !reflect.DeepEqual(got, core.TypeU64{}) {
			t.Errorf("out = %#v, want u64", got)
		}
	})
}

// The leniency is for supplied instances only. Components linked to each other
// must provide every declared export.
func TestInstanceTypeAlias_ComponentLinkStaysStrict(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	_, err := e.LoadComponent(ctx, bytes.NewReader(watToBinary(t, `(component
  (component $p (instance $empty) (export "n" (instance $empty)))
  (instance $pi (instantiate $p))
  (component $g (import "foo" (instance
    (export "n" (instance (type $u u64) (export "t" (type (eq $u)))))
  )))
  (instance (instantiate $g (with "foo" (instance $pi))))
)`)))
	if err == nil || !strings.Contains(err.Error(), "missing expected export `t`") {
		t.Fatalf("err = %v, want missing expected export t", err)
	}
}

// A directly imported resource has no declaration to fall back on, so a missing
// one reads as a missing import.
func TestInstanceTypeAlias_DirectResourceWithoutProvider(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	_, err := instantiateGuest(t, e, `(component (import "foo" (type (sub resource))))`)
	if err == nil || !strings.Contains(err.Error(), `import "foo": was not found`) {
		t.Fatalf("err = %v, want import foo was not found", err)
	}
}

// A resource `use`d by an instance that also defines its own keeps its identity.
func TestInstanceTypeAlias_UsedResourceAlongsideOwnResource(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	guest := `(component
  (import "poll" (instance $poll
    (export "pollable" (type (sub resource)))
  ))
  (alias export $poll "pollable" (type $p))
  (import "http" (instance $http
    (export "fields" (type (sub resource)))
    (alias outer 1 $p (type $p'))
    (export "pollable" (type $pe (eq $p')))
    (type $o (own $pe))
    (type $l (list $o))
    (export "pollables" (type (eq $l)))
  ))
  (alias export $http "pollables" (type $cl))
  (export "cl" (type $cl))
)`
	poll := buildProvider(t, e, func(b *host.Builder) { b.AddResource("pollable", nil) })

	omits := buildProvider(t, e, func(b *host.Builder) { b.AddResource("fields", nil) })
	inst, err := instantiateGuest(t, e, guest,
		wacogo.WithInstanceImport("poll", poll.Core()),
		wacogo.WithInstanceImport("http", omits.Core()))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	res := poll.Core().ExportedType("pollable").(*core.TypeResource)
	want := core.TypeList{Elem: core.TypeOwn{ResourceType: res}}
	if got := inst.ExportedType("cl"); !sameType(got, want) {
		t.Errorf("cl = %#v, want %#v", got, want)
	}

	different := buildProvider(t, e, func(b *host.Builder) {
		b.AddResource("fields", nil)
		b.AddResource("pollable", nil)
	})
	_, err = instantiateGuest(t, e, guest,
		wacogo.WithInstanceImport("poll", poll.Core()),
		wacogo.WithInstanceImport("http", different.Core()))
	if err == nil || !strings.Contains(err.Error(), "mismatched resource types") {
		t.Errorf("err = %v, want mismatched resource types", err)
	}
}

// Two imports of one instance type get distinct resources, including when the
// type defines them only inside a nested instance.
func TestInstanceTypeAlias_NestedResourcesAreDistinctPerImport(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	provider := func() *host.ComponentInstance {
		return buildProvider(t, e, func(b *host.Builder) { b.AddNestedInstance("n").AddResource("r", nil) })
	}
	a, b := provider(), provider()
	inst, err := instantiateGuest(t, e, `(component
  (type $t (instance (export "n" (instance (export "r" (type (sub resource)))))))
  (import "a" (instance $a (type $t)))
  (import "b" (instance $b (type $t)))
  (alias export $a "n" (instance $an))
  (alias export $b "n" (instance $bn))
  (alias export $an "r" (type $ar))
  (alias export $bn "r" (type $br))
  (export "ar" (type $ar))
  (export "br" (type $br))
)`, wacogo.WithInstanceImport("a", a.Core()), wacogo.WithInstanceImport("b", b.Core()))
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	if got, want := inst.ExportedType("ar"), a.Core().ExportedInstance("n").ExportedType("r"); got != want {
		t.Errorf("ar = %#v, want %#v", got, want)
	}
	if got, want := inst.ExportedType("br"), b.Core().ExportedInstance("n").ExportedType("r"); got != want {
		t.Errorf("br = %#v, want %#v", got, want)
	}
}

// A compound type aliased into another import keeps its identity, whether or not
// that import defines a resource of its own.
func TestInstanceTypeAlias_CompoundAliasedIntoImport(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	for name, own := range map[string]string{
		"no own resource": "",
		"own resource":    `(export "other" (type (sub resource)))`,
	} {
		t.Run(name, func(t *testing.T) {
			http := buildProvider(t, e, func(b *host.Builder) { b.AddResource("fields", nil) })
			consumer := buildProvider(t, e, func(b *host.Builder) {
				if own != "" {
					b.AddResource("other", nil)
				}
			})
			inst, err := instantiateGuest(t, e, `(component
  (import "http" (instance $http
    (export "fields" (type $f (sub resource)))
    (type $o (own $f))
    (type $r (record (field "h" $o)))
    (export "bundle" (type (eq $r)))
  ))
  (alias export $http "bundle" (type $b))
  (import "consumer" (instance $consumer
    `+own+`
    (alias outer 1 $b (type $b'))
    (export "bundle" (type $be (eq $b')))
  ))
  (alias export $consumer "bundle" (type $cb))
  (export "cb" (type $cb))
)`, wacogo.WithInstanceImport("http", http.Core()), wacogo.WithInstanceImport("consumer", consumer.Core()))
			if err != nil {
				t.Fatalf("Instantiate: %v", err)
			}
			res := http.Core().ExportedType("fields").(*core.TypeResource)
			want := core.TypeRecord{Fields: []core.FieldType{{Name: "h", Type: core.TypeOwn{ResourceType: res}}}}
			if got := inst.ExportedType("cb"); !sameType(got, want) {
				t.Errorf("cb = %#v, want %#v", got, want)
			}
		})
	}
}

// A resource re-exported through a nested instance is still the resource of the
// instance that defines it.
func TestInstanceTypeAlias_ResourceReexportedThroughNestedInstance(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	addRes := func(b *host.Builder) { b.AddResource("r", nil) }
	a := buildProvider(t, e, addRes).Core()
	other := buildProvider(t, e, addRes).Core()
	nestedA := buildProvider(t, e, func(b *host.Builder) {
		b.AddNestedInstance("deep").AddResource("r", nil)
	}).Core()

	const importA = `(import "a" (instance $a (export "r" (type (sub resource)))))
  (alias export $a "r" (type $r))`

	reexport := func(t *testing.T, wat string, opts ...wacogo.InstantiateOption) *wacogo.ComponentInstance {
		t.Helper()
		inst, err := instantiateGuest(t, e, wat, opts...)
		if err != nil {
			t.Fatalf("re-exporting instance: %v", err)
		}
		return inst
	}

	throughNested := reexport(t, `(component
  `+importA+`
  (instance $n (export "r" (type $r)))
  (export "n" (instance $n))
)`, wacogo.WithInstanceImport("a", a))

	throughTwoLevels := reexport(t, `(component
  `+importA+`
  (instance $i (export "r" (type $r)))
  (instance $n (export "i" (instance $i)))
  (export "n" (instance $n))
)`, wacogo.WithInstanceImport("a", a))

	throughSubcomponent := reexport(t, `(component
  `+importA+`
  (component $s
    (import "r" (type $r (sub resource)))
    (instance $i (export "r" (type $r)))
    (export "n" (instance $i))
  )
  (instance $x (instantiate $s (with "r" (type $r))))
  (alias export $x "n" (instance $n))
  (export "n" (instance $n))
)`, wacogo.WithInstanceImport("a", a))

	withFunc := reexport(t, `(component
  `+importA+`
  (core module $m (func (export "f") (param i32)))
  (core instance $mi (instantiate $m))
  (type $o (own $r))
  (func $f (param "h" $o) (canon lift (core func $mi "f")))
  (instance $n (export "r" (type $r)) (export "f" (func $f)))
  (export "n" (instance $n))
)`, wacogo.WithInstanceImport("a", a))

	fromNestedImport := reexport(t, `(component
  (import "a" (instance $a (export "deep" (instance (export "r" (type (sub resource)))))))
  (alias export $a "deep" (instance $d))
  (alias export $d "r" (type $r))
  (instance $n (export "r" (type $r)))
  (export "n" (instance $n))
  (export "top" (type $r))
)`, wacogo.WithInstanceImport("a", nestedA))

	chained := reexport(t, `(component
  (import "p" (instance $p (export "n" (instance $pn (export "r" (type (sub resource)))))))
  (alias export $p "n" (instance $pn))
  (alias export $pn "r" (type $r))
  (instance $n (export "r" (type $r)))
  (export "n" (instance $n))
)`, wacogo.WithInstanceImport("p", throughNested))

	// The nested instance carries the type at runtime, as the same resource.
	if got, want := throughNested.ExportedInstance("n").ExportedType("r"), a.ExportedType("r"); got == nil || !sameType(got, want) {
		t.Errorf("nested instance's resource = %v, want %v", got, want)
	}

	const wantsNestedR = `(component
  ` + importA + `
  (import "p" (instance (export "n" (instance
    (alias outer 2 $r (type $r2))
    (export "r" (type (eq $r2)))
  ))))
)`

	for _, tc := range []struct {
		name    string
		guest   string
		a       *wacogo.ComponentInstance
		p       *wacogo.ComponentInstance
		wantErr string
	}{
		{name: "same resource", guest: wantsNestedR, a: a, p: throughNested},
		{name: "different resource", guest: wantsNestedR, a: other, p: throughNested, wantErr: "mismatched resource types"},
		{name: "through a subcomponent", guest: wantsNestedR, a: a, p: throughSubcomponent},
		{name: "different resource through a subcomponent", guest: wantsNestedR, a: other, p: throughSubcomponent, wantErr: "mismatched resource types"},
		{name: "chained through a nested import", guest: wantsNestedR, a: a, p: chained},
		{name: "different resource chained", guest: wantsNestedR, a: other, p: chained, wantErr: "mismatched resource types"},
		{
			name: "two levels deep",
			guest: `(component
  ` + importA + `
  (import "p" (instance (export "n" (instance (export "i" (instance
    (alias outer 3 $r (type $r2))
    (export "r" (type (eq $r2)))
  ))))))
)`,
			a: a, p: throughTwoLevels,
		},
		{
			name: "two levels deep, different resource",
			guest: `(component
  ` + importA + `
  (import "p" (instance (export "n" (instance (export "i" (instance
    (alias outer 3 $r (type $r2))
    (export "r" (type (eq $r2)))
  ))))))
)`,
			a: other, p: throughTwoLevels, wantErr: "mismatched resource types",
		},
		{
			name: "function over the resource",
			guest: `(component
  ` + importA + `
  (import "p" (instance (export "n" (instance
    (alias outer 2 $r (type $r2))
    (export "r" (type (eq $r2)))
    (type $o (own $r2))
    (export "f" (func (param "h" $o)))
  ))))
)`,
			a: a, p: withFunc,
		},
		{
			name: "function over the resource, different resource",
			guest: `(component
  ` + importA + `
  (import "p" (instance (export "n" (instance
    (alias outer 2 $r (type $r2))
    (export "r" (type (eq $r2)))
    (type $o (own $r2))
    (export "f" (func (param "h" $o)))
  ))))
)`,
			a: other, p: withFunc, wantErr: "mismatched resource types",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := instantiateGuest(t, e, tc.guest, wacogo.WithInstanceImport("a", tc.a), wacogo.WithInstanceImport("p", tc.p))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Instantiate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Instantiate error = %v, want one containing %q", err, tc.wantErr)
			}
		})
	}

	// The resource comes from inside a nested instance of the import.
	t.Run("from a nested import", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			deep    *wacogo.ComponentInstance
			wantErr string
		}{
			{name: "same", deep: nestedA},
			{name: "different", deep: buildProvider(t, e, func(b *host.Builder) {
				b.AddNestedInstance("deep").AddResource("r", nil)
			}).Core(), wantErr: "mismatched resource types"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := instantiateGuest(t, e, `(component
  (import "a" (instance $a (export "deep" (instance (export "r" (type (sub resource)))))))
  (alias export $a "deep" (instance $d))
  (alias export $d "r" (type $r))
  (import "p" (instance
    (export "n" (instance (alias outer 2 $r (type $r2)) (export "r" (type (eq $r2)))))
    (alias outer 1 $r (type $r3))
    (export "top" (type (eq $r3)))
  ))
)`, wacogo.WithInstanceImport("a", tc.deep), wacogo.WithInstanceImport("p", fromNestedImport))
				if tc.wantErr == "" {
					if err != nil {
						t.Fatalf("Instantiate: %v", err)
					}
					return
				}
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Instantiate error = %v, want one containing %q", err, tc.wantErr)
				}
			})
		}
	})
}
