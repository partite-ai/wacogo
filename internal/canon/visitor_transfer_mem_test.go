package canon

import (
	"context"
	"testing"
)

// testU8Type is a minimal Type that visits as u8, for layout/padding tests.
type testU8Type struct{}

func (testU8Type) Accept(v TypeVisitor) { v.VisitU8() }

func TestMemTransferU32(t *testing.T) {
	srcMem := setupMem(t, 64)
	dstMem := setupMem(t, 64)

	// Lay out a u32 at src offset 0.
	if !srcMem.WriteUint32Le(0, 0xCAFEBABE) {
		t.Fatalf("failed to seed src memory")
	}

	v := &memTransferVisitor{}
	v.VisitU32()
	if len(v.out) != 1 {
		t.Fatalf("expected 1 step, got %d", len(v.out))
	}
	if v.byteOff != 4 {
		t.Fatalf("byteOff should be 4 after u32, got %d", v.byteOff)
	}
	if v.maxAlign != 4 {
		t.Fatalf("maxAlign should be 4 after u32, got %d", v.maxAlign)
	}

	tc := &transferContext{
		caller: &transferSide{Memory: srcMem},
		callee: &transferSide{Memory: dstMem},
	}
	v.out[0](context.Background(), tc, 0, 0)

	got, _ := dstMem.ReadUint32Le(0)
	if got != 0xCAFEBABE {
		t.Fatalf("dst u32 = %#x, want 0xCAFEBABE", got)
	}
}

func TestMemTransferRecord(t *testing.T) {
	v := &memTransferVisitor{}
	v.VisitRecord([]RecordField{
		{Name: "a", Type: testU32Type{}},
		{Name: "b", Type: testU32Type{}},
	})
	if len(v.out) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(v.out))
	}
	if v.byteOff != 8 {
		t.Fatalf("byteOff should be 8, got %d", v.byteOff)
	}
	if v.maxAlign != 4 {
		t.Fatalf("maxAlign should be 4, got %d", v.maxAlign)
	}

	srcMem := setupMem(t, 64)
	dstMem := setupMem(t, 64)
	_ = srcMem.WriteUint32Le(0, 0x11111111)
	_ = srcMem.WriteUint32Le(4, 0x22222222)

	tc := &transferContext{
		caller: &transferSide{Memory: srcMem},
		callee: &transferSide{Memory: dstMem},
	}
	for _, step := range v.out {
		step(context.Background(), tc, 0, 0)
	}

	a, _ := dstMem.ReadUint32Le(0)
	b, _ := dstMem.ReadUint32Le(4)
	if a != 0x11111111 || b != 0x22222222 {
		t.Fatalf("dst record = (%#x, %#x); want (0x11111111, 0x22222222)", a, b)
	}
}

func TestMemTransferU8ThenU32Padding(t *testing.T) {
	// Record{u8, u32}: u8 at 0, pad to 4, u32 at 4; trailing pad to 8.
	v := &memTransferVisitor{}
	v.VisitRecord([]RecordField{
		{Name: "a", Type: testU8Type{}},
		{Name: "b", Type: testU32Type{}},
	})
	if v.byteOff != 8 {
		t.Fatalf("expected byteOff 8 (record padded to maxAlign), got %d", v.byteOff)
	}
	if v.maxAlign != 4 {
		t.Fatalf("expected maxAlign 4, got %d", v.maxAlign)
	}

	srcMem := setupMem(t, 64)
	dstMem := setupMem(t, 64)
	_ = srcMem.WriteByte(0, 0xAB)
	_ = srcMem.WriteUint32Le(4, 0xDEADBEEF)

	tc := &transferContext{
		caller: &transferSide{Memory: srcMem},
		callee: &transferSide{Memory: dstMem},
	}
	for _, step := range v.out {
		step(context.Background(), tc, 0, 0)
	}

	gotU8, _ := dstMem.ReadByte(0)
	gotU32, _ := dstMem.ReadUint32Le(4)
	if gotU8 != 0xAB {
		t.Fatalf("dst u8 = %#x, want 0xAB", gotU8)
	}
	if gotU32 != 0xDEADBEEF {
		t.Fatalf("dst u32 (at offset 4) = %#x, want 0xDEADBEEF", gotU32)
	}
}

func TestMemTransferVariantDiscSizes(t *testing.T) {
	// Small variant: 3 no-payload cases → disc size 1.
	{
		v := &memTransferVisitor{}
		cases := []VariantCase{{Name: "a"}, {Name: "b"}, {Name: "c"}}
		v.VisitVariant(cases)
		if v.maxAlign != 1 {
			t.Fatalf("small variant: expected maxAlign=1, got %d", v.maxAlign)
		}
		if v.byteOff != 1 {
			t.Fatalf("small variant: expected byteOff=1 (just the 1-byte disc), got %d", v.byteOff)
		}
	}
	// Medium variant: 300 no-payload cases → disc size 2.
	{
		v := &memTransferVisitor{}
		cases := make([]VariantCase, 300)
		for i := range cases {
			cases[i] = VariantCase{Name: "c"}
		}
		v.VisitVariant(cases)
		if v.maxAlign != 2 {
			t.Fatalf("medium variant: expected maxAlign=2, got %d", v.maxAlign)
		}
		if v.byteOff != 2 {
			t.Fatalf("medium variant: expected byteOff=2 (2-byte disc), got %d", v.byteOff)
		}
	}
}

func TestMemTransferFlagsSizes(t *testing.T) {
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
		v := &memTransferVisitor{}
		v.VisitFlags(labels)
		if v.byteOff != c.wantSize {
			t.Errorf("n=%d: byteOff=%d, want %d", c.n, v.byteOff, c.wantSize)
		}
		if v.maxAlign != c.wantSize {
			t.Errorf("n=%d: maxAlign=%d, want %d", c.n, v.maxAlign, c.wantSize)
		}
	}
}

