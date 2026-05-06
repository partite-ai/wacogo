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
// value well past any realistic wasm memory size. When the witgen wrap
// method tries to lift the string from that address, the Memory.Read
// call on the host stub's memory (which has only 64 KB) will fail and
// the wrapper must return a non-nil error.
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
// out-of-bounds memory pointer, the witgen-generated lift helper
// surfaces a descriptive error rather than panicking or returning zero
// data.
func TestJoinOOBMemoryRead(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	// Build a host instance that provides the concat interface. This
	// instance also supplies the stub memory that the wrap lift reads from.
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
	// host instance. The wrap method's lift closure reads the returned
	// pointer against the caller's stub memory (64 KB). 0xFFFF0000 is
	// far past that boundary, so the Memory.ReadUint32Le call fails.
	wrapped := concat.WrapInstance(callerInst, oobInst)
	_, err = wrapped.Join(ctx, []string{"a"}, ",")
	if err == nil {
		t.Fatal("expected memory-read error from OOB pointer, got nil")
	}
	if !strings.Contains(err.Error(), "memory") {
		t.Fatalf("expected error to mention 'memory', got: %v", err)
	}
}
