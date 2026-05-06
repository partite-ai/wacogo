package wasm

import (
	"context"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
)

// TestRewriteBlankImportNames_NoBlank verifies that a module with no blank import
// module names is returned unchanged with an empty synthetic name.
func TestRewriteBlankImportNames_NoBlank(t *testing.T) {
	var b ModuleBuilder
	b.AddImportFunc("env", "foo", FuncSig{Params: nil, Results: nil})
	original := b.Encode()

	rewritten, synthetic, err := RewriteBlankImportNames(original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if synthetic != "" {
		t.Fatalf("expected empty synthetic name, got %q", synthetic)
	}
	if string(rewritten) != string(original) {
		t.Fatal("expected original bytes to be returned unchanged")
	}
}

// TestRewriteBlankImportNames_WithBlank verifies that a module with a blank import
// module name is rewritten, the synthetic name is non-empty, and the rewritten
// module compiles with wazero.
func TestRewriteBlankImportNames_WithBlank(t *testing.T) {
	var b ModuleBuilder
	b.AddImportFunc("", "foo", FuncSig{Params: nil, Results: nil})
	original := b.Encode()

	rewritten, synthetic, err := RewriteBlankImportNames(original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if synthetic == "" {
		t.Fatal("expected non-empty synthetic name")
	}

	// Rewritten module should compile with wazero
	r := wazero.NewRuntime(context.Background())
	defer r.Close(context.Background())
	cm, err := r.CompileModule(context.Background(), rewritten)
	if err != nil {
		t.Fatalf("CompileModule on rewritten module failed: %v", err)
	}
	defer cm.Close(context.Background())

	// Verify the import's module name was replaced
	imports := cm.ImportedFunctions()
	if len(imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(imports))
	}
	mod, name, _ := imports[0].Import()
	if mod != synthetic {
		t.Fatalf("expected module name %q, got %q", synthetic, mod)
	}
	if name != "foo" {
		t.Fatalf("expected field name %q, got %q", "foo", name)
	}
}

// TestRewriteBlankImportNames_CollisionAvoidance verifies that when "__wacogo_0"
// already exists as an import module name, the synthetic name chosen is different.
func TestRewriteBlankImportNames_CollisionAvoidance(t *testing.T) {
	var b ModuleBuilder
	b.AddImportFunc("", "foo", FuncSig{Params: nil, Results: nil})
	b.AddImportFunc("__wacogo_0", "bar", FuncSig{Params: nil, Results: nil})
	original := b.Encode()

	_, synthetic, err := RewriteBlankImportNames(original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if synthetic == "" {
		t.Fatal("expected non-empty synthetic name")
	}
	if synthetic == "__wacogo_0" {
		t.Fatalf("synthetic name should not be __wacogo_0 due to collision avoidance")
	}
	if !strings.HasPrefix(synthetic, "__wacogo_") {
		t.Fatalf("expected synthetic name to start with __wacogo_, got %q", synthetic)
	}
}

// TestRewriteBlankImportNames_NoImportSection verifies that a module with no
// import section is returned unchanged.
func TestRewriteBlankImportNames_NoImportSection(t *testing.T) {
	var b ModuleBuilder
	// No imports, just a memory
	b.AddMemory(1, 0)
	original := b.Encode()

	rewritten, synthetic, err := RewriteBlankImportNames(original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if synthetic != "" {
		t.Fatalf("expected empty synthetic name, got %q", synthetic)
	}
	if string(rewritten) != string(original) {
		t.Fatal("expected original bytes to be returned unchanged")
	}
}
