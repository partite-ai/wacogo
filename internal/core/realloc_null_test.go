package core

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// reallocComponentWAT exports f: func(s: string) over a core module whose
// realloc returns the given expression. "(i32.const 0)" models a guest such
// as TinyGo that returns null for a zero-size request.
func reallocComponentWAT(zeroSizeResult string) string {
	return `(component
  (core module $m
    (memory (export "memory") 1)
    (func (export "realloc") (param i32 i32 i32 i32) (result i32)
      (if (result i32) (i32.eqz (local.get 3))
        (then ` + zeroSizeResult + `)
        (else (i32.const 1024))))
    (func (export "f") (param i32 i32))
  )
  (core instance $i (instantiate $m))
  (func (export "f") (param "s" string)
    (canon lift (core func $i "f") (memory $i "memory") (realloc (func $i "realloc")) string-encoding=utf8))
)`
}

func callStringExport(t *testing.T, wat string, args ...string) error {
	t.Helper()
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
	f := inst.ExportedFunc("f")
	if f == nil {
		t.Fatal("export f missing")
	}
	for _, arg := range args {
		if _, err := f.Call(ctx, ValString(arg)); err != nil {
			return err
		}
	}
	return nil
}

func TestReallocNullAcceptedForZeroSize(t *testing.T) {
	if err := callStringExport(t, reallocComponentWAT("(i32.const 0)"), "x", ""); err != nil {
		t.Fatalf("lowering an empty string: %v", err)
	}
}

// TestReallocNullAcceptedForNonZeroSize checks address 0 is accepted for
// any size: the canonical ABI checks only alignment and bounds.
func TestReallocNullAcceptedForNonZeroSize(t *testing.T) {
	wat := strings.Replace(reallocComponentWAT("(i32.const 0)"), "(else (i32.const 1024))", "(else (i32.const 0))", 1)
	if err := callStringExport(t, wat, "x"); err != nil {
		t.Fatalf("lowering a non-empty string: %v", err)
	}
}
