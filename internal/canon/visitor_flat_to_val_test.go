package canon

import (
	"context"
	"math"
	"testing"
)

// runFlatToValStep runs the single emitted lift step and returns the Val.
func runFlatToValStep(t *testing.T, v *flatToValVisitor, gcc *gocallContext) Val {
	t.Helper()
	if len(v.out) != 1 {
		t.Fatalf("expected 1 step, got %d", len(v.out))
	}
	val, err := v.out[0](context.Background(), gcc, 0)
	if err != nil {
		t.Fatalf("step failed: %v", err)
	}
	return val
}

func TestFlatToValBool(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitBool()

	gcc := &gocallContext{registers: []uint64{1}}
	got := runFlatToValStep(t, v, gcc)
	if b, ok := got.(ValBool); !ok || b != true {
		t.Fatalf("got %#v, want ValBool(true)", got)
	}

	gcc = &gocallContext{registers: []uint64{0}}
	got = runFlatToValStep(t, v, gcc)
	if b, ok := got.(ValBool); !ok || b != false {
		t.Fatalf("got %#v, want ValBool(false)", got)
	}
}

func TestFlatToValU8(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitU8()
	gcc := &gocallContext{registers: []uint64{0xAB}}
	got := runFlatToValStep(t, v, gcc)
	if u, ok := got.(ValU8); !ok || u != 0xAB {
		t.Fatalf("got %#v", got)
	}
}

func TestFlatToValU16(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitU16()
	gcc := &gocallContext{registers: []uint64{0xBEEF}}
	got := runFlatToValStep(t, v, gcc)
	if u, ok := got.(ValU16); !ok || u != 0xBEEF {
		t.Fatalf("got %#v", got)
	}
}

func TestFlatToValU32(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitU32()
	gcc := &gocallContext{registers: []uint64{0xCAFEBABE}}
	got := runFlatToValStep(t, v, gcc)
	if u, ok := got.(ValU32); !ok || u != 0xCAFEBABE {
		t.Fatalf("got %#v", got)
	}
}

func TestFlatToValU64(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitU64()
	gcc := &gocallContext{registers: []uint64{0xDEADBEEFCAFEBABE}}
	got := runFlatToValStep(t, v, gcc)
	if u, ok := got.(ValU64); !ok || u != 0xDEADBEEFCAFEBABE {
		t.Fatalf("got %#v", got)
	}
}

func TestFlatToValS8Negative(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitS8()
	gcc := &gocallContext{registers: []uint64{0xFFFFFFFF}}
	got := runFlatToValStep(t, v, gcc)
	if s, ok := got.(ValS8); !ok || s != -1 {
		t.Fatalf("got %#v, want ValS8(-1)", got)
	}
}

func TestFlatToValS16Negative(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitS16()
	gcc := &gocallContext{registers: []uint64{0xFFFFFFFF}}
	got := runFlatToValStep(t, v, gcc)
	if s, ok := got.(ValS16); !ok || s != -1 {
		t.Fatalf("got %#v", got)
	}
}

func TestFlatToValS32Negative(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitS32()
	gcc := &gocallContext{registers: []uint64{0xFFFFFFFF}}
	got := runFlatToValStep(t, v, gcc)
	if s, ok := got.(ValS32); !ok || s != -1 {
		t.Fatalf("got %#v", got)
	}
}

func TestFlatToValS64Negative(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitS64()
	gcc := &gocallContext{registers: []uint64{0xFFFFFFFFFFFFFFFF}}
	got := runFlatToValStep(t, v, gcc)
	if s, ok := got.(ValS64); !ok || s != -1 {
		t.Fatalf("got %#v", got)
	}
}

func TestFlatToValF32(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitF32()
	gcc := &gocallContext{registers: []uint64{uint64(math.Float32bits(3.14))}}
	got := runFlatToValStep(t, v, gcc)
	if f, ok := got.(ValF32); !ok || float32(f) != 3.14 {
		t.Fatalf("got %#v", got)
	}
}

func TestFlatToValF64(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitF64()
	gcc := &gocallContext{registers: []uint64{math.Float64bits(3.14)}}
	got := runFlatToValStep(t, v, gcc)
	if f, ok := got.(ValF64); !ok || float64(f) != 3.14 {
		t.Fatalf("got %#v", got)
	}
}

func TestFlatToValCharValid(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitChar()
	gcc := &gocallContext{registers: []uint64{'A'}}
	got := runFlatToValStep(t, v, gcc)
	if c, ok := got.(ValChar); !ok || rune(c) != 'A' {
		t.Fatalf("got %#v", got)
	}
}

func TestFlatToValCharInvalidSurrogate(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitChar()
	gcc := &gocallContext{registers: []uint64{0xD800}}
	_, err := v.out[0](context.Background(), gcc, 0)
	if err == nil {
		t.Fatal("expected error for surrogate char")
	}
}

