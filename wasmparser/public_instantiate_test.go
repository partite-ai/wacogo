package wasmparser

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestNewInstanceReturnsDistinctHandles(t *testing.T) {
	src := `(component
  (type $r (resource (rep i32)))
  (export "r" (type $r))
)`
	bin := wat2wasm(t, src)
	vp := NewValidatingParser(bytes.NewReader(bin), DefaultFeatures())
	for {
		_, err := vp.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	ct := vp.TopLevelComponentType()
	if ct == nil {
		t.Fatal("no top-level ComponentType")
	}

	inst1 := ct.NewInstance()
	inst2 := ct.NewInstance()

	if inst1 == inst2 {
		t.Fatal("want distinct *InstanceType handles from separate NewInstance calls")
	}
	if inst1.source != ct || inst2.source != ct {
		t.Fatal("each handle should reference the same source ComponentType")
	}
}

func TestNewInstanceIsMemoryNeutral(t *testing.T) {
	src := `(component
  (type $r (resource (rep i32)))
  (export "r" (type $r))
)`
	ct := loadComponent(t, src)
	before := arenaShape(ct.arena)
	for range 1000 {
		_ = ct.NewInstance()
	}
	after := arenaShape(ct.arena)
	if before != after {
		t.Fatalf("arena mutated across 1000 NewInstance calls: before=%+v after=%+v", before, after)
	}
}

func TestCheckInstantiationIsMemoryNeutral(t *testing.T) {
	consSrc := `(component
  (import "I1" (instance $i1 (export "r" (type (sub resource)))))
  (alias export $i1 "r" (type $r))
  (import "I2" (instance $i2 (export "r" (type (eq $r)))))
)`
	provSrc := `(component
  (type $r (resource (rep i32)))
  (export "r" (type $r))
)`
	prov := loadComponent(t, provSrc)
	cons := loadComponent(t, consSrc)

	beforeProv := arenaShape(prov.arena)
	beforeCons := arenaShape(cons.arena)
	for range 100 {
		shared := prov.NewInstance()
		if err := cons.CheckInstantiation(map[string]any{
			"I1": shared,
			"I2": shared,
		}); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		i1 := prov.NewInstance()
		i2 := prov.NewInstance()
		if err := cons.CheckInstantiation(map[string]any{
			"I1": i1,
			"I2": i2,
		}); err == nil {
			t.Fatal("expected mismatch, got nil")
		}
	}
	if got := arenaShape(prov.arena); got != beforeProv {
		t.Fatalf("provider arena mutated across 100 checks: before=%+v after=%+v", beforeProv, got)
	}
	if got := arenaShape(cons.arena); got != beforeCons {
		t.Fatalf("consumer arena mutated across 100 checks: before=%+v after=%+v", beforeCons, got)
	}
}

// arenaShape snapshots an arena's visible extents for equality comparison
// in memory-neutrality tests.
type arenaSize struct {
	coreFunc, coreMod, coreInst, defined, funcs, inst, comp int
	nextRes                                                 ResourceID
}

func arenaShape(a *TypeArena) arenaSize {
	return arenaSize{
		coreFunc: len(a.CoreFuncTypes),
		coreMod:  len(a.CoreModuleTypes),
		coreInst: len(a.CoreInstanceTypes),
		defined:  len(a.DefinedTypes),
		funcs:    len(a.FuncTypes),
		inst:     len(a.InstanceTypes),
		comp:     len(a.ComponentTypes),
		nextRes:  a.nextResourceID,
	}
}

// loadComponent parses the given WAT source with a fresh ValidatingParser
// and returns the top-level ComponentType handle. Each call allocates its
// own *TypeArena.
func loadComponent(t *testing.T, src string) *ComponentType {
	t.Helper()
	bin := wat2wasm(t, src)
	vp := NewValidatingParser(bytes.NewReader(bin), DefaultFeatures())
	for {
		_, err := vp.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
	}
	ct := vp.TopLevelComponentType()
	if ct == nil {
		t.Fatal("no top-level ComponentType")
	}
	return ct
}

func TestCheckInstantiationAcceptsSharedResource(t *testing.T) {
	consSrc := `(component
  (import "I1" (instance $i1 (export "r" (type (sub resource)))))
  (alias export $i1 "r" (type $r))
  (import "I2" (instance $i2 (export "r" (type (eq $r)))))
)`
	provSrc := `(component
  (type $r (resource (rep i32)))
  (export "r" (type $r))
)`
	prov := loadComponent(t,provSrc)
	cons := loadComponent(t,consSrc)

	shared := prov.NewInstance()

	err := cons.CheckInstantiation(map[string]any{
		"I1": shared,
		"I2": shared,
	})
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestCheckInstantiationConcurrentlySafe(t *testing.T) {
	// Exercise under `go test -race`: many goroutines running
	// NewInstance + CheckInstantiation concurrently against the same
	// *Validator must not race, since neither path mutates shared state
	// after load completes.
	consSrc := `(component
  (import "I1" (instance $i1 (export "r" (type (sub resource)))))
  (alias export $i1 "r" (type $r))
  (import "I2" (instance $i2 (export "r" (type (eq $r)))))
)`
	provSrc := `(component
  (type $r (resource (rep i32)))
  (export "r" (type $r))
)`
	prov := loadComponent(t,provSrc)
	cons := loadComponent(t,consSrc)

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 50 {
				shared := prov.NewInstance()
				_ = cons.CheckInstantiation(map[string]any{
					"I1": shared, "I2": shared,
				})
				i1 := prov.NewInstance()
				i2 := prov.NewInstance()
				_ = cons.CheckInstantiation(map[string]any{
					"I1": i1, "I2": i2,
				})
			}
		})
	}
	wg.Wait()
}

