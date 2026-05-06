package canon

import (
	"context"
	"math"
	"testing"
)

// runMemToValStep runs the single emitted lift step at the given base and
// returns the Val.
func runMemToValStep(t *testing.T, v *memToValVisitor, gcc *gocallContext, base uint32) Val {
	t.Helper()
	if len(v.out) != 1 {
		t.Fatalf("expected 1 step, got %d", len(v.out))
	}
	val, err := v.out[0](context.Background(), gcc, base)
	if err != nil {
		t.Fatalf("step failed: %v", err)
	}
	return val
}

func TestMemToValU32(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteUint32Le(8, 0xCAFEBABE)

	v := &memToValVisitor{}
	v.VisitU32()
	if v.byteOff != 4 {
		t.Fatalf("byteOff=%d, want 4", v.byteOff)
	}
	if v.maxAlign != 4 {
		t.Fatalf("maxAlign=%d, want 4", v.maxAlign)
	}

	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 8)
	if u, ok := got.(ValU32); !ok || u != 0xCAFEBABE {
		t.Fatalf("got %#v", got)
	}
}

func TestMemToValBool(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteByte(0, 1)
	v := &memToValVisitor{}
	v.VisitBool()
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	if b, ok := got.(ValBool); !ok || !bool(b) {
		t.Fatalf("got %#v", got)
	}

	_ = mem.WriteByte(0, 0)
	got = runMemToValStep(t, v, gcc, 0)
	if b, ok := got.(ValBool); !ok || bool(b) {
		t.Fatalf("got %#v", got)
	}
}

func TestMemToValU8(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteByte(4, 0xAB)
	v := &memToValVisitor{}
	v.VisitU8()
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 4)
	if u, ok := got.(ValU8); !ok || u != 0xAB {
		t.Fatalf("got %#v", got)
	}
}

func TestMemToValS8Negative(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteByte(0, 0xFF)
	v := &memToValVisitor{}
	v.VisitS8()
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	if s, ok := got.(ValS8); !ok || s != -1 {
		t.Fatalf("got %#v", got)
	}
}

func TestMemToValS16Negative(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteUint16Le(0, 0xFFFF)
	v := &memToValVisitor{}
	v.VisitS16()
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	if s, ok := got.(ValS16); !ok || s != -1 {
		t.Fatalf("got %#v", got)
	}
}

func TestMemToValU64(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteUint64Le(0, 0xDEADBEEFCAFEBABE)
	v := &memToValVisitor{}
	v.VisitU64()
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	if u, ok := got.(ValU64); !ok || u != 0xDEADBEEFCAFEBABE {
		t.Fatalf("got %#v", got)
	}
}

func TestMemToValF32(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteUint32Le(0, math.Float32bits(3.14))
	v := &memToValVisitor{}
	v.VisitF32()
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	if f, ok := got.(ValF32); !ok || float32(f) != 3.14 {
		t.Fatalf("got %#v", got)
	}
}

func TestMemToValF64(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteUint64Le(0, math.Float64bits(3.14))
	v := &memToValVisitor{}
	v.VisitF64()
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	if f, ok := got.(ValF64); !ok || float64(f) != 3.14 {
		t.Fatalf("got %#v", got)
	}
}

func TestMemToValCharValid(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteUint32Le(0, 'A')
	v := &memToValVisitor{}
	v.VisitChar()
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	if c, ok := got.(ValChar); !ok || rune(c) != 'A' {
		t.Fatalf("got %#v", got)
	}
}

func TestMemToValCharInvalidSurrogate(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteUint32Le(0, 0xD800)
	v := &memToValVisitor{}
	v.VisitChar()
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	_, err := v.out[0](context.Background(), gcc, 0)
	if err == nil {
		t.Fatal("expected error for surrogate char")
	}
}

func TestMemToValEnum(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteByte(0, 2)
	v := &memToValVisitor{}
	v.VisitEnum(3)
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	e := got.(*ValEnum)
	if e.Discriminant() != 2 {
		t.Fatalf("disc=%d, want 2", e.Discriminant())
	}
}

func TestMemToValFlags(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteUint32Le(0, 0x15)
	v := &memToValVisitor{}
	v.VisitFlags([]string{"a", "b", "c", "d", "e"})
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	f := got.(*ValFlags)
	if f.packedBits() != 0x15 {
		t.Fatalf("bits=%b, want 0b10101", f.packedBits())
	}
}

