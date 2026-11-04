package core

import (
	"bytes"
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
)

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

// TestLoadComponent_SuccessKeepsModules checks that a loaded component is left
// with what it compiled.
func TestLoadComponent_SuccessKeepsModules(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)
	counter := countCompiles(engine)

	if _, err := engine.LoadComponent(ctx, bytes.NewReader(parseLinkWat(t, `(component (core module))`))); err != nil {
		t.Fatal(err)
	}
	if counter.compiled != 1 || counter.closed != 0 {
		t.Fatalf("compiled %d, closed %d", counter.compiled, counter.closed)
	}
}
