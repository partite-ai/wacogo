package host_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
	"github.com/partite-ai/wacogo/wasmtools"
)

// TestMultiImport_ResourceShared exercises the guest-side type check
// that fails when a host's wpInstance carries Build-time placeholder
// ResourceIDs for AddResourceRef. The guest imports two host
// interfaces; the second `use`s a resource defined by the first. Both
// instances must agree on the resource's identity for
// CheckInstantiation to accept them.
//
// Without the per-instance wpType wiring, this test fails with
// "mismatched resource types" inside the wasi-cli world's stdin/streams
// pair.
func TestMultiImport_ResourceShared(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	// Lender host component: defines resource "stream".
	lenderB := e.NewHostBuilder("lender")
	streamRT := lenderB.AddResource("stream", nil)
	_ = streamRT
	lender, err := lenderB.Build(ctx)
	if err != nil {
		t.Fatalf("lender Build: %v", err)
	}
	defer lender.Close(ctx)
	lenderInst, err := lender.Instantiate(ctx)
	if err != nil {
		t.Fatalf("lender Instantiate: %v", err)
	}
	defer lenderInst.Close(ctx)

	// Borrower host component: imports the stream resource via
	// AddResourceRef and re-exports it. (No funcs needed for this
	// test — the resource type export alone is what the guest will
	// match against.)
	borrowerB := e.NewHostBuilder("borrower")
	borrowerStreamRef := borrowerB.AddResourceRef("stream")
	borrower, err := borrowerB.Build(ctx)
	if err != nil {
		t.Fatalf("borrower Build: %v", err)
	}
	defer borrower.Close(ctx)
	borrowerInst, err := borrower.Instantiate(ctx,
		host.WithResourceFrom(borrowerStreamRef, lenderInst.Core(), "stream"))
	if err != nil {
		t.Fatalf("borrower Instantiate: %v", err)
	}
	defer borrowerInst.Close(ctx)

	// Guest component imports both interfaces and references the
	// resource through both. The resource type identity must match.
	guestWAT := `(component
  (import "lender" (instance $a
    (export "stream" (type (sub resource)))
  ))
  (alias export $a "stream" (type $stream))
  (import "borrower" (instance $b
    (export "stream" (type (eq $stream)))
  ))
)`
	guestBin := watToBinary(t, guestWAT)
	comp, err := e.LoadComponent(ctx, bytes.NewReader(guestBin))
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}
	if _, err := comp.Instantiate(ctx,
		wacogo.WithInstanceImport("lender", lenderInst.Core()),
		wacogo.WithInstanceImport("borrower", borrowerInst.Core()),
	); err != nil {
		t.Fatalf("guest Instantiate: %v", err)
	}
}

// TestMultiImport_DifferentLenders_Rejected confirms that supplying two
// borrower-style host instances pinned to DIFFERENT lenders is
// correctly rejected by CheckInstantiation. Both borrowers must agree
// on the underlying resource identity if the guest declares them
// linked via `use`.
func TestMultiImport_DifferentLenders_Rejected(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	mkLender := func(name string) *core.ComponentInstance {
		b := e.NewHostBuilder(name)
		b.AddResource("stream", nil)
		comp, err := b.Build(ctx)
		if err != nil {
			t.Fatalf("%s Build: %v", name, err)
		}
		t.Cleanup(func() { _ = comp.Close(ctx) })
		inst, err := comp.Instantiate(ctx)
		if err != nil {
			t.Fatalf("%s Instantiate: %v", name, err)
		}
		t.Cleanup(func() { _ = inst.Close(ctx) })
		return inst.Core()
	}
	lender1 := mkLender("lender1")
	lender2 := mkLender("lender2")

	mkBorrower := func(lender *core.ComponentInstance, name string) *core.ComponentInstance {
		b := e.NewHostBuilder(name)
		ref := b.AddResourceRef("stream")
		comp, err := b.Build(ctx)
		if err != nil {
			t.Fatalf("%s Build: %v", name, err)
		}
		t.Cleanup(func() { _ = comp.Close(ctx) })
		inst, err := comp.Instantiate(ctx, host.WithResourceFrom(ref, lender, "stream"))
		if err != nil {
			t.Fatalf("%s Instantiate: %v", name, err)
		}
		t.Cleanup(func() { _ = inst.Close(ctx) })
		return inst.Core()
	}
	b1 := mkBorrower(lender1, "borrower1")
	b2 := mkBorrower(lender2, "borrower2")

	guestWAT := `(component
  (import "a" (instance $a
    (export "stream" (type (sub resource)))
  ))
  (alias export $a "stream" (type $stream))
  (import "b" (instance $b
    (export "stream" (type (eq $stream)))
  ))
)`
	comp, err := e.LoadComponent(ctx, bytes.NewReader(watToBinary(t, guestWAT)))
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}
	if _, err := comp.Instantiate(ctx,
		wacogo.WithInstanceImport("a", b1),
		wacogo.WithInstanceImport("b", b2),
	); err == nil {
		t.Fatal("Instantiate with mismatched lenders should fail; got nil error")
	}
}

func watToBinary(t *testing.T, wat string) []byte {
	t.Helper()
	ctx := t.Context()
	tool, err := wasmtools.Default(ctx)
	if err != nil {
		t.Fatalf("wasmtools.Default: %v", err)
	}
	out, err := tool.Parse(ctx, []byte(wat))
	if err != nil {
		t.Fatalf("wasm-tools parse: %v", err)
	}
	return out
}
