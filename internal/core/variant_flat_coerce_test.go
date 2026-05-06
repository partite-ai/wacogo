package core

import (
	"bytes"
	"context"
	"testing"
)

// TestFuncCall_VariantFlatCoerceF32ThroughI64Join verifies that a variant
// whose cases widen to an i64 joined flat payload round-trips an f32 case
// without bit corruption.
//
// Variant { a(f32), b(s64) } flattens to (i32 disc, i64 payload). Case a
// requires the lower path to coerce f32→i64 (high bits zero) and the
// callee to reinterpret the low 32 bits back as f32. A buggy coerce will
// show up as a divergent f32 on return.
func TestFuncCall_VariantFlatCoerceF32ThroughI64Join(t *testing.T) {
	const wat = `(component
  (core module $m
    (memory (export "memory") 1)
    (func (export "realloc") (param i32 i32 i32 i32) (result i32) i32.const 0)
    ;; params: (disc: i32, payload: i64).
    ;;   case 0 (a): low 32 bits of payload are the f32 bits; reinterpret.
    ;;   case 1 (b): return 0.0.
    (func (export "extract") (param i32 i64) (result f32)
      local.get 0
      (if (result f32)
        (then f32.const 0)
        (else
          local.get 1
          i32.wrap_i64
          f32.reinterpret_i32))))
  (core instance $i (instantiate $m))
  (alias core export $i "memory" (core memory $mem))
  (alias core export $i "realloc" (core func $realloc))
  (alias core export $i "extract" (core func $ex))
  (type $vt (variant (case "a" f32) (case "b" s64)))
  (export $vt2 "vt" (type $vt))
  (type $ft (func (param "v" $vt2) (result f32)))
  (func $lifted (type $ft) (canon lift (core func $ex) (memory $mem) (realloc $realloc)))
  (export "extract" (func $lifted)))`

	ctx := context.Background()
	engine := NewEngine(ctx)
	defer engine.Close(ctx)

	comp, err := engine.LoadComponent(ctx, bytes.NewReader(buildComponentBytes(t, wat)))
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}
	inst, err := comp.Instantiate(ctx)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	defer inst.Close(ctx)

	fn := inst.ExportedFunc("extract")
	if fn == nil {
		t.Fatal("export 'extract' missing")
	}

	out, err := fn.Call(ctx, NewValVariant(0, ValF32(1.5)))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	got, ok := out[0].(ValF32)
	if !ok {
		t.Fatalf("result type: got %T, want ValF32", out[0])
	}
	if float32(got) != 1.5 {
		t.Errorf("round-trip f32 through i64 join: got %v, want 1.5", got)
	}
}
