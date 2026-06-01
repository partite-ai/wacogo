package canon

import (
	"context"
	"testing"
)

func TestFlatTransferU8(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitU8()
	if len(v.out) != 1 {
		t.Fatalf("expected 1 step, got %d", len(v.out))
	}
	tc := &transferContext{registers: []uint64{0x1FF}}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 0xFF {
		t.Fatalf("u8 mask failed: got %x", tc.registers[0])
	}
}

func TestFlatTransferU16(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitU16()
	tc := &transferContext{registers: []uint64{0x1FFFF}}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 0xFFFF {
		t.Fatalf("u16 mask failed: got %x", tc.registers[0])
	}
}

func TestFlatTransferU32(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitU32()
	if len(v.out) != 1 {
		t.Fatalf("expected 1 step, got %d", len(v.out))
	}
	tc := &transferContext{
		registers: []uint64{0xCAFEBABE, 0},
	}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 0xCAFEBABE {
		t.Fatalf("u32 transfer lost value: got %x", tc.registers[0])
	}
}

func TestFlatTransferU32Masks(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitU32()
	tc := &transferContext{registers: []uint64{0xFFFFFFFF_CAFEBABE}}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 0xCAFEBABE {
		t.Fatalf("u32 mask failed: got %x", tc.registers[0])
	}
}

func TestFlatTransferU64(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitU64()
	tc := &transferContext{registers: []uint64{0xDEADBEEFCAFEBABE}}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 0xDEADBEEFCAFEBABE {
		t.Fatalf("u64 passthrough failed: got %x", tc.registers[0])
	}
}

func TestFlatTransferS8SignExtend(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitS8()
	tc := &transferContext{registers: []uint64{0xFF}}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 0xFFFFFFFF {
		t.Fatalf("s8 sign-extend failed: got %x, want %x", tc.registers[0], uint64(0xFFFFFFFF))
	}
}

func TestFlatTransferS8Positive(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitS8()
	tc := &transferContext{registers: []uint64{0x7F}}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 0x7F {
		t.Fatalf("s8 positive failed: got %x", tc.registers[0])
	}
}

func TestFlatTransferS16SignExtend(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitS16()
	tc := &transferContext{registers: []uint64{0xFFFF}}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 0xFFFFFFFF {
		t.Fatalf("s16 sign-extend failed: got %x", tc.registers[0])
	}
}

func TestFlatTransferS16Positive(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitS16()
	tc := &transferContext{registers: []uint64{0x1234}}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 0x1234 {
		t.Fatalf("s16 positive failed: got %x", tc.registers[0])
	}
}

func TestFlatTransferS32(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitS32()
	tc := &transferContext{registers: []uint64{0xFFFFFFFF_DEADBEEF}}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 0xDEADBEEF {
		t.Fatalf("s32 mask failed: got %x", tc.registers[0])
	}
}

func TestFlatTransferS64(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitS64()
	tc := &transferContext{registers: []uint64{0xDEADBEEFCAFEBABE}}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 0xDEADBEEFCAFEBABE {
		t.Fatalf("s64 passthrough failed: got %x", tc.registers[0])
	}
}

func TestFlatTransferF32(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitF32()
	tc := &transferContext{registers: []uint64{0xFFFFFFFF_40490FDB}} // upper bits should be dropped
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 0x40490FDB {
		t.Fatalf("f32 mask failed: got %x", tc.registers[0])
	}
}

func TestFlatTransferF64(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitF64()
	tc := &transferContext{registers: []uint64{0x400921FB54442D18}}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 0x400921FB54442D18 {
		t.Fatalf("f64 passthrough failed: got %x", tc.registers[0])
	}
}

func TestFlatTransferBoolFalse(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitBool()
	tc := &transferContext{registers: []uint64{0}}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 0 {
		t.Fatalf("bool false canonicalize failed: got %x", tc.registers[0])
	}
}

func TestFlatTransferBoolTrue(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitBool()
	tc := &transferContext{registers: []uint64{42}}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 1 {
		t.Fatalf("bool true canonicalize failed: got %x", tc.registers[0])
	}
}

func TestFlatTransferCharValid(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitChar()
	tc := &transferContext{registers: []uint64{'A'}}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 'A' {
		t.Fatalf("char valid failed: got %x", tc.registers[0])
	}
}

func TestFlatTransferCharInvalid(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitChar()
	tc := &transferContext{registers: []uint64{0xD800}} // surrogate
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected trap on surrogate code point")
		}
	}()
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
}

func TestFlatTransferCharOutOfRange(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitChar()
	tc := &transferContext{registers: []uint64{0x110000}}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected trap on out-of-range code point")
		}
	}()
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
}

// testU32Type is a minimal Type for tests; real parent-package Types
// arrive via canon_bridge.
type testU32Type struct{}

func (testU32Type) Accept(v TypeVisitor) { v.VisitU32() }

func TestFlatTransferRecord(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitRecord([]RecordField{
		{Name: "a", Type: testU32Type{}},
		{Name: "b", Type: testU32Type{}},
	})
	if len(v.out) != 2 {
		t.Fatalf("expected 2 steps (a, b); got %d", len(v.out))
	}
}

