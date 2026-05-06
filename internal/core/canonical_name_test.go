package core

import (
	"bytes"
	"context"
	"testing"
)

// TestInstantiate_CanonicalImportNameMatch verifies that an instance
// import whose name carries one semver (e.g. "foo:bar/baz@0.2.3")
// can be satisfied by a provider supplied under a different but
// canonically-equivalent name ("foo:bar/baz@0.2.8"). Both names
// canonicalize to "foo:bar/baz@0.2" per the Component Model spec, so
// linking should succeed by literal equality on the canonical form.
func TestInstantiate_CanonicalImportNameMatch(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	// Provider component: exports the interface "foo:bar/baz@0.2.8"
	// containing a single func double(u32) -> u32 implemented in core
	// wasm.
	providerWAT := `(component
  (core module $m
    (func (export "double") (param i32) (result i32)
      local.get 0
      i32.const 1
      i32.shl
    )
  )
  (core instance $i (instantiate $m))
  (alias core export $i "double" (core func $cf))
  (func $f (param "x" u32) (result u32) (canon lift (core func $cf)))
  (instance $iface
    (export "double" (func $f))
  )
  (export "foo:bar/baz@0.2.8" (instance $iface))
)`
	providerComp, err := engine.LoadComponent(ctx, bytes.NewReader(buildComponentBytes(t, providerWAT)))
	if err != nil {
		t.Fatalf("load provider: %v", err)
	}
	providerInst, err := providerComp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("instantiate provider: %v", err)
	}
	defer providerInst.Close(ctx)

	provIface := providerInst.ExportedInstance("foo:bar/baz@0.2.8")
	if provIface == nil {
		t.Fatal("provider: missing exported instance foo:bar/baz@0.2.8")
	}

	// Consumer component: imports "foo:bar/baz@0.2.3" (different patch)
	// and re-exports its double func at the top level.
	consumerWAT := `(component
  (import "foo:bar/baz@0.2.3" (instance $h
    (export "double" (func (param "x" u32) (result u32)))
  ))
  (alias export $h "double" (func $d))
  (export "double" (func $d))
)`
	consumerComp, err := engine.LoadComponent(ctx, bytes.NewReader(buildComponentBytes(t, consumerWAT)))
	if err != nil {
		t.Fatalf("load consumer: %v", err)
	}

	// Wire the provider instance under its own name (0.2.8) — different
	// patch from the consumer's import (0.2.3). Canonicalization must
	// reduce both to "foo:bar/baz@0.2" for linking to succeed.
	consumerInst, err := consumerComp.Instantiate(ctx,
		WithInstanceImport("foo:bar/baz@0.2.8", provIface),
	)
	if err != nil {
		t.Fatalf("instantiate consumer with version-mismatched name: %v", err)
	}
	defer consumerInst.Close(ctx)

	got, err := consumerInst.ExportedFunc("double").Call(ctx, ValU32(21))
	if err != nil {
		t.Fatalf("double(21): %v", err)
	}
	if got[0] != ValU32(42) {
		t.Fatalf("double(21) = %v, want 42", got[0])
	}
}
