package canon

import (
	"context"
	"math"
	"testing"
)

// runValToFlatStep runs the single emitted step against gcc with the given
// input Val.
func runValToFlatStep(t *testing.T, v *valToFlatVisitor, gcc *gocallContext, val Val) {
	t.Helper()
	if len(v.out) != 1 {
		t.Fatalf("expected 1 step, got %d", len(v.out))
	}
	if err := v.out[0](context.Background(), gcc, val, 0); err != nil {
		t.Fatalf("step failed: %v", err)
	}
}

func TestValToFlatBoolTrue(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitBool()
	gcc := &gocallContext{registers: make([]uint64, 1)}
	runValToFlatStep(t, v, gcc, ValBool(true))
	if gcc.registers[0] != 1 {
		t.Fatalf("bool true -> %d, want 1", gcc.registers[0])
	}
}

func TestValToFlatBoolFalse(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitBool()
	gcc := &gocallContext{registers: make([]uint64, 1)}
	runValToFlatStep(t, v, gcc, ValBool(false))
	if gcc.registers[0] != 0 {
		t.Fatalf("bool false -> %d, want 0", gcc.registers[0])
	}
}

func TestValToFlatU8(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitU8()
	gcc := &gocallContext{registers: make([]uint64, 1)}
	runValToFlatStep(t, v, gcc, ValU8(0xAB))
	if gcc.registers[0] != 0xAB {
		t.Fatalf("u8 not lowered: got %x", gcc.registers[0])
	}
}

func TestValToFlatU16(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitU16()
	gcc := &gocallContext{registers: make([]uint64, 1)}
	runValToFlatStep(t, v, gcc, ValU16(0xBEEF))
	if gcc.registers[0] != 0xBEEF {
		t.Fatalf("u16 not lowered: got %x", gcc.registers[0])
	}
}

func TestValToFlatU32(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitU32()
	gcc := &gocallContext{registers: make([]uint64, 1)}
	runValToFlatStep(t, v, gcc, ValU32(42))
	if gcc.registers[0] != 42 {
		t.Fatalf("u32 not lowered: got %d", gcc.registers[0])
	}
}

func TestValToFlatU64(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitU64()
	gcc := &gocallContext{registers: make([]uint64, 1)}
	runValToFlatStep(t, v, gcc, ValU64(0xDEADBEEFCAFEBABE))
	if gcc.registers[0] != 0xDEADBEEFCAFEBABE {
		t.Fatalf("u64 not lowered: got %x", gcc.registers[0])
	}
}

func TestValToFlatS8SignExtend(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitS8()
	gcc := &gocallContext{registers: make([]uint64, 1)}
	runValToFlatStep(t, v, gcc, ValS8(-1))
	if gcc.registers[0] != 0xFFFFFFFF {
		t.Fatalf("s8 sign-extend: got %x, want 0xFFFFFFFF", gcc.registers[0])
	}
}

func TestValToFlatS16SignExtend(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitS16()
	gcc := &gocallContext{registers: make([]uint64, 1)}
	runValToFlatStep(t, v, gcc, ValS16(-1))
	if gcc.registers[0] != 0xFFFFFFFF {
		t.Fatalf("s16 sign-extend: got %x, want 0xFFFFFFFF", gcc.registers[0])
	}
}

func TestValToFlatS32(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitS32()
	gcc := &gocallContext{registers: make([]uint64, 1)}
	runValToFlatStep(t, v, gcc, ValS32(-1))
	if gcc.registers[0] != 0xFFFFFFFF {
		t.Fatalf("s32 -1: got %x, want 0xFFFFFFFF", gcc.registers[0])
	}
}

func TestValToFlatS64(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitS64()
	gcc := &gocallContext{registers: make([]uint64, 1)}
	runValToFlatStep(t, v, gcc, ValS64(-1))
	if gcc.registers[0] != 0xFFFFFFFFFFFFFFFF {
		t.Fatalf("s64 -1: got %x", gcc.registers[0])
	}
}

