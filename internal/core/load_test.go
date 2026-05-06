package core

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// buildComponentBytes compiles WAT text to a binary component using wasm-tools.
func buildComponentBytes(t *testing.T, wat string) []byte {
	t.Helper()
	cmd := exec.Command("wasm-tools", "parse", "-")
	cmd.Stdin = strings.NewReader(wat)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("wasm-tools parse: %s\n%s", err, ee.Stderr)
		}
		t.Fatalf("wasm-tools parse: %v", err)
	}
	return out
}

func TestLoadComponent_SimpleAdd(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	f, err := os.Open("testdata/simple-add.wasm")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	comp, err := engine.LoadComponent(ctx, f)
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}

	// Verify at least 1 compiled module.
	if len(comp.compiledModules) == 0 {
		t.Fatal("expected at least 1 compiled module")
	}

	// Verify plan has steps.
	if len(comp.plan) == 0 {
		t.Fatal("expected plan to have steps")
	}

	// Verify exports include "add".
	found := false
	for _, e := range comp.Exports() {
		if e.Name == "add" {
			found = true
			if e.Kind != SortFunc {
				t.Fatalf("export 'add' kind: got %d, want SortFunc (%d)", e.Kind, SortFunc)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected export 'add', got exports: %+v", comp.Exports())
	}

	// Verify plan step types for simple-add component:
	// Expected: 1 planInstantiateModule, 3 planAlias (memory, realloc, add), 1 planLift
	var (
		instantiates int
		aliases      int
		lifts        int
	)
	for _, step := range comp.plan {
		switch step.(type) {
		case *planInstantiateModule:
			instantiates++
		case *planAlias:
			aliases++
		case *planLift:
			lifts++
		}
	}
	if instantiates != 1 {
		t.Errorf("planInstantiateModule count: got %d, want 1", instantiates)
	}
	if aliases != 3 {
		t.Errorf("planAlias count: got %d, want 3", aliases)
	}
	if lifts != 1 {
		t.Errorf("planLift count: got %d, want 1", lifts)
	}

	// Verify no imports (simple-add has none).
	if len(comp.Imports()) != 0 {
		t.Errorf("expected 0 imports, got %d", len(comp.Imports()))
	}
}

func TestLoadedComponentHasWasmparserType(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)

	f, err := os.Open("testdata/simple-add.wasm")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	comp, err := e.LoadComponent(ctx, f)
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}
	if comp.wpType == nil {
		t.Fatal("expected top-level ComponentType handle to be populated")
	}
}

func TestLoadedComponentInlineModulesCarryHandle(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx)
	defer e.Close(ctx)

	bin := buildComponentBytes(t, `(component
  (core module (func))
)`)
	comp, err := e.LoadComponent(ctx, bytes.NewReader(bin))
	if err != nil {
		t.Fatal(err)
	}
	if len(comp.compiledModules) == 0 {
		t.Fatal("expected at least one compiled module")
	}
	if comp.compiledModules[0].wpModuleType == nil {
		t.Fatal("expected wpModuleType on inline module")
	}
}
