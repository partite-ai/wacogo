package canon

import (
	"context"
	"testing"
)

// bumpRealloc returns a ReallocFunc that hands out successive bump-allocated
// regions starting at base. The cursor is captured by the returned closure
// so tests can share a single allocator across multiple Realloc calls.
func bumpRealloc(base uint32) (ReallocFunc, *uint32) {
	cur := base
	cursor := &cur
	return func(_ context.Context, _, _, align, n uint32) (uint32, error) {
		if align > 1 {
			*cursor = alignUp(*cursor, align)
		}
		out := *cursor
		*cursor += n
		return out, nil
	}, cursor
}

// runValToMemStep runs the single emitted step against gcc with the given
// input Val and base.
func runValToMemStep(t *testing.T, v *valToMemVisitor, gcc *gocallContext, val Val, base uint32) {
	t.Helper()
	if len(v.out) != 1 {
		t.Fatalf("expected 1 step, got %d", len(v.out))
	}
	if err := v.out[0](context.Background(), gcc, val, base); err != nil {
		t.Fatalf("step failed: %v", err)
	}
}

func TestValToMemU32(t *testing.T) {
	mem := setupMem(t, 64)

	v := &valToMemVisitor{}
	v.VisitU32()
	if v.byteOff != 4 {
		t.Fatalf("byteOff=%d, want 4", v.byteOff)
	}
	if v.maxAlign != 4 {
		t.Fatalf("maxAlign=%d, want 4", v.maxAlign)
	}

	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	runValToMemStep(t, v, gcc, ValU32(0xCAFEBABE), 8)
	got, _ := mem.ReadUint32Le(8)
	if got != 0xCAFEBABE {
		t.Fatalf("mem[8]=%x, want 0xCAFEBABE", got)
	}
}

func TestValToMemBool(t *testing.T) {
	mem := setupMem(t, 64)
	v := &valToMemVisitor{}
	v.VisitBool()
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	runValToMemStep(t, v, gcc, ValBool(true), 0)
	got, _ := mem.ReadByte(0)
	if got != 1 {
		t.Fatalf("mem[0]=%d, want 1", got)
	}
}

func TestValToMemRecord(t *testing.T) {
	mem := setupMem(t, 64)
	v := &valToMemVisitor{}
	v.VisitRecord([]RecordField{
		{Name: "a", Type: testU32Type{}},
		{Name: "b", Type: testU32Type{}},
	})
	if v.byteOff != 8 {
		t.Fatalf("byteOff=%d, want 8", v.byteOff)
	}
	if v.maxAlign != 4 {
		t.Fatalf("maxAlign=%d, want 4", v.maxAlign)
	}

	rec := NewValRecord(
		Field{Name: "a", Val: ValU32(0x11111111)},
		Field{Name: "b", Val: ValU32(0x22222222)},
	)
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	runValToMemStep(t, v, gcc, rec, 0)
	a, _ := mem.ReadUint32Le(0)
	b, _ := mem.ReadUint32Le(4)
	if a != 0x11111111 || b != 0x22222222 {
		t.Fatalf("record=(%x,%x), want (0x11111111,0x22222222)", a, b)
	}
}

func TestValToMemRecordU8U32Padding(t *testing.T) {
	v := &valToMemVisitor{}
	v.VisitRecord([]RecordField{
		{Name: "a", Type: testU8Type{}},
		{Name: "b", Type: testU32Type{}},
	})
	if v.byteOff != 8 {
		t.Fatalf("expected byteOff=8 (padded to maxAlign), got %d", v.byteOff)
	}
	if v.maxAlign != 4 {
		t.Fatalf("expected maxAlign=4, got %d", v.maxAlign)
	}
}

func TestValToMemString(t *testing.T) {
	mem := setupMem(t, 256)
	realloc, _ := bumpRealloc(64)
	v := &valToMemVisitor{}
	v.VisitString()
	if v.byteOff != 8 {
		t.Fatalf("byteOff=%d, want 8 (ptr+len)", v.byteOff)
	}

	gcc := &gocallContext{
		callee: &transferSide{
			Memory:         mem,
			Realloc:        realloc,
			StringEncoding: EncUTF8,
		},
	}
	runValToMemStep(t, v, gcc, ValString("hello"), 0)
	ptr, _ := mem.ReadUint32Le(0)
	ln, _ := mem.ReadUint32Le(4)
	if ln != 5 {
		t.Fatalf("string len=%d, want 5", ln)
	}
	bytes, _ := mem.Read(ptr, ln)
	if string(bytes) != "hello" {
		t.Fatalf("string bytes=%q, want hello", bytes)
	}
}