func TestCheckInstantiationRejectsMismatchedResources(t *testing.T) {
	consSrc := `(component
  (import "I1" (instance $i1 (export "r" (type (sub resource)))))
  (alias export $i1 "r" (type $r))
  (import "I2" (instance $i2 (export "r" (type (eq $r)))))
)`
	provSrc := `(component
  (type $r (resource (rep i32)))
  (export "r" (type $r))
)`
	prov := loadComponent(t,provSrc)
	cons := loadComponent(t,consSrc)

	i1 := prov.NewInstance()
	i2 := prov.NewInstance()

	err := cons.CheckInstantiation(map[string]any{
		"I1": i1,
		"I2": i2,
	})
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "mismatched resource types") {
		t.Fatalf("expected 'mismatched resource types', got %q", err.Error())
	}
}

// exportHandle extracts a typed handle from the top-level component's
// named export. Returns a *FuncType, *ModuleType, *ComponentType, or
// *InstanceType depending on the export kind — test-only, bypasses the
// validated-payload plumbing that real callers use.
func exportHandle(t *testing.T, ct *ComponentType, name string) any {
	t.Helper()
	arena := ct.arena
	internal := arena.ComponentTypes[ct.id]
	et, ok := internal.Exports[name]
	if !ok {
		t.Fatalf("no export %q", name)
	}
	switch et.Kind {
	case EntityFunc:
		return &FuncType{arena: arena, id: et.FuncID}
	case EntityModule:
		return &ModuleType{arena: arena, id: et.ModuleID}
	case EntityComponent:
		return &ComponentType{arena: arena, id: et.CompID}
	case EntityInstance:
		return &InstanceType{source: ct}
	default:
		t.Fatalf("unsupported export kind %v", et.Kind)
		return nil
	}
}

