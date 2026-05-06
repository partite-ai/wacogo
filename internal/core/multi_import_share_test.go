package core

import (
	"bytes"
	"context"
	"testing"
)

// TestMultiImport_RealComponentResourceShare exercises the shared-
// resource-across-distinct-instances pattern using only wasm-loaded
// components. Confirms wasmparser's CheckInstantiation accepts the
// case so we can use it as the spec-anchored reference for the host
// equivalent.
func TestMultiImport_RealComponentResourceShare(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	lenderBin := buildComponentBytes(t, `(component
  (type $r (resource (rep i32)))
  (export "r" (type $r))
)`)
	lenderC, err := engine.LoadComponent(ctx, bytes.NewReader(lenderBin))
	if err != nil {
		t.Fatalf("load lender: %v", err)
	}
	lenderI, err := lenderC.Instantiate(ctx)
	if err != nil {
		t.Fatalf("instantiate lender: %v", err)
	}
	defer lenderI.Close(ctx)

	borrowerBin := buildComponentBytes(t, `(component
  (import "src" (instance $s (export "r" (type (sub resource)))))
  (alias export $s "r" (type $r))
  (export "r" (type $r))
)`)
	borrowerC, err := engine.LoadComponent(ctx, bytes.NewReader(borrowerBin))
	if err != nil {
		t.Fatalf("load borrower: %v", err)
	}
	borrowerI, err := borrowerC.Instantiate(ctx, WithInstanceImport("src", lenderI))
	if err != nil {
		t.Fatalf("instantiate borrower: %v", err)
	}
	defer borrowerI.Close(ctx)

	guestBin := buildComponentBytes(t, `(component
  (import "I1" (instance $i1 (export "r" (type (sub resource)))))
  (alias export $i1 "r" (type $r))
  (import "I2" (instance $i2 (export "r" (type (eq $r)))))
)`)
	guestC, err := engine.LoadComponent(ctx, bytes.NewReader(guestBin))
	if err != nil {
		t.Fatalf("load guest: %v", err)
	}
	if _, err := guestC.Instantiate(ctx,
		WithInstanceImport("I1", lenderI),
		WithInstanceImport("I2", borrowerI),
	); err != nil {
		t.Fatalf("guest Instantiate: %v", err)
	}
}