func TestValToMemList(t *testing.T) {
	mem := setupMem(t, 256)
	realloc, _ := bumpRealloc(64)
	v := &valToMemVisitor{}
	v.VisitList(testU32Type{})
	if v.byteOff != 8 {
		t.Fatalf("byteOff=%d, want 8 (ptr+len)", v.byteOff)
	}

	list := NewValListOf[ValU32](ValU32(1), ValU32(2), ValU32(3))
	gcc := &gocallContext{
		callee: &transferSide{
			Memory:         mem,
			Realloc:        realloc,
			StringEncoding: EncUTF8,
		},
	}
	runValToMemStep(t, v, gcc, list, 0)
	ptr, _ := mem.ReadUint32Le(0)
	ln, _ := mem.ReadUint32Le(4)
	if ln != 3 {
		t.Fatalf("list len=%d, want 3", ln)
	}
	for i := range ln {
		got, _ := mem.ReadUint32Le(ptr + i*4)
		if got != i+1 {
			t.Fatalf("list[%d]=%d, want %d", i, got, i+1)
		}
	}
}

func TestValToMemVariantU32Payload(t *testing.T) {
	mem := setupMem(t, 256)
	realloc, _ := bumpRealloc(64)

	v := &valToMemVisitor{}
	v.VisitVariant([]VariantCase{
		{Name: "empty"},
		{Name: "withU32", Payload: testU32Type{}},
	})
	// Disc byte (1) + pad to 4-align + 4 bytes payload = 8
	if v.byteOff != 8 {
		t.Fatalf("variant byteOff=%d, want 8", v.byteOff)
	}
	if v.maxAlign != 4 {
		t.Fatalf("variant maxAlign=%d, want 4", v.maxAlign)
	}

	variant := NewValVariant(1, ValU32(42))
	gcc := &gocallContext{
		callee: &transferSide{
			Memory:         mem,
			Realloc:        realloc,
			StringEncoding: EncUTF8,
		},
	}
	runValToMemStep(t, v, gcc, variant, 0)
	d, _ := mem.ReadByte(0)
	if d != 1 {
		t.Fatalf("variant disc=%d, want 1", d)
	}
	payload, _ := mem.ReadUint32Le(4)
	if payload != 42 {
		t.Fatalf("variant payload=%d, want 42", payload)
	}
}

func TestValToMemOptionSome(t *testing.T) {
	mem := setupMem(t, 256)
	realloc, _ := bumpRealloc(64)

	v := &valToMemVisitor{}
	v.VisitOption(testU32Type{})

	gcc := &gocallContext{
		callee: &transferSide{
			Memory:         mem,
			Realloc:        realloc,
			StringEncoding: EncUTF8,
		},
	}
	runValToMemStep(t, v, gcc, ValOptionSome(ValU32(0xABCD)), 0)
	d, _ := mem.ReadByte(0)
	if d != 1 {
		t.Fatalf("option disc=%d, want 1 (some)", d)
	}
	payload, _ := mem.ReadUint32Le(4)
	if payload != 0xABCD {
		t.Fatalf("option payload=%x, want 0xABCD", payload)
	}
}

func TestValToMemOptionNone(t *testing.T) {
	mem := setupMem(t, 256)
	realloc, _ := bumpRealloc(64)

	v := &valToMemVisitor{}
	v.VisitOption(testU32Type{})

	gcc := &gocallContext{
		callee: &transferSide{
			Memory:         mem,
			Realloc:        realloc,
			StringEncoding: EncUTF8,
		},
	}
	runValToMemStep(t, v, gcc, ValOptionNone(), 0)
	d, _ := mem.ReadByte(0)
	if d != 0 {
		t.Fatalf("option disc=%d, want 0 (none)", d)
	}
}

func TestValToMemResultOk(t *testing.T) {
	mem := setupMem(t, 256)
	realloc, _ := bumpRealloc(64)

	v := &valToMemVisitor{}
	v.VisitResult(testU32Type{}, testU32Type{})

	gcc := &gocallContext{
		callee: &transferSide{
			Memory:         mem,
			Realloc:        realloc,
			StringEncoding: EncUTF8,
		},
	}
	runValToMemStep(t, v, gcc, ValResultOk(ValU32(111)), 0)
	d, _ := mem.ReadByte(0)
	if d != 0 {
		t.Fatalf("result disc=%d, want 0 (ok)", d)
	}
	payload, _ := mem.ReadUint32Le(4)
	if payload != 111 {
		t.Fatalf("result ok payload=%d, want 111", payload)
	}
}

func TestValToMemResultErr(t *testing.T) {
	mem := setupMem(t, 256)
	realloc, _ := bumpRealloc(64)

	v := &valToMemVisitor{}
	v.VisitResult(testU32Type{}, testU32Type{})

	gcc := &gocallContext{
		callee: &transferSide{
			Memory:         mem,
			Realloc:        realloc,
			StringEncoding: EncUTF8,
		},
	}
	runValToMemStep(t, v, gcc, ValResultErr(ValU32(222)), 0)
	d, _ := mem.ReadByte(0)
	if d != 1 {
		t.Fatalf("result disc=%d, want 1 (err)", d)
	}
	payload, _ := mem.ReadUint32Le(4)
	if payload != 222 {
		t.Fatalf("result err payload=%d, want 222", payload)
	}
}