func TestValToFlatF32(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitF32()
	gcc := &gocallContext{registers: make([]uint64, 1)}
	runValToFlatStep(t, v, gcc, ValF32(3.14))
	want := uint64(math.Float32bits(3.14))
	if gcc.registers[0] != want {
		t.Fatalf("f32 bits: got %x, want %x", gcc.registers[0], want)
	}
}

func TestValToFlatF64(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitF64()
	gcc := &gocallContext{registers: make([]uint64, 1)}
	runValToFlatStep(t, v, gcc, ValF64(3.14))
	want := math.Float64bits(3.14)
	if gcc.registers[0] != want {
		t.Fatalf("f64 bits: got %x, want %x", gcc.registers[0], want)
	}
}

func TestValToFlatCharValid(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitChar()
	gcc := &gocallContext{registers: make([]uint64, 1)}
	runValToFlatStep(t, v, gcc, ValChar('A'))
	if gcc.registers[0] != 'A' {
		t.Fatalf("char valid: got %x", gcc.registers[0])
	}
}

func TestValToFlatCharInvalid(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitChar()
	gcc := &gocallContext{registers: make([]uint64, 1)}
	err := v.out[0](context.Background(), gcc, ValChar(0xD800), 0)
	if err == nil {
		t.Fatal("expected error for surrogate char")
	}
}

func TestValToFlatRecord(t *testing.T) {
	// Record{a: u32, b: u32} lowered to flat slots.
	v := &valToFlatVisitor{}
	v.VisitRecord([]RecordField{
		{Name: "a", Type: testU32Type{}},
		{Name: "b", Type: testU32Type{}},
	})
	if len(v.out) != 1 {
		t.Fatalf("expected 1 composite step, got %d", len(v.out))
	}
	if v.slot != 2 {
		t.Fatalf("expected 2 slots, got %d", v.slot)
	}
	rec := NewValRecord(
		Field{Name: "a", Val: ValU32(10)},
		Field{Name: "b", Val: ValU32(20)},
	)
	gcc := &gocallContext{registers: make([]uint64, 2)}
	runValToFlatStep(t, v, gcc, rec)
	if gcc.registers[0] != 10 || gcc.registers[1] != 20 {
		t.Fatalf("record fields not lowered: %v", gcc.registers)
	}
}

func TestValToFlatTuple(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitTuple([]Type{testU32Type{}, testU32Type{}})
	if len(v.out) != 1 {
		t.Fatalf("expected 1 composite step, got %d", len(v.out))
	}
	tup := NewValRecord(
		Field{Name: "0", Val: ValU32(100)},
		Field{Name: "1", Val: ValU32(200)},
	)
	gcc := &gocallContext{registers: make([]uint64, 2)}
	runValToFlatStep(t, v, gcc, tup)
	if gcc.registers[0] != 100 || gcc.registers[1] != 200 {
		t.Fatalf("tuple elems not lowered: %v", gcc.registers)
	}
}

func TestValToFlatFlags(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitFlags([]string{"a", "b", "c", "d", "e"})
	flags := NewValFlags([]string{"a", "b", "c", "d", "e"}, "a", "b", "c")
	gcc := &gocallContext{registers: make([]uint64, 1)}
	runValToFlatStep(t, v, gcc, flags)
	if gcc.registers[0] != 0x7 {
		t.Fatalf("flags low-3: got %x, want 0x7", gcc.registers[0])
	}
}

func TestValToFlatFlagsMasksAboveNumLabels(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitFlags([]string{"a", "b", "c"})
	flags := newValFlagsFromBits(0xFF)
	gcc := &gocallContext{registers: make([]uint64, 1)}
	runValToFlatStep(t, v, gcc, flags)
	if gcc.registers[0] != 0x7 {
		t.Fatalf("flags mask above numLabels: got %x, want 0x7", gcc.registers[0])
	}
}

func TestValToFlatEnumValid(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitEnum(3)
	gcc := &gocallContext{registers: make([]uint64, 1)}
	runValToFlatStep(t, v, gcc, NewValEnum(1))
	if gcc.registers[0] != 1 {
		t.Fatalf("enum valid: got %x", gcc.registers[0])
	}
}