func TestMemToValFlagsSizes(t *testing.T) {
	cases := []struct {
		n        int
		wantSize uint32
	}{
		{1, 1}, {8, 1}, {9, 2}, {16, 2}, {17, 4}, {32, 4},
	}
	for _, c := range cases {
		labels := make([]string, c.n)
		for i := range labels {
			labels[i] = "f"
		}
		v := &memToValVisitor{}
		v.VisitFlags(labels)
		if v.byteOff != c.wantSize {
			t.Errorf("n=%d: byteOff=%d, want %d", c.n, v.byteOff, c.wantSize)
		}
		if v.maxAlign != c.wantSize {
			t.Errorf("n=%d: maxAlign=%d, want %d", c.n, v.maxAlign, c.wantSize)
		}
	}
}

func TestMemToValFlagsSmallReadsOnlyOneByte(t *testing.T) {
	mem := setupMem(t, 64)
	// Write a sentinel in the upper 3 bytes that would corrupt the result
	// if the visitor reads a u32 unconditionally for n<=8 flags.
	_ = mem.WriteUint32Le(0, 0xFFFFFF03)
	v := &memToValVisitor{}
	v.VisitFlags([]string{"a", "b", "c"})
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	f := got.(*ValFlags)
	if f.packedBits() != 0x03 {
		t.Fatalf("bits=%#x, want 0x03 (small flags must read 1 byte, not 4)", f.packedBits())
	}
}

func TestMemToValRecord(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteUint32Le(0, 0x11111111)
	_ = mem.WriteUint32Le(4, 0x22222222)

	v := &memToValVisitor{}
	v.VisitRecord([]RecordField{
		{Name: "a", Type: testU32Type{}},
		{Name: "b", Type: testU32Type{}},
	})
	if v.byteOff != 8 {
		t.Fatalf("byteOff=%d, want 8", v.byteOff)
	}

	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	rec, ok := got.(*ValRecord)
	if !ok {
		t.Fatalf("got %T, want *ValRecord", got)
	}
	fields := rec.Fields()
	if fields[0].Name != "a" || fields[0].Val.(ValU32) != 0x11111111 {
		t.Fatalf("field 0: %+v", fields[0])
	}
	if fields[1].Name != "b" || fields[1].Val.(ValU32) != 0x22222222 {
		t.Fatalf("field 1: %+v", fields[1])
	}
}

func TestMemToValTuple(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteUint32Le(0, 100)
	_ = mem.WriteUint32Le(4, 200)

	v := &memToValVisitor{}
	v.VisitTuple([]Type{testU32Type{}, testU32Type{}})

	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	rec := got.(*ValRecord)
	if rec.FieldByIndex(0).(ValU32) != 100 || rec.FieldByIndex(1).(ValU32) != 200 {
		t.Fatalf("tuple=%+v", rec.Fields())
	}
}

func TestMemToValString(t *testing.T) {
	mem := setupMem(t, 256)
	if !mem.Write(64, []byte("hello")) {
		t.Fatal("write bytes")
	}
	_ = mem.WriteUint32Le(0, 64)
	_ = mem.WriteUint32Le(4, 5)

	v := &memToValVisitor{}
	v.VisitString()
	gcc := &gocallContext{callee: &transferSide{Memory: mem, StringEncoding: EncUTF8}}
	got := runMemToValStep(t, v, gcc, 0)
	if s, ok := got.(ValString); !ok || string(s) != "hello" {
		t.Fatalf("got %#v", got)
	}
}

func TestMemToValList(t *testing.T) {
	mem := setupMem(t, 256)
	_ = mem.WriteUint32Le(32, 10)
	_ = mem.WriteUint32Le(36, 20)
	_ = mem.WriteUint32Le(40, 30)
	_ = mem.WriteUint32Le(0, 32)
	_ = mem.WriteUint32Le(4, 3)

	v := &memToValVisitor{}
	v.VisitList(testU32Type{})
	gcc := &gocallContext{callee: &transferSide{Memory: mem, StringEncoding: EncUTF8}}
	got := runMemToValStep(t, v, gcc, 0)
	list, ok := got.(*ValList)
	if !ok {
		t.Fatalf("got %T, want *ValList", got)
	}
	if list.Len() != 3 {
		t.Fatalf("Len=%d, want 3", list.Len())
	}
	for i, want := range []uint32{10, 20, 30} {
		if u := list.Get(i).(ValU32); u != ValU32(want) {
			t.Fatalf("list[%d]=%d, want %d", i, u, want)
		}
	}
}