func TestMemTransferFlagsRoundtripSmall(t *testing.T) {
	srcMem := setupMem(t, 64)
	dstMem := setupMem(t, 64)
	// 3 labels → 1-byte storage. Seed only the LSB; upper bytes must not
	// be read (out-of-region) and must not be written.
	if !srcMem.WriteByte(0, 0x05) {
		t.Fatal("seed src")
	}
	// Pre-fill dst beyond the 1-byte region with a sentinel; the write must
	// not clobber it.
	if !dstMem.WriteByte(1, 0xAA) {
		t.Fatal("seed dst sentinel")
	}

	v := &memTransferVisitor{}
	v.VisitFlags([]string{"a", "b", "c"})
	tc := &transferContext{
		caller: &transferSide{Memory: srcMem},
		callee: &transferSide{Memory: dstMem},
	}
	v.out[0](context.Background(), tc, 0, 0)

	got, _ := dstMem.ReadByte(0)
	if got != 0x05 {
		t.Fatalf("dst flags byte = %#x, want 0x05", got)
	}
	sentinel, _ := dstMem.ReadByte(1)
	if sentinel != 0xAA {
		t.Fatalf("write clobbered byte past flags region: %#x, want 0xAA", sentinel)
	}
}

func TestMemTransferBool(t *testing.T) {
	srcMem := setupMem(t, 64)
	dstMem := setupMem(t, 64)
	_ = srcMem.WriteByte(0, 0x7F) // non-zero → should normalize to 1

	v := &memTransferVisitor{}
	v.VisitBool()
	tc := &transferContext{
		caller: &transferSide{Memory: srcMem},
		callee: &transferSide{Memory: dstMem},
	}
	v.out[0](context.Background(), tc, 0, 0)

	got, _ := dstMem.ReadByte(0)
	if got != 1 {
		t.Fatalf("bool normalize: dst=%#x, want 1", got)
	}
}

func TestMemTransferVisitor_VisitBorrow_AllocatesCalleeEntry(t *testing.T) {
	// Use a callee table with a different owner than the defining instance,
	// so the same-component shortcut does not fire.
	defining := &testInstance{name: "defining"}
	rtWithInst := &resTypeWithInst{name: "R", inst: defining}
	calleeOwner := &testInstance{name: "callee"}
	callerTable := newStubResourceTable()
	calleeTable := newStubResourceTableFor(calleeOwner) // owner != defining → no shortcut
	callerH := callerTable.IssueOwn(rtWithInst, 99).HandleID()

	srcMem := setupMem(t, 64)
	dstMem := setupMem(t, 64)
	if !srcMem.WriteUint32Le(0, callerH) {
		t.Fatalf("failed to write caller handle to src memory")
	}

	tc := &transferContext{
		caller: &transferSide{Memory: srcMem, ResourceTable: callerTable},
		callee: &transferSide{Memory: dstMem, ResourceTable: calleeTable},
	}
	v := &memTransferVisitor{}
	v.VisitBorrow(rtWithInst)
	if len(v.out) != 1 {
		t.Fatalf("expected 1 step, got %d", len(v.out))
	}
	v.out[0](context.Background(), tc, 0, 0)

	calleeH, ok := dstMem.ReadUint32Le(0)
	if !ok || calleeH == 0 {
		t.Fatal("expected callee borrow handle in dst memory")
	}
	h, err := calleeTable.LookupBorrowable(rtWithInst, calleeH)
	if err != nil {
		t.Fatalf("LookupBorrowable on callee: %v", err)
	}
	if rep := h.Rep(); rep != 99 {
		t.Errorf("Rep on callee: rep=%d want 99", rep)
	}
	if tc.Task.NumBorrows != 1 {
		t.Errorf("NumBorrows: got %d want 1", tc.Task.NumBorrows)
	}
}

func TestMemTransferVisitor_VisitBorrow_SameComponentShortcut(t *testing.T) {
	// The same-component shortcut now fires inside LendTo when
	// calleeTable.owner == rt.DefiningInstance().
	defining := &testInstance{name: "defining"}
	rt := &resTypeWithInst{name: "R", inst: defining}
	callerTable := newStubResourceTable()
	calleeTable := newStubResourceTableFor(defining) // owner == defining → shortcut!
	callerH := callerTable.IssueOwn(rt, 99).HandleID()

	srcMem := setupMem(t, 64)
	dstMem := setupMem(t, 64)
	if !srcMem.WriteUint32Le(0, callerH) {
		t.Fatalf("failed to write caller handle to src memory")
	}

	tc := &transferContext{
		caller: &transferSide{Memory: srcMem, ResourceTable: callerTable},
		callee: &transferSide{Memory: dstMem, ResourceTable: calleeTable},
	}
	v := &memTransferVisitor{}
	v.VisitBorrow(rt)
	v.out[0](context.Background(), tc, 0, 0)

	written, ok := dstMem.ReadUint32Le(0)
	if !ok {
		t.Fatal("failed to read dst memory")
	}
	if written != 99 {
		t.Errorf("expected rep 99 in dst memory (shortcut), got %d", written)
	}
	// No callee-side entry should be created.
	if _, err := calleeTable.LookupBorrowable(rt, 1); err == nil {
		t.Errorf("expected no callee entry under shortcut")
	}
	if tc.Task.NumBorrows != 0 {
		t.Errorf("NumBorrows: got %d want 0 (shortcut)", tc.Task.NumBorrows)
	}
}
