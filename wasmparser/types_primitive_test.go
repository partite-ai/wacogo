package wasmparser

import (
	"bytes"
	"testing"
)

func TestPrimitiveValType_AllOpcodes(t *testing.T) {
	tests := []struct {
		name     string
		opcode   byte
		expected PrimitiveValType
	}{
		{"Bool", 0x7f, PrimBool},
		{"S8", 0x7e, PrimS8},
		{"U8", 0x7d, PrimU8},
		{"S16", 0x7c, PrimS16},
		{"U16", 0x7b, PrimU16},
		{"S32", 0x7a, PrimS32},
		{"U32", 0x79, PrimU32},
		{"S64", 0x78, PrimS64},
		{"U64", 0x77, PrimU64},
		{"F32", 0x76, PrimF32},
		{"F64", 0x75, PrimF64},
		{"Char", 0x74, PrimChar},
		{"String", 0x73, PrimString},
		{"ErrorContext", 0x64, PrimErrorContext},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newBinaryReaderAt(bytes.NewReader([]byte{tt.opcode}), 0)
			var pv PrimitiveValType
			if err := pv.unmarshalBinary(r); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pv != tt.expected {
				t.Fatalf("got %d, want %d", pv, tt.expected)
			}
		})
	}
}

func TestPrimitiveValType_InvalidOpcode(t *testing.T) {
	r := newBinaryReaderAt(bytes.NewReader([]byte{0x00}), 0)
	var pv PrimitiveValType
	if err := pv.unmarshalBinary(r); err == nil {
		t.Fatal("expected error for unknown opcode, got nil")
	}
}

func TestReadComponentValType_Primitive(t *testing.T) {
	// 0x73 = PrimString
	r := newBinaryReaderAt(bytes.NewReader([]byte{0x73}), 0)
	vt, err := readComponentValType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pv, ok := vt.(PrimitiveValType)
	if !ok {
		t.Fatalf("expected PrimitiveValType, got %T", vt)
	}
	if pv != PrimString {
		t.Fatalf("got %d, want PrimString (%d)", pv, PrimString)
	}
}

func TestReadComponentValType_TypeIndex(t *testing.T) {
	// 0x05 = type index 5 (positive s33, not a primitive opcode)
	r := newBinaryReaderAt(bytes.NewReader([]byte{0x05}), 0)
	vt, err := readComponentValType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ti, ok := vt.(TypeIndexValType)
	if !ok {
		t.Fatalf("expected TypeIndexValType, got %T", vt)
	}
	if ti != TypeIndexValType(5) {
		t.Fatalf("got %d, want 5", ti)
	}
}

func TestReadComponentValType_Bool(t *testing.T) {
	// 0x7f = PrimBool
	r := newBinaryReaderAt(bytes.NewReader([]byte{0x7f}), 0)
	vt, err := readComponentValType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pv, ok := vt.(PrimitiveValType)
	if !ok {
		t.Fatalf("expected PrimitiveValType, got %T", vt)
	}
	if pv != PrimBool {
		t.Fatalf("got %d, want PrimBool", pv)
	}
}

func TestReadComponentValType_U8(t *testing.T) {
	// 0x7d = PrimU8
	r := newBinaryReaderAt(bytes.NewReader([]byte{0x7d}), 0)
	vt, err := readComponentValType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pv, ok := vt.(PrimitiveValType)
	if !ok {
		t.Fatalf("expected PrimitiveValType, got %T", vt)
	}
	if pv != PrimU8 {
		t.Fatalf("got %d, want PrimU8", pv)
	}
}