func TestFlatTransferTuple(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitTuple([]Type{testU32Type{}, testU32Type{}})
	if len(v.out) != 2 {
		t.Fatalf("expected 2 steps; got %d", len(v.out))
	}
}

func TestFlatTransferFlags(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitFlags([]string{"a", "b", "c", "d", "e"})
	tc := &transferContext{registers: []uint64{0xFF}}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	// low 5 bits of 0xFF = 0x1F
	if tc.registers[0] != 0x1F {
		t.Fatalf("flags mask failed: got %x", tc.registers[0])
	}
}

func TestFlatTransferEnumOutOfRange(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitEnum(3)
	tc := &transferContext{registers: []uint64{5}}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected trap on out-of-range enum disc")
		}
	}()
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
}

func TestFlatTransferEnumValid(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitEnum(3)
	tc := &transferContext{registers: []uint64{1}}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})
	if tc.registers[0] != 1 {
		t.Fatalf("enum valid: got %x", tc.registers[0])
	}
}

func TestFlatTransferVariantSlotReservation(t *testing.T) {
	// Variant with two cases: no-payload, and (u32, u32) tuple.
	// Expected flat slots: 1 disc + 2 payload = 3 total.
	v := &flatTransferVisitor{}
	v.VisitVariant([]VariantCase{
		{Name: "empty"},
		{Name: "pair", Payload: pairType{}},
	})
	if v.slot != 3 {
		t.Fatalf("expected 3 slots (disc + 2 joined payload), got %d", v.slot)
	}
}

// pairType is a tuple<u32, u32> for testing.
type pairType struct{}

func (pairType) Accept(v TypeVisitor) {
	v.VisitTuple([]Type{testU32Type{}, testU32Type{}})
}

func TestFlatTransferOptionSlotReservation(t *testing.T) {
	v := &flatTransferVisitor{}
	v.VisitOption(testU32Type{})
	// 1 disc + 1 payload = 2 slots
	if v.slot != 2 {
		t.Fatalf("expected 2 slots, got %d", v.slot)
	}
}

// resTypeWithInst is a testResourceType variant that returns its .inst field
// from DefiningInstance().
type resTypeWithInst struct {
	name string
	inst Instance
}

func (*resTypeWithInst) IsResourceType()                                 {}
func (r *resTypeWithInst) DefiningInstance() Instance                    { return r.inst }
func (*resTypeWithInst) Destructor() func(context.Context, uint32) error { return nil }

func TestFlatTransferVisitor_VisitBorrow_AllocatesCalleeEntry(t *testing.T) {
	// Use a callee table with a different owner than the defining instance,
	// so the same-component shortcut does not fire.
	defining := &testInstance{name: "defining"}
	rtWithInst := &resTypeWithInst{name: "R", inst: defining}
	calleeOwner := &testInstance{name: "callee"}
	callerTable := newStubResourceTable()
	calleeTable := newStubResourceTableFor(calleeOwner) // owner != defining → no shortcut
	callerH := callerTable.IssueOwn(rtWithInst, 99).HandleID()

	tc := &transferContext{
		caller:    &transferSide{ResourceTable: callerTable},
		callee:    &transferSide{ResourceTable: calleeTable},
		registers: []uint64{uint64(callerH)},
	}
	v := &flatTransferVisitor{}
	v.VisitBorrow(rtWithInst)
	if len(v.out) != 1 {
		t.Fatalf("expected 1 step, got %d", len(v.out))
	}
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})

	calleeH := uint32(tc.registers[0])
	if calleeH == 0 {
		t.Fatal("expected callee borrow handle")
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

func TestFlatTransferVisitor_VisitBorrow_SameComponentShortcut(t *testing.T) {
	// The same-component shortcut now fires inside LendTo when
	// calleeTable.owner == rt.DefiningInstance().
	defining := &testInstance{name: "defining"}
	rt := &resTypeWithInst{name: "R", inst: defining}
	callerTable := newStubResourceTable()
	calleeTable := newStubResourceTableFor(defining) // owner == defining → shortcut!
	callerH := callerTable.IssueOwn(rt, 99).HandleID()

	tc := &transferContext{
		caller:    &transferSide{ResourceTable: callerTable},
		callee:    &transferSide{ResourceTable: calleeTable},
		registers: []uint64{uint64(callerH)},
	}
	v := &flatTransferVisitor{}
	v.VisitBorrow(rt)
	v.out[0].transfer(context.Background(), tc, 0, 0, &allocSource{})

	if got := uint32(tc.registers[0]); got != 99 {
		t.Errorf("expected rep 99 in slot 0 (shortcut), got %d", got)
	}
	// No callee-side entry should be created.
	if _, err := calleeTable.LookupBorrowable(rt, 1); err == nil {
		t.Errorf("expected no callee entry under shortcut")
	}
	if tc.Task.NumBorrows != 0 {
		t.Errorf("NumBorrows: got %d want 0 (shortcut)", tc.Task.NumBorrows)
	}
}