func TestMemToValVariantWithPayload(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteByte(0, 1) // case index
	_ = mem.WriteUint32Le(4, 42)

	v := &memToValVisitor{}
	v.VisitVariant([]VariantCase{
		{Name: "empty"},
		{Name: "withU32", Payload: testU32Type{}},
	})
	if v.byteOff != 8 {
		t.Fatalf("byteOff=%d, want 8", v.byteOff)
	}

	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	variant := got.(*ValVariant)
	if variant.Discriminant() != 1 {
		t.Fatalf("disc=%d, want 1", variant.Discriminant())
	}
	if u, ok := variant.Val().(ValU32); !ok || u != 42 {
		t.Fatalf("payload=%#v", variant.Val())
	}
}

func TestMemToValVariantEmptyCase(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteByte(0, 0)

	v := &memToValVisitor{}
	v.VisitVariant([]VariantCase{
		{Name: "empty"},
		{Name: "withU32", Payload: testU32Type{}},
	})
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	variant := got.(*ValVariant)
	if variant.Discriminant() != 0 {
		t.Fatalf("disc=%d, want 0", variant.Discriminant())
	}
	if variant.Val() != nil {
		t.Fatalf("payload=%v, want nil", variant.Val())
	}
}

func TestMemToValOptionSome(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteByte(0, 1)
	_ = mem.WriteUint32Le(4, 0xABCD)

	v := &memToValVisitor{}
	v.VisitOption(testU32Type{})

	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	opt := got.(*ValOption)
	if opt.IsNone() {
		t.Fatal("expected Some")
	}
	if u := opt.Val().(ValU32); u != 0xABCD {
		t.Fatalf("payload=%x, want 0xABCD", u)
	}
}

func TestMemToValOptionNone(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteByte(0, 0)

	v := &memToValVisitor{}
	v.VisitOption(testU32Type{})
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	opt := got.(*ValOption)
	if !opt.IsNone() {
		t.Fatal("expected None")
	}
}

func TestMemToValResultOk(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteByte(0, 0)
	_ = mem.WriteUint32Le(4, 111)

	v := &memToValVisitor{}
	v.VisitResult(testU32Type{}, testU32Type{})

	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	r := got.(*ValResult)
	if !r.IsOk() {
		t.Fatal("expected Ok")
	}
	if u := r.Ok().(ValU32); u != 111 {
		t.Fatalf("ok=%d, want 111", u)
	}
}

func TestMemToValResultErr(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteByte(0, 1)
	_ = mem.WriteUint32Le(4, 222)

	v := &memToValVisitor{}
	v.VisitResult(testU32Type{}, testU32Type{})
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	got := runMemToValStep(t, v, gcc, 0)
	r := got.(*ValResult)
	if r.IsOk() {
		t.Fatal("expected Err")
	}
	if u := r.Err().(ValU32); u != 222 {
		t.Fatalf("err=%d, want 222", u)
	}
}

func TestMemToValOwn(t *testing.T) {
	rt := &testResourceType{name: "R"}
	tbl := newStubResourceTable()
	live := tbl.IssueOwn(rt, 42)

	mem := setupMem(t, 64)
	_ = mem.WriteUint32Le(0, live.HandleID())

	v := &memToValVisitor{}
	v.VisitOwn(rt)
	gcc := &gocallContext{callee: &transferSide{Instance: &testInstance{}, Memory: mem, ResourceTable: tbl}}
	got := runMemToValStep(t, v, gcc, 0)
	h, ok := got.(*ValOwnHandle)
	if !ok {
		t.Fatalf("got %T, want *ValOwnHandle", got)
	}
	if h.Rep() != 42 {
		t.Fatalf("rep = %d, want 42", h.Rep())
	}
}

func TestMemToValBorrowPanics(t *testing.T) {
	mem := setupMem(t, 64)
	_ = mem.WriteUint32Le(0, 7)
	v := &memToValVisitor{}
	v.VisitBorrow(nil)
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic: borrow as result is not permitted")
		}
	}()
	runMemToValStep(t, v, gcc, 0)
}
