package wasmparser

import (
	"bytes"
	"testing"
)

// TestRecordType tests decoding a record with 2 fields: "name" (string=0x73), "age" (u32=0x79)
func TestRecordType(t *testing.T) {
	data := []byte{
		0x72,                          // opcode: record
		0x02,                          // count: 2
		0x04, 'n', 'a', 'm', 'e',     // field name "name"
		0x73,                          // type: string
		0x03, 'a', 'g', 'e',          // field name "age"
		0x79,                          // type: u32
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	dt, err := readComponentDefinedType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := dt.(*RecordType)
	if !ok {
		t.Fatalf("expected *RecordType, got %T", dt)
	}
	if len(rec.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(rec.Fields))
	}
	if rec.Fields[0].Name != "name" {
		t.Fatalf("field[0] name: got %q, want %q", rec.Fields[0].Name, "name")
	}
	if rec.Fields[0].Type != PrimString {
		t.Fatalf("field[0] type: got %v, want PrimString", rec.Fields[0].Type)
	}
	if rec.Fields[1].Name != "age" {
		t.Fatalf("field[1] name: got %q, want %q", rec.Fields[1].Name, "age")
	}
	if rec.Fields[1].Type != PrimU32 {
		t.Fatalf("field[1] type: got %v, want PrimU32", rec.Fields[1].Type)
	}
}

// TestListType tests decoding list<u8>
func TestListType(t *testing.T) {
	data := []byte{0x70, 0x7d} // list, u8
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	dt, err := readComponentDefinedType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lt, ok := dt.(*ListType)
	if !ok {
		t.Fatalf("expected *ListType, got %T", dt)
	}
	if lt.Element != PrimU8 {
		t.Fatalf("element type: got %v, want PrimU8", lt.Element)
	}
}

// TestOptionType tests decoding option<string>
func TestOptionType(t *testing.T) {
	data := []byte{0x6b, 0x73} // option, string
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	dt, err := readComponentDefinedType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ot, ok := dt.(*OptionType)
	if !ok {
		t.Fatalf("expected *OptionType, got %T", dt)
	}
	if ot.Inner != PrimString {
		t.Fatalf("inner type: got %v, want PrimString", ot.Inner)
	}
}

// TestResultType_BothPresent tests decoding result<string, u32>
// Encoding: 0x6a, ok-present=0x01, string=0x73, err-present=0x01, u32=0x79
func TestResultType_BothPresent(t *testing.T) {
	data := []byte{0x6a, 0x01, 0x73, 0x01, 0x79}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	dt, err := readComponentDefinedType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rt, ok := dt.(*ResultType)
	if !ok {
		t.Fatalf("expected *ResultType, got %T", dt)
	}
	if !rt.Ok.Valid {
		t.Fatal("expected Ok to be present")
	}
	if rt.Ok.Value != PrimString {
		t.Fatalf("Ok type: got %v, want PrimString", rt.Ok.Value)
	}
	if !rt.Err.Valid {
		t.Fatal("expected Err to be present")
	}
	if rt.Err.Value != PrimU32 {
		t.Fatalf("Err type: got %v, want PrimU32", rt.Err.Value)
	}
}

// TestResultType_BothAbsent tests decoding result<_, _> (both absent)
// Encoding: 0x6a, 0x00 (ok absent), 0x00 (err absent)
func TestResultType_BothAbsent(t *testing.T) {
	data := []byte{0x6a, 0x00, 0x00}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	dt, err := readComponentDefinedType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rt, ok := dt.(*ResultType)
	if !ok {
		t.Fatalf("expected *ResultType, got %T", dt)
	}
	if rt.Ok.Valid {
		t.Fatal("expected Ok to be absent")
	}
	if rt.Err.Valid {
		t.Fatal("expected Err to be absent")
	}
}

// TestEnumType tests decoding enum with 2 cases "red", "blue"
func TestEnumType(t *testing.T) {
	data := []byte{
		0x6d,                              // opcode: enum
		0x02,                              // count: 2
		0x03, 'r', 'e', 'd',              // "red"
		0x04, 'b', 'l', 'u', 'e',         // "blue"
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	dt, err := readComponentDefinedType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	et, ok := dt.(*EnumType)
	if !ok {
		t.Fatalf("expected *EnumType, got %T", dt)
	}
	if len(et.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(et.Labels))
	}
	if et.Labels[0] != "red" {
		t.Fatalf("label[0]: got %q, want %q", et.Labels[0], "red")
	}
	if et.Labels[1] != "blue" {
		t.Fatalf("label[1]: got %q, want %q", et.Labels[1], "blue")
	}
}

// TestOwnType tests decoding own<0>
func TestOwnType(t *testing.T) {
	data := []byte{0x69, 0x00} // own, u32=0
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	dt, err := readComponentDefinedType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ot, ok := dt.(*OwnType)
	if !ok {
		t.Fatalf("expected *OwnType, got %T", dt)
	}
	if ot.ResourceIndex != 0 {
		t.Fatalf("resource index: got %d, want 0", ot.ResourceIndex)
	}
}