func TestCheckInstantiationAcceptsFuncArg(t *testing.T) {
	consSrc := `(component
  (type (func (param "x" u32) (result u32)))
  (import "f" (func (type 0)))
)`
	provSrc := `(component
  (core module $m
    (func (export "f") (param i32) (result i32) local.get 0)
  )
  (core instance $i (instantiate $m))
  (func (export "f") (param "x" u32) (result u32)
    (canon lift (core func $i "f")))
)`
	prov := loadComponent(t,provSrc)
	cons := loadComponent(t,consSrc)

	fn := exportHandle(t, prov, "f")
	if err := cons.CheckInstantiation(map[string]any{"f": fn}); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestCheckInstantiationRejectsFuncSignatureMismatch(t *testing.T) {
	consSrc := `(component
  (type (func (param "x" u32) (result u32)))
  (import "f" (func (type 0)))
)`
	provSrc := `(component
  (core module $m
    (func (export "f") (param i32 i32) (result i32) local.get 0)
  )
  (core instance $i (instantiate $m))
  (func (export "f") (param "x" u32) (param "y" u32) (result u32)
    (canon lift (core func $i "f")))
)`
	prov := loadComponent(t,provSrc)
	cons := loadComponent(t,consSrc)
	fn := exportHandle(t, prov, "f")
	err := cons.CheckInstantiation(map[string]any{"f": fn})
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
}

func TestCheckInstantiationRejectsWrongKind(t *testing.T) {
	consSrc := `(component
  (type (func (param "x" u32) (result u32)))
  (import "f" (func (type 0)))
)`
	provSrc := `(component
  (type $r (resource (rep i32)))
  (export "r" (type $r))
)`
	prov := loadComponent(t,provSrc)
	cons := loadComponent(t,consSrc)
	err := cons.CheckInstantiation(map[string]any{"f": prov.NewInstance()})
	if err == nil {
		t.Fatal("expected kind-mismatch, got nil")
	}
}

func TestCheckInstantiationAcceptsModuleArg(t *testing.T) {
	consSrc := `(component
  (core type $mt (module (export "f" (func))))
  (import "m" (core module (type $mt)))
)`
	provSrc := `(component
  (core module $m
    (func (export "f"))
  )
  (export "m" (core module $m))
)`
	prov := loadComponent(t,provSrc)
	cons := loadComponent(t,consSrc)
	mt := exportHandle(t, prov, "m")
	if err := cons.CheckInstantiation(map[string]any{"m": mt}); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestCheckInstantiationAcceptsComponentArg(t *testing.T) {
	consSrc := `(component
  (type $ct (component (export "x" (type (sub resource)))))
  (import "c" (component (type $ct)))
)`
	provSrc := `(component
  (component $c
    (type $r (resource (rep i32)))
    (export "x" (type $r))
  )
  (export "c" (component $c))
)`
	prov := loadComponent(t,provSrc)
	cons := loadComponent(t,consSrc)
	ct := exportHandle(t, prov, "c")
	if err := cons.CheckInstantiation(map[string]any{"c": ct}); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestCheckInstantiationSkipsMissingArg(t *testing.T) {
	consSrc := `(component
  (type (func))
  (import "f" (func (type 0)))
)`
	cons := loadComponent(t,consSrc)
	if err := cons.CheckInstantiation(map[string]any{}); err != nil {
		t.Fatalf("want nil (missing args are skipped), got %v", err)
	}
}

func TestCheckInstantiationRejectsModuleSignatureMismatch(t *testing.T) {
	// Consumer expects a module exporting a func named "f".
	consSrc := `(component
  (core type $mt (module (export "f" (func))))
  (import "m" (core module (type $mt)))
)`
	// Provider's module exports "g" instead of "f".
	provSrc := `(component
  (core module $m (func (export "g")))
  (export "m" (core module $m))
)`
	prov := loadComponent(t,provSrc)
	cons := loadComponent(t,consSrc)
	mt := exportHandle(t, prov, "m")
	err := cons.CheckInstantiation(map[string]any{"m": mt})
	if err == nil {
		t.Fatal("expected export-missing error, got nil")
	}
}

func TestCheckInstantiationRejectsModuleWrongKind(t *testing.T) {
	consSrc := `(component
  (core type $mt (module (export "f" (func))))
  (import "m" (core module (type $mt)))
)`
	provSrc := `(component
  (type $r (resource (rep i32)))
  (export "r" (type $r))
)`
	prov := loadComponent(t,provSrc)
	cons := loadComponent(t,consSrc)
	// Supply an *InstanceType for a core-module import.
	err := cons.CheckInstantiation(map[string]any{"m": prov.NewInstance()})
	if err == nil {
		t.Fatal("expected kind-mismatch error, got nil")
	}
}