func TestFlatToValEnum(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitEnum(3)
	gcc := &gocallContext{registers: []uint64{2}}
	got := runFlatToValStep(t, v, gcc)
	e, ok := got.(*ValEnum)
	if !ok {
		t.Fatalf("got %T, want *ValEnum", got)
	}
	if e.Discriminant() != 2 {
		t.Fatalf("disc=%d, want 2", e.Discriminant())
	}
}

func TestFlatToValEnumOutOfRange(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitEnum(3)
	gcc := &gocallContext{registers: []uint64{5}}
	_, err := v.out[0](context.Background(), gcc, 0)
	if err == nil {
		t.Fatal("expected error for out-of-range enum disc")
	}
}

func TestFlatToValFlags(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitFlags([]string{"a", "b", "c", "d", "e"})
	gcc := &gocallContext{registers: []uint64{0x15}}
	got := runFlatToValStep(t, v, gcc)
	f, ok := got.(*ValFlags)
	if !ok {
		t.Fatalf("got %T, want *ValFlags", got)
	}
	if f.packedBits() != 0x15 {
		t.Fatalf("bits=%b, want 0b10101", f.packedBits())
	}
}

func TestFlatToValFlagsMasksAboveNumLabels(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitFlags([]string{"a", "b", "c"})
	gcc := &gocallContext{registers: []uint64{0xFF}}
	got := runFlatToValStep(t, v, gcc)
	f := got.(*ValFlags)
	if f.packedBits() != 0x7 {
		t.Fatalf("bits=%b, want 0b111", f.packedBits())
	}
}

func TestFlatToValRecord(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitRecord([]RecordField{
		{Name: "a", Type: testU32Type{}},
		{Name: "b", Type: testU32Type{}},
	})
	if v.slot != 2 {
		t.Fatalf("slot=%d, want 2", v.slot)
	}
	gcc := &gocallContext{registers: []uint64{10, 20}}
	got := runFlatToValStep(t, v, gcc)
	rec, ok := got.(*ValRecord)
	if !ok {
		t.Fatalf("got %T, want *ValRecord", got)
	}
	fields := rec.Fields()
	if len(fields) != 2 {
		t.Fatalf("fields=%d, want 2", len(fields))
	}
	if fields[0].Name != "a" || fields[0].Val.(ValU32) != 10 {
		t.Fatalf("field 0: %+v", fields[0])
	}
	if fields[1].Name != "b" || fields[1].Val.(ValU32) != 20 {
		t.Fatalf("field 1: %+v", fields[1])
	}
}

func TestFlatToValTuple(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitTuple([]Type{testU32Type{}, testU32Type{}})
	gcc := &gocallContext{registers: []uint64{100, 200}}
	got := runFlatToValStep(t, v, gcc)
	rec := got.(*ValRecord)
	if rec.FieldByIndex(0).(ValU32) != 100 || rec.FieldByIndex(1).(ValU32) != 200 {
		t.Fatalf("tuple: %+v", rec.Fields())
	}
	// Tuple fields are positionally named "0", "1", etc.
	if rec.Fields()[0].Name != "0" {
		t.Fatalf("tuple field 0 name: %q, want %q", rec.Fields()[0].Name, "0")
	}
	if rec.Fields()[1].Name != "1" {
		t.Fatalf("tuple field 1 name: %q, want %q", rec.Fields()[1].Name, "1")
	}
}

func TestFlatToValOwn(t *testing.T) {
	rt := &testResourceType{name: "R"}
	tbl := newStubResourceTable()
	live := tbl.IssueOwn(rt, 42)

	v := &flatToValVisitor{}
	v.VisitOwn(rt)
	gcc := &gocallContext{
		callee:    &transferSide{Instance: &testInstance{}, ResourceTable: tbl},
		registers: []uint64{uint64(live.HandleID())},
	}
	got := runFlatToValStep(t, v, gcc)
	h, ok := got.(*ValOwnHandle)
	if !ok {
		t.Fatalf("got %T, want *ValOwnHandle", got)
	}
	if h.Rep() != 42 {
		t.Fatalf("rep = %d, want 42", h.Rep())
	}
	// Source slot must be free after lift.
	if _, err := tbl.LookupOwn(rt, live.HandleID()); err == nil {
		t.Errorf("source handle still in table after lift")
	}
}

func TestFlatToValBorrowPanics(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitBorrow(nil)
	gcc := &gocallContext{
		callee:    &transferSide{},
		registers: []uint64{7},
	}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic: borrow as result is not permitted")
		}
	}()
	runFlatToValStep(t, v, gcc)
}

