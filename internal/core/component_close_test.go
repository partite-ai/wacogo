package core

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
)

// TestInstantiate_NilImport checks a nil import is an error rather than a crash. A
// failed lookup such as ExportedInstance("typo") returns nil, so it's easy to pass.
func TestInstantiate_NilImport(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	for name, c := range map[string]struct {
		wat  string
		opt  InstantiateOption
		want string
	}{
		"instance":  {`(component (import "x" (instance)))`, WithInstanceImport("x", nil), "no instance supplied"},
		"func":      {`(component (import "x" (func)))`, WithFuncImport("x", nil), "no func supplied"},
		"module":    {`(component (import "x" (core module)))`, WithModuleImport("x", nil), "no module supplied"},
		"component": {`(component (import "x" (instance)))`, WithComponentImport("x", nil), "no component supplied"},
		"resource":  {`(component (import "x" (type (sub resource))))`, WithResourceTypeImport("x", nil, "r"), "no instance supplied"},
	} {
		t.Run(name, func(t *testing.T) {
			comp, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, c.wat)))
			if err != nil {
				t.Fatal(err)
			}
			defer comp.Close(ctx)
			_, err = comp.Instantiate(ctx, c.opt)
			if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), `import "x"`) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

// compileCounter counts the modules compiled through a runtime and those closed,
// which is otherwise only visible as memory that's never returned.
type compileCounter struct {
	wazero.Runtime
	compiled, closed int
}

type countedModule struct {
	wazero.CompiledModule
	c *compileCounter
}

func (c *compileCounter) CompileModule(ctx context.Context, binary []byte) (wazero.CompiledModule, error) {
	m, err := c.Runtime.CompileModule(ctx, binary)
	if err != nil {
		return nil, err
	}
	c.compiled++
	return countedModule{m, c}, nil
}

func (m countedModule) Close(ctx context.Context) error {
	m.c.closed++
	return m.CompiledModule.Close(ctx)
}

func countCompiles(e *Engine) *compileCounter {
	c := &compileCounter{Runtime: e.runtime}
	e.runtime = c
	return c
}

// TestLoadComponent_FailureClosesModules covers each way a load can fail after
// compiling, whichever component the modules are in.
func TestLoadComponent_FailureClosesModules(t *testing.T) {
	const invalid = `(type (func (param "a" u32) (param "a" u32)))`

	for name, c := range map[string]struct {
		wat      string
		limits   ParseLimits
		compiled int
	}{
		"invalid after a module": {
			wat:      `(component (core module) ` + invalid + `)`,
			compiled: 1,
		},
		"invalid in a nested component": {
			wat:      `(component (core module) (component (core module) ` + invalid + `))`,
			compiled: 2,
		},
		"after a nested component has ended": {
			wat:      `(component (component (core module)) (core module) ` + invalid + `)`,
			compiled: 2,
		},
		"instance limit": {
			wat: `(component (component $c (core module $m)
  (core instance (instantiate $m)) (core instance (instantiate $m)))
  (instance (instantiate $c)))`,
			limits:   ParseLimits{CoreInstances: 1},
			compiled: 1,
		},
		"root imports a component": {
			wat:      `(component (core module) (import "c" (component)))`,
			compiled: 1,
		},
		"root exports a component": {
			wat:      `(component (core module) (component $c) (export "c" (component $c)))`,
			compiled: 1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			engine := NewEngine(ctx, WithParseLimits(c.limits))
			defer engine.Close(ctx)
			counter := countCompiles(engine)

			if _, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, c.wat))); err == nil {
				t.Fatal("expected the load to fail")
			}
			if counter.compiled != c.compiled {
				t.Fatalf("compiled %d modules, want %d", counter.compiled, c.compiled)
			}
			if counter.closed != c.compiled {
				t.Fatalf("closed %d of %d modules", counter.closed, c.compiled)
			}
		})
	}
}

// TestComponentClose_ReleasesModules checks that a loaded component keeps what it
// compiled, nested components included, until Close releases each of them once,
// however many index spaces an outer alias put a module in.
func TestComponentClose_ReleasesModules(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)
	counter := countCompiles(engine)

	comp, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, `(component
  (core module $m)
  (component
    (alias outer 1 $m (core module))
    (core module)))`)))
	if err != nil {
		t.Fatal(err)
	}
	if counter.compiled != 2 || counter.closed != 0 {
		t.Fatalf("after load: compiled %d, closed %d", counter.compiled, counter.closed)
	}
	for range 2 {
		if err := comp.Close(ctx); err != nil {
			t.Fatal(err)
		}
		if counter.closed != 2 {
			t.Fatalf("after Close: closed %d of 2", counter.closed)
		}
	}
	if _, err := comp.Instantiate(ctx); !errors.Is(err, ErrComponentClosed) {
		t.Fatalf("Instantiate after Close: got %v", err)
	}
}

const closeTestAnswer = `(component
  (core module $m (func (export "f") (result i32) i32.const 7))
  (core instance $i (instantiate $m))
  (func (export "f") (result u32) (canon lift (core func $i "f"))))`

func callAnswer(t *testing.T, ctx context.Context, inst *ComponentInstance) {
	t.Helper()
	got, err := inst.ExportedFunc("f").Call(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != ValU32(7) {
		t.Fatalf("got %v", got)
	}
}

// TestComponentClose_InstancesKeepRunning checks an instance outlives its
// Component, including once the collector has had a chance to run.
func TestComponentClose_InstancesKeepRunning(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	comp, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, closeTestAnswer)))
	if err != nil {
		t.Fatal(err)
	}
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := comp.Close(ctx); err != nil {
		t.Fatal(err)
	}
	comp = nil
	runtime.GC()
	runtime.GC()
	callAnswer(t, ctx, inst)
	if err := inst.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestComponentClose_SharedCodeSurvives checks that closing a component leaves
// another compiled from the same bytes working, as they share compiled code.
func TestComponentClose_SharedCodeSurvives(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)
	bin := parseLinkWat(t, closeTestAnswer)

	first, err := engine.LoadComponent(ctx, bytes.NewReader(bin))
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.LoadComponent(ctx, bytes.NewReader(bin))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	inst, err := second.Instantiate(ctx)
	if err != nil {
		t.Fatalf("instantiating the other component: %v", err)
	}
	callAnswer(t, ctx, inst)

	// A failed load of the same module must not take the code away either.
	if _, err := engine.LoadComponent(ctx, bytes.NewReader(append(bytes.Clone(bin), 7, 2, 1, 0))); err == nil {
		t.Fatal("expected the load to fail")
	}
	inst2, err := second.Instantiate(ctx)
	if err != nil {
		t.Fatalf("instantiating after a failed load: %v", err)
	}
	callAnswer(t, ctx, inst2)
}

// TestComponentClose_ExportedModule checks that a module taken from an instance
// fails cleanly once its Component is closed.
func TestComponentClose_ExportedModule(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	provider, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, `(component
  (core module $m)
  (export "m" (core module $m)))`)))
	if err != nil {
		t.Fatal(err)
	}
	inst, err := provider.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mod := inst.ExportedModule("m")
	if mod == nil {
		t.Fatal("no exported module")
	}
	consumer, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, `(component
  (import "m" (core module $m))
  (core instance (instantiate $m)))`)))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := consumer.Instantiate(ctx, WithModuleImport("m", mod)); err != nil {
		t.Fatalf("before Close: %v", err)
	}
	if err := provider.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := consumer.Instantiate(ctx, WithModuleImport("m", mod)); err == nil {
		t.Fatal("instantiated a module whose Component is closed")
	}
}