func TestValToFlatEnumOutOfRange(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitEnum(3)
	gcc := &gocallContext{registers: make([]uint64, 1)}
	err := v.out[0](context.Background(), gcc, NewValEnum(5), 0)
	if err == nil {
		t.Fatal("expected error for out-of-range enum disc")
	}
}

func TestValToFlatString(t *testing.T) {
	mem := setupMem(t, 256)
	realloc, _ := bumpRealloc(64)

	v := &valToFlatVisitor{}
	v.VisitString()
	if v.slot != 2 {
		t.Fatalf("string slots=%d, want 2", v.slot)
	}

	gcc := &gocallContext{
		callee: &transferSide{
			Memory:         mem,
			Realloc:        realloc,
			StringEncoding: EncUTF8,
		},
		registers: make([]uint64, 2),
	}
	runValToFlatStep(t, v, gcc, ValString("hi!"))
	ptr := uint32(gcc.registers[0])
	ln := uint32(gcc.registers[1])
	if ln != 3 {
		t.Fatalf("string len=%d, want 3", ln)
	}
	bytes, _ := mem.Read(ptr, ln)
	if string(bytes) != "hi!" {
		t.Fatalf("string bytes=%q, want hi!", bytes)
	}
}

func TestValToFlatList(t *testing.T) {
	mem := setupMem(t, 256)
	realloc, _ := bumpRealloc(64)

	v := &valToFlatVisitor{}
	v.VisitList(testU32Type{})
	if v.slot != 2 {
		t.Fatalf("list slots=%d, want 2 (ptr+len)", v.slot)
	}

	list := NewValListOf[ValU32](ValU32(10), ValU32(20), ValU32(30))
	gcc := &gocallContext{
		callee: &transferSide{
			Memory:         mem,
			Realloc:        realloc,
			StringEncoding: EncUTF8,
		},
		registers: make([]uint64, 2),
	}
	runValToFlatStep(t, v, gcc, list)
	ptr := uint32(gcc.registers[0])
	ln := uint32(gcc.registers[1])
	if ln != 3 {
		t.Fatalf("list len=%d, want 3", ln)
	}
	for i := range ln {
		got, _ := mem.ReadUint32Le(ptr + i*4)
		want := (i + 1) * 10
		if got != want {
			t.Fatalf("list[%d]=%d, want %d", i, got, want)
		}
	}
}

func TestValToFlatVariantEmptyCase(t *testing.T) {
	// Variant: case 0 = empty, case 1 = u32 payload.
	// Expect slots = 1 (disc) + 1 (joined payload) = 2.
	v := &valToFlatVisitor{}
	v.VisitVariant([]VariantCase{
		{Name: "empty"},
		{Name: "u32", Payload: testU32Type{}},
	})
	if v.slot != 2 {
		t.Fatalf("variant slots=%d, want 2", v.slot)
	}

	// Lower case 0 (no payload): disc=0, joined slot must be zeroed.
	variant := NewValVariant(0, nil)
	gcc := &gocallContext{registers: []uint64{0xDEAD, 0xDEAD}}
	runValToFlatStep(t, v, gcc, variant)
	if gcc.registers[0] != 0 {
		t.Fatalf("disc=%x, want 0", gcc.registers[0])
	}
	if gcc.registers[1] != 0 {
		t.Fatalf("unused payload slot not zeroed: got %x", gcc.registers[1])
	}
}

func TestValToFlatVariantWithPayload(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitVariant([]VariantCase{
		{Name: "empty"},
		{Name: "u32", Payload: testU32Type{}},
	})

	variant := NewValVariant(1, ValU32(77))
	gcc := &gocallContext{registers: []uint64{0, 0}}
	runValToFlatStep(t, v, gcc, variant)
	if gcc.registers[0] != 1 {
		t.Fatalf("disc=%d, want 1", gcc.registers[0])
	}
	if gcc.registers[1] != 77 {
		t.Fatalf("payload=%d, want 77", gcc.registers[1])
	}
}

