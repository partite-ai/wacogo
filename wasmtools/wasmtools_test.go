package wasmtools

import (
	"bytes"
	"testing"
)

func TestParse(t *testing.T) {
	ctx := t.Context()
	tool, err := New(ctx)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer tool.Close(ctx)

	wat := []byte(`(component
  (core module $m
    (func (export "add") (param i32 i32) (result i32)
      local.get 0
      local.get 1
      i32.add)))`)

	got, err := tool.Parse(ctx, wat)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !bytes.HasPrefix(got, []byte{0x00, 0x61, 0x73, 0x6d}) {
		t.Fatalf("output does not start with wasm magic: % x ...", got[:8])
	}
	// Component-model preamble: layer/version bytes 0x0d 0x00 0x01 0x00.
	if len(got) < 8 || got[6] != 0x01 || got[7] != 0x00 {
		t.Fatalf("output not a component (header: % x)", got[:8])
	}
}

func TestParseError(t *testing.T) {
	ctx := t.Context()
	tool, err := New(ctx)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer tool.Close(ctx)

	if _, err := tool.Parse(ctx, []byte("not valid wat")); err == nil {
		t.Fatal("expected error from invalid WAT")
	}
}