// TestBorrowType tests decoding borrow<1>
func TestBorrowType(t *testing.T) {
	data := []byte{0x68, 0x01} // borrow, u32=1
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	dt, err := readComponentDefinedType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bt, ok := dt.(*BorrowType)
	if !ok {
		t.Fatalf("expected *BorrowType, got %T", dt)
	}
	if bt.ResourceIndex != 1 {
		t.Fatalf("resource index: got %d, want 1", bt.ResourceIndex)
	}
}

// TestTupleType tests decoding tuple<u8, string>
func TestTupleType(t *testing.T) {
	data := []byte{
		0x6f,  // opcode: tuple
		0x02,  // count: 2
		0x7d,  // u8
		0x73,  // string
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	dt, err := readComponentDefinedType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tt, ok := dt.(*TupleType)
	if !ok {
		t.Fatalf("expected *TupleType, got %T", dt)
	}
	if len(tt.Types) != 2 {
		t.Fatalf("expected 2 types, got %d", len(tt.Types))
	}
	if tt.Types[0] != PrimU8 {
		t.Fatalf("type[0]: got %v, want PrimU8", tt.Types[0])
	}
	if tt.Types[1] != PrimString {
		t.Fatalf("type[1]: got %v, want PrimString", tt.Types[1])
	}
}

// TestFlagsType tests decoding flags with 2 labels "read", "write"
func TestFlagsType(t *testing.T) {
	data := []byte{
		0x6e,                                    // opcode: flags
		0x02,                                    // count: 2
		0x04, 'r', 'e', 'a', 'd',               // "read"
		0x05, 'w', 'r', 'i', 't', 'e',          // "write"
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	dt, err := readComponentDefinedType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ft, ok := dt.(*FlagsType)
	if !ok {
		t.Fatalf("expected *FlagsType, got %T", dt)
	}
	if len(ft.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(ft.Labels))
	}
	if ft.Labels[0] != "read" {
		t.Fatalf("label[0]: got %q, want %q", ft.Labels[0], "read")
	}
	if ft.Labels[1] != "write" {
		t.Fatalf("label[1]: got %q, want %q", ft.Labels[1], "write")
	}
}

// TestVariantType tests decoding a variant with 2 cases
// Case 0: "some" with type string (present), no refines
// Case 1: "none" with no type, no refines
func TestVariantType(t *testing.T) {
	data := []byte{
		0x71,                          // opcode: variant
		0x02,                          // count: 2
		0x04, 's', 'o', 'm', 'e',     // case name "some"
		0x01,                          // type present
		0x73,                          // type: string
		0x00,                          // no refines
		0x04, 'n', 'o', 'n', 'e',     // case name "none"
		0x00,                          // no type
		0x00,                          // no refines
	}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	dt, err := readComponentDefinedType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	vt, ok := dt.(*VariantType)
	if !ok {
		t.Fatalf("expected *VariantType, got %T", dt)
	}
	if len(vt.Cases) != 2 {
		t.Fatalf("expected 2 cases, got %d", len(vt.Cases))
	}
	if vt.Cases[0].Name != "some" {
		t.Fatalf("case[0] name: got %q, want %q", vt.Cases[0].Name, "some")
	}
	if !vt.Cases[0].Type.Valid {
		t.Fatal("case[0]: expected type to be present")
	}
	if vt.Cases[0].Type.Value != PrimString {
		t.Fatalf("case[0] type: got %v, want PrimString", vt.Cases[0].Type.Value)
	}
	if vt.Cases[0].Refines.Valid {
		t.Fatal("case[0]: expected refines to be absent")
	}
	if vt.Cases[1].Name != "none" {
		t.Fatalf("case[1] name: got %q, want %q", vt.Cases[1].Name, "none")
	}
	if vt.Cases[1].Type.Valid {
		t.Fatal("case[1]: expected type to be absent")
	}
}

// TestReadComponentDefinedType_Primitive tests that a primitive opcode is decoded as PrimitiveValType
func TestReadComponentDefinedType_Primitive(t *testing.T) {
	data := []byte{0x7f} // bool
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	dt, err := readComponentDefinedType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pv, ok := dt.(PrimitiveValType)
	if !ok {
		t.Fatalf("expected PrimitiveValType, got %T", dt)
	}
	if pv != PrimBool {
		t.Fatalf("got %v, want PrimBool", pv)
	}
}

// TestReadComponentDefinedType_UnknownOpcode tests that an unknown opcode returns an error
func TestReadComponentDefinedType_UnknownOpcode(t *testing.T) {
	data := []byte{0x00} // unknown
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	_, err := readComponentDefinedType(r)
	if err == nil {
		t.Fatal("expected error for unknown opcode, got nil")
	}
}