func TestValToFlatOptionSome(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitOption(testU32Type{})
	if v.slot != 2 {
		t.Fatalf("option slots=%d, want 2", v.slot)
	}
	gcc := &gocallContext{registers: make([]uint64, 2)}
	runValToFlatStep(t, v, gcc, ValOptionSome(ValU32(5)))
	if gcc.registers[0] != 1 {
		t.Fatalf("some disc=%d, want 1", gcc.registers[0])
	}
	if gcc.registers[1] != 5 {
		t.Fatalf("some payload=%d, want 5", gcc.registers[1])
	}
}

func TestValToFlatOptionNone(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitOption(testU32Type{})
	gcc := &gocallContext{registers: []uint64{0xDEAD, 0xDEAD}}
	runValToFlatStep(t, v, gcc, ValOptionNone())
	if gcc.registers[0] != 0 {
		t.Fatalf("none disc=%d, want 0", gcc.registers[0])
	}
	if gcc.registers[1] != 0 {
		t.Fatalf("none payload slot not zeroed: got %x", gcc.registers[1])
	}
}

func TestValToFlatResultOk(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitResult(testU32Type{}, testU32Type{})
	gcc := &gocallContext{registers: make([]uint64, 2)}
	runValToFlatStep(t, v, gcc, ValResultOk(ValU32(100)))
	if gcc.registers[0] != 0 {
		t.Fatalf("ok disc=%d, want 0", gcc.registers[0])
	}
	if gcc.registers[1] != 100 {
		t.Fatalf("ok payload=%d, want 100", gcc.registers[1])
	}
}

func TestValToFlatResultErr(t *testing.T) {
	v := &valToFlatVisitor{}
	v.VisitResult(testU32Type{}, testU32Type{})
	gcc := &gocallContext{registers: make([]uint64, 2)}
	runValToFlatStep(t, v, gcc, ValResultErr(ValU32(200)))
	if gcc.registers[0] != 1 {
		t.Fatalf("err disc=%d, want 1", gcc.registers[0])
	}
	if gcc.registers[1] != 200 {
		t.Fatalf("err payload=%d, want 200", gcc.registers[1])
	}
}

func TestValToFlatOwn(t *testing.T) {
	rt := &testResourceType{name: "R"}
	calleeTbl := newStubResourceTable()

	src := newStubResourceTable()
	live := src.IssueOwn(rt, 42)
	val := liftIntoVal(t, live)

	v := &valToFlatVisitor{}
	v.VisitOwn(rt)
	gcc := &gocallContext{
		callee:    &transferSide{Instance: &testInstance{}, ResourceTable: calleeTbl},
		registers: make([]uint64, 1),
	}
	runValToFlatStep(t, v, gcc, val)

	got, err := calleeTbl.LookupOwn(rt, uint32(gcc.registers[0]))
	if err != nil {
		t.Fatalf("LookupOwn: %v", err)
	}
	if got.Rep() != 42 {
		t.Fatalf("rep = %d, want 42", got.Rep())
	}
}

func TestValToFlatBorrowFromOwn(t *testing.T) {
	rt := &testResourceType{name: "R"}
	calleeTbl := newStubResourceTableFor(&testInstance{name: "callee"})

	src := newStubResourceTable()
	live := src.IssueOwn(rt, 7)
	val := liftIntoVal(t, live)

	v := &valToFlatVisitor{}
	v.VisitBorrow(rt)
	gcc := &gocallContext{
		callee:    &transferSide{Instance: &testInstance{name: "callee"}, ResourceTable: calleeTbl},
		registers: make([]uint64, 1),
	}
	runValToFlatStep(t, v, gcc, val)

	if gcc.Task.NumBorrows != 1 {
		t.Fatalf("Task.NumBorrows = %d, want 1", gcc.Task.NumBorrows)
	}
	got, err := calleeTbl.LookupBorrowable(rt, uint32(gcc.registers[0]))
	if err != nil {
		t.Fatalf("LookupBorrowable: %v", err)
	}
	if got.Rep() != 7 {
		t.Fatalf("borrow rep = %d, want 7", got.Rep())
	}
	if err := got.Drop(context.Background()); err != nil {
		t.Fatalf("borrow.Drop: %v", err)
	}
	if err := gcc.Task.End(); err != nil {
		t.Fatalf("Task.End: %v", err)
	}
}