func TestValToMemEnum(t *testing.T) {
	mem := setupMem(t, 64)
	v := &valToMemVisitor{}
	v.VisitEnum(3)
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	runValToMemStep(t, v, gcc, NewValEnum(2), 0)
	d, _ := mem.ReadByte(0)
	if d != 2 {
		t.Fatalf("enum disc=%d, want 2", d)
	}
}

func TestValToMemFlags(t *testing.T) {
	mem := setupMem(t, 64)
	v := &valToMemVisitor{}
	v.VisitFlags([]string{"a", "b", "c", "d", "e"})
	flags := NewValFlags([]string{"a", "b", "c", "d", "e"}, "a", "c")
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	runValToMemStep(t, v, gcc, flags, 0)
	got, _ := mem.ReadUint32Le(0)
	if got != 5 {
		t.Fatalf("flags=%b, want 0b101", got)
	}
}

func TestValToMemFlagsSizes(t *testing.T) {
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
		v := &valToMemVisitor{}
		v.VisitFlags(labels)
		if v.byteOff != c.wantSize {
			t.Errorf("n=%d: byteOff=%d, want %d", c.n, v.byteOff, c.wantSize)
		}
		if v.maxAlign != c.wantSize {
			t.Errorf("n=%d: maxAlign=%d, want %d", c.n, v.maxAlign, c.wantSize)
		}
	}
}

func TestValToMemFlagsSmallWritesOnlyOneByte(t *testing.T) {
	mem := setupMem(t, 64)
	// Pre-fill bytes 1..3 with a sentinel; a 1-byte flags write must
	// leave them untouched.
	_ = mem.WriteByte(1, 0xAA)
	_ = mem.WriteByte(2, 0xBB)
	_ = mem.WriteByte(3, 0xCC)
	v := &valToMemVisitor{}
	v.VisitFlags([]string{"a", "b", "c"})
	flags := NewValFlags([]string{"a", "b", "c"}, "a", "c")
	gcc := &gocallContext{callee: &transferSide{Memory: mem}}
	runValToMemStep(t, v, gcc, flags, 0)
	b0, _ := mem.ReadByte(0)
	b1, _ := mem.ReadByte(1)
	b2, _ := mem.ReadByte(2)
	b3, _ := mem.ReadByte(3)
	if b0 != 0x05 {
		t.Fatalf("flags byte = %#x, want 0x05", b0)
	}
	if b1 != 0xAA || b2 != 0xBB || b3 != 0xCC {
		t.Fatalf("write clobbered bytes past 1-byte flags region: [%#x %#x %#x], want [0xAA 0xBB 0xCC]", b1, b2, b3)
	}
}

func TestValToMemOwn(t *testing.T) {
	rt := &testResourceType{name: "R"}
	calleeTbl := newStubResourceTable()

	src := newStubResourceTable()
	live := src.IssueOwn(rt, 42)
	val := liftIntoVal(t, live)

	mem := setupMem(t, 64)
	v := &valToMemVisitor{}
	v.VisitOwn(rt)
	gcc := &gocallContext{
		callee: &transferSide{Instance: &testInstance{}, Memory: mem, ResourceTable: calleeTbl},
	}
	runValToMemStep(t, v, gcc, val, 0)

	wire, ok := mem.ReadUint32Le(0)
	if !ok {
		t.Fatal("oob read")
	}
	got, err := calleeTbl.LookupOwn(rt, wire)
	if err != nil {
		t.Fatalf("LookupOwn: %v", err)
	}
	if got.Rep() != 42 {
		t.Fatalf("rep = %d, want 42", got.Rep())
	}
}

func TestValToMemBorrowFromOwn(t *testing.T) {
	rt := &testResourceType{name: "R"}
	calleeTbl := newStubResourceTableFor(&testInstance{name: "callee"})

	src := newStubResourceTable()
	live := src.IssueOwn(rt, 7)
	val := liftIntoVal(t, live)

	mem := setupMem(t, 64)
	v := &valToMemVisitor{}
	v.VisitBorrow(rt)
	gcc := &gocallContext{
		callee: &transferSide{Instance: &testInstance{name: "callee"}, Memory: mem, ResourceTable: calleeTbl},
	}
	runValToMemStep(t, v, gcc, val, 0)

	wire, ok := mem.ReadUint32Le(0)
	if !ok {
		t.Fatal("oob read")
	}
	got, err := calleeTbl.LookupBorrowable(rt, wire)
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
