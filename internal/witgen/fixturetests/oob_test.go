package fixturetests_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/concat"
)

// oobJoinWAT is a component that exports a join function whose core
// implementation always returns 0xFFFF0000 as the result pointer — a
// value past the callee's 64 KB memory. CallRaw must reject that result
// before the generated decoder reads it, returning a memory-bounds error.
const oobJoinWAT = `(component
  (core module $m
    (memory (export "memory") 1)
    (func (export "realloc") (param i32 i32 i32 i32) (result i32)
      i32.const 0
    )
    (func (export "join") (param i32 i32 i32 i32) (result i32)
      i32.const -65536
    )
  )
  (core instance $i (instantiate $m))
  (alias core export $i "memory" (core memory $mem))
  (alias core export $i "realloc" (core func $realloc))
  (alias core export $i "join" (core func $join_core))
  (type $ft (func (param "parts" (list string)) (param "sep" string) (result string)))
  (func $join_lifted (type $ft) (canon lift (core func $join_core) (memory $mem) (realloc $realloc)))
  (export "join" (func $join_lifted))
)`

type stubConcat struct{}

func (stubConcat) Join(ctx context.Context, parts []string, sep string) (string, error) {
	return strings.Join(parts, sep), nil
}

// TestJoinOOBMemoryRead verifies that when a wasm callee returns an
// out-of-bounds memory pointer, the generated wrapper surfaces a
// descriptive error rather than panicking or returning zero data.
func TestJoinOOBMemoryRead(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	// Build the caller-side instance for the generated concat wrapper.
	fac, err := concat.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	callerInst, err := fac.NewInstance(ctx, stubConcat{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer callerInst.Close(ctx)

	// Load and instantiate the OOB component.
	bin := watToBinary(t, oobJoinWAT)
	comp, err := e.LoadComponent(ctx, bytes.NewReader(bin))
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}
	oobInst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate oob component: %v", err)
	}
	defer oobInst.Close(ctx)

	// WrapInstance wires the OOB component's join export to the caller
	// host instance. CallRaw preflights the returned string against the
	// callee's memory before invoking the generated decoder. Reading its
	// pointer at 0xFFFF0000 must fail with a memory-bounds diagnostic.
	wrapped := concat.WrapInstance(callerInst, oobInst)
	_, err = wrapped.Join(ctx, []string{"a"}, ",")
	if err == nil {
		t.Fatal("expected memory-read error from OOB pointer, got nil")
	}
	if !strings.Contains(err.Error(), "memory") {
		t.Fatalf("expected error to mention 'memory', got: %v", err)
	}
}