func TestFlatToValString(t *testing.T) {
	mem := setupMem(t, 256)
	if !mem.Write(16, []byte("hello")) {
		t.Fatal("failed to seed mem")
	}
	v := &flatToValVisitor{}
	v.VisitString()
	gcc := &gocallContext{
		callee: &transferSide{
			Memory:         mem,
			StringEncoding: EncUTF8,
		},
		registers: []uint64{16, 5}, // ptr=16, coded=5
	}
	got := runFlatToValStep(t, v, gcc)
	if s, ok := got.(ValString); !ok || string(s) != "hello" {
		t.Fatalf("got %#v, want ValString(\"hello\")", got)
	}
}

func TestFlatToValList(t *testing.T) {
	mem := setupMem(t, 256)
	_ = mem.WriteUint32Le(32, 10)
	_ = mem.WriteUint32Le(36, 20)
	_ = mem.WriteUint32Le(40, 30)

	v := &flatToValVisitor{}
	v.VisitList(testU32Type{})
	gcc := &gocallContext{
		callee:    &transferSide{Memory: mem, StringEncoding: EncUTF8},
		registers: []uint64{32, 3}, // ptr=32, n=3
	}
	got := runFlatToValStep(t, v, gcc)
	list, ok := got.(*ValList)
	if !ok {
		t.Fatalf("got %T, want *ValList", got)
	}
	if list.Len() != 3 {
		t.Fatalf("list.Len=%d, want 3", list.Len())
	}
	for i, want := range []uint32{10, 20, 30} {
		if u := list.Get(i).(ValU32); u != ValU32(want) {
			t.Fatalf("list[%d]=%d, want %d", i, u, want)
		}
	}
}

func TestFlatToValVariantEmptyCase(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitVariant([]VariantCase{
		{Name: "empty"},
		{Name: "u32", Payload: testU32Type{}},
	})
	if v.slot != 2 {
		t.Fatalf("slots=%d, want 2", v.slot)
	}
	gcc := &gocallContext{registers: []uint64{0, 0}}
	got := runFlatToValStep(t, v, gcc)
	variant, ok := got.(*ValVariant)
	if !ok {
		t.Fatalf("got %T, want *ValVariant", got)
	}
	if variant.Discriminant() != 0 {
		t.Fatalf("disc=%d, want 0", variant.Discriminant())
	}
	if variant.Val() != nil {
		t.Fatalf("payload=%v, want nil", variant.Val())
	}
}

func TestFlatToValVariantWithPayload(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitVariant([]VariantCase{
		{Name: "empty"},
		{Name: "u32", Payload: testU32Type{}},
	})
	gcc := &gocallContext{registers: []uint64{1, 77}}
	got := runFlatToValStep(t, v, gcc)
	variant := got.(*ValVariant)
	if variant.Discriminant() != 1 {
		t.Fatalf("disc=%d, want 1", variant.Discriminant())
	}
	if u, ok := variant.Val().(ValU32); !ok || u != 77 {
		t.Fatalf("payload=%#v", variant.Val())
	}
}

func TestFlatToValOptionNone(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitOption(testU32Type{})
	gcc := &gocallContext{registers: []uint64{0, 0}}
	got := runFlatToValStep(t, v, gcc)
	opt, ok := got.(*ValOption)
	if !ok {
		t.Fatalf("got %T, want *ValOption", got)
	}
	if !opt.IsNone() {
		t.Fatal("expected None")
	}
}

func TestFlatToValOptionSome(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitOption(testU32Type{})
	gcc := &gocallContext{registers: []uint64{1, 42}}
	got := runFlatToValStep(t, v, gcc)
	opt := got.(*ValOption)
	if opt.IsNone() {
		t.Fatal("expected Some")
	}
	if u, ok := opt.Val().(ValU32); !ok || u != 42 {
		t.Fatalf("payload=%#v, want ValU32(42)", opt.Val())
	}
}

func TestFlatToValResultOk(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitResult(testU32Type{}, testU32Type{})
	gcc := &gocallContext{registers: []uint64{0, 111}}
	got := runFlatToValStep(t, v, gcc)
	r, ok := got.(*ValResult)
	if !ok {
		t.Fatalf("got %T", got)
	}
	if !r.IsOk() {
		t.Fatal("expected Ok")
	}
	if u := r.Ok().(ValU32); u != 111 {
		t.Fatalf("ok=%d, want 111", u)
	}
}

func TestFlatToValResultErr(t *testing.T) {
	v := &flatToValVisitor{}
	v.VisitResult(testU32Type{}, testU32Type{})
	gcc := &gocallContext{registers: []uint64{1, 222}}
	got := runFlatToValStep(t, v, gcc)
	r := got.(*ValResult)
	if r.IsOk() {
		t.Fatal("expected Err")
	}
	if u := r.Err().(ValU32); u != 222 {
		t.Fatalf("err=%d, want 222", u)
	}
}
