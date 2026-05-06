package fixturetests_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/concat"
)

// trapJoinWAT is a component that exports a join function whose core
// implementation executes the unreachable instruction, causing a wasm trap.
// The witgen wrap method must capture that trap and surface it as a non-nil
// error to the caller.
const trapJoinWAT = `(component
  (core module $m
    (memory (export "memory") 1)
    (func (export "realloc") (param i32 i32 i32 i32) (result i32)
      i32.const 0
    )
    (func (export "join") (param i32 i32 i32 i32) (result i32)
      unreachable
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

type trapStub struct{}

func (trapStub) Join(ctx context.Context, parts []string, sep string) (string, error) {
	return strings.Join(parts, sep), nil
}

// TestWasmTrapPropagates verifies that when a wasm callee traps via the
// unreachable instruction, the witgen-generated wrap method surfaces a
// non-nil error rather than panicking or returning zero data.
func TestWasmTrapPropagates(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	// Build a host instance that provides the concat interface.
	fac, err := concat.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	defer fac.Close(ctx)

	callerInst, err := fac.NewInstance(ctx, trapStub{}, nil)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	defer callerInst.Close(ctx)

	// Load and instantiate the trapping component.
	bin := watToBinary(t, trapJoinWAT)
	comp, err := e.LoadComponent(ctx, bytes.NewReader(bin))
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}
	trapInst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate trap component: %v", err)
	}
	defer trapInst.Close(ctx)

	// WrapInstance wires the trapping component's join export to the caller
	// host instance. Calling Join should propagate the wasm trap as an error.
	wrapped := concat.WrapInstance(callerInst, trapInst)
	_, err = wrapped.Join(ctx, []string{"a"}, ",")
	if err == nil {
		t.Fatal("expected wasm trap error, got nil")
	}
	// Soft check: the error message should mention the trap or unreachable.
	msg := err.Error()
	if !strings.Contains(msg, "trap") && !strings.Contains(msg, "unreachable") {
		t.Logf("note: error does not mention 'trap' or 'unreachable' (got: %v)", err)
	}
}
