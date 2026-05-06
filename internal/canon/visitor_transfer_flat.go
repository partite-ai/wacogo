package canon

import (
	"context"
	"fmt"
)

// flatTransferVisitor emits transferPlanStep closures for component→component
// transfers in flat (register-to-register) mode. Slot indices are tracked
// absolutely and baked into each emitted closure; base is ignored.
type flatTransferVisitor struct {
	slot uint32 // current absolute slot cursor
	out  []transferPlanStep
}

func (v *flatTransferVisitor) assignSlot() uint32 {
	s := v.slot
	v.slot++
	return s
}

// emit appends a step. Kept as a method so future instrumentation (metrics,
// assertion frames) can hook here.
func (v *flatTransferVisitor) emit(s transferPlanStep) {
	v.out = append(v.out, s)
}

// In flat-mode transfer, src and dst registers are the same slice
// (the wazero host-callback stack). Read happens before write, but for
// primitives the transformation is identity or a canonicalize. For bool
// we normalize to 0/1; for char we validate.

func (v *flatTransferVisitor) VisitU8() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, tc *transferContext, _, _ uint32) {
		tc.registers[slot] = uint64(uint8(tc.registers[slot]))
	})
}

func (v *flatTransferVisitor) VisitU16() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, tc *transferContext, _, _ uint32) {
		tc.registers[slot] = uint64(uint16(tc.registers[slot]))
	})
}

func (v *flatTransferVisitor) VisitU32() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, tc *transferContext, _, _ uint32) {
		tc.registers[slot] = uint64(uint32(tc.registers[slot]))
	})
}

func (v *flatTransferVisitor) VisitU64() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, tc *transferContext, _, _ uint32) {
		// u64 fills the entire slot; nothing to mask.
		_ = tc.registers[slot]
	})
}

func (v *flatTransferVisitor) VisitS8() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, tc *transferContext, _, _ uint32) {
		// sign-extend s8 → i32 in the slot (low 32 bits hold sign-extended value)
		tc.registers[slot] = uint64(uint32(int32(int8(tc.registers[slot]))))
	})
}

func (v *flatTransferVisitor) VisitS16() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, tc *transferContext, _, _ uint32) {
		tc.registers[slot] = uint64(uint32(int32(int16(tc.registers[slot]))))
	})
}

func (v *flatTransferVisitor) VisitS32() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, tc *transferContext, _, _ uint32) {
		tc.registers[slot] = uint64(uint32(tc.registers[slot]))
	})
}

func (v *flatTransferVisitor) VisitS64() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, tc *transferContext, _, _ uint32) {
		_ = tc.registers[slot]
	})
}

func (v *flatTransferVisitor) VisitF32() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, tc *transferContext, _, _ uint32) {
		// f32 canonicalization: bit pattern preservation suffices; wazero
		// already delivers canonical NaN-preserving values per wasm semantics.
		tc.registers[slot] = uint64(uint32(tc.registers[slot]))
	})
}

func (v *flatTransferVisitor) VisitF64() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, tc *transferContext, _, _ uint32) {
		_ = tc.registers[slot]
	})
}

func (v *flatTransferVisitor) VisitBool() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, tc *transferContext, _, _ uint32) {
		if tc.registers[slot] != 0 {
			tc.registers[slot] = 1
		}
	})
}

func (v *flatTransferVisitor) VisitChar() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, tc *transferContext, _, _ uint32) {
		cp := uint32(tc.registers[slot])
		mustTransfer(validateChar(rune(cp)))
		tc.registers[slot] = uint64(cp)
	})
}

func (v *flatTransferVisitor) VisitString() {
	ptrSlot := v.assignSlot()
	lenSlot := v.assignSlot()
	v.emit(func(ctx context.Context, tc *transferContext, _, _ uint32) {
		srcPtr := uint32(tc.registers[ptrSlot])
		srcCoded := uint32(tc.registers[lenSlot])
		dstPtr, outCoded := transferStringContent(ctx, tc, srcPtr, srcCoded)
		tc.registers[ptrSlot] = uint64(dstPtr)
		tc.registers[lenSlot] = uint64(outCoded)
	})
}

func (v *flatTransferVisitor) VisitList(elem Type) {
	ptrSlot := v.assignSlot()
	lenSlot := v.assignSlot()

	child := &memTransferVisitor{}
	elem.Accept(child)
	elemSize := alignUp(child.byteOff, child.maxAlign)
	elemAlign := child.maxAlign
	if elemAlign == 0 {
		elemAlign = 1
	}
	subSteps := child.out

	v.emit(func(ctx context.Context, tc *transferContext, _, _ uint32) {
		srcPtr := uint32(tc.registers[ptrSlot])
		n := uint32(tc.registers[lenSlot])
		dstPtr, outN := transferListContent(ctx, tc, srcPtr, n, elemSize, elemAlign, subSteps)
		tc.registers[ptrSlot] = uint64(dstPtr)
		tc.registers[lenSlot] = uint64(outN)
	})
}

func (v *flatTransferVisitor) VisitRecord(fields []RecordField) {
	for _, f := range fields {
		f.Type.Accept(v)
	}
}

func (v *flatTransferVisitor) VisitTuple(types []Type) {
	for _, t := range types {
		t.Accept(v)
	}
}

func (v *flatTransferVisitor) VisitFlags(names []string) {
	numLabels := uint32(len(names))
	mustTransfer(validateFlagsLabelCount(numLabels))
	slot := v.assignSlot()
	v.emit(func(_ context.Context, tc *transferContext, _, _ uint32) {
		bits := uint32(tc.registers[slot])
		if numLabels < 32 {
			bits &= (uint32(1) << numLabels) - 1
		}
		tc.registers[slot] = uint64(bits)
	})
}

func (v *flatTransferVisitor) VisitOwn(rt ResourceType) {
	slot := v.assignSlot()
	v.emit(func(ctx context.Context, tc *transferContext, _, _ uint32) {
		h := uint32(tc.registers[slot])
		srcH, err := tc.caller.ResourceTable.LookupOwn(rt, h)
		if err != nil {
			panic(&Trap{msg: err.Error()})
		}
		dstH, err := srcH.TransferOwn(tc.callee.ResourceTable)
		if err != nil {
			panic(&Trap{msg: err.Error()})
		}
		tc.registers[slot] = uint64(dstH.HandleID())
	})
}

func (v *flatTransferVisitor) VisitBorrow(rt ResourceType) {
	slot := v.assignSlot()
	v.emit(func(ctx context.Context, tc *transferContext, _, _ uint32) {
		h := uint32(tc.registers[slot])
		srcH, err := tc.caller.ResourceTable.LookupBorrowable(rt, h)
		if err != nil {
			panic(&Trap{msg: err.Error()})
		}
		dstH, err := srcH.LendTo(tc.callee.ResourceTable, &tc.Task)
		if err != nil {
			panic(&Trap{msg: err.Error()})
		}
		tc.registers[slot] = uint64(dstH.HandleID())
	})
}

func (v *flatTransferVisitor) VisitEnum(numCases uint32) {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, tc *transferContext, _, _ uint32) {
		disc := uint32(tc.registers[slot])
		if disc >= numCases {
			panic(&Trap{msg: "enum: out-of-range discriminant"})
		}
		tc.registers[slot] = uint64(disc)
	})
}

func (v *flatTransferVisitor) VisitVariant(cases []VariantCase) {
	discSlot := v.assignSlot()
	payloadStart := v.slot

	var joinedSlots uint32
	for _, c := range cases {
		if c.Payload == nil {
			continue
		}
		n := flatCountForType(c.Payload)
		if n > joinedSlots {
			joinedSlots = n
		}
	}
	v.slot = payloadStart + joinedSlots

	type caseEntry struct {
		steps          []transferPlanStep
		caseLocalSlots uint32
	}
	entries := make([]caseEntry, len(cases))
	for i, c := range cases {
		if c.Payload == nil {
			entries[i] = caseEntry{nil, 0}
			continue
		}
		child := &flatTransferVisitor{slot: payloadStart}
		c.Payload.Accept(child)
		entries[i] = caseEntry{child.out, child.slot - payloadStart}
	}

	v.emit(func(ctx context.Context, tc *transferContext, _, _ uint32) {
		disc := uint32(tc.registers[discSlot])
		if int(disc) >= len(entries) {
			panic(&Trap{msg: fmt.Sprintf("invalid variant discriminant %d", disc)})
		}
		for _, step := range entries[disc].steps {
			step(ctx, tc, 0, 0)
		}
		for i := entries[disc].caseLocalSlots; i < joinedSlots; i++ {
			tc.registers[payloadStart+i] = 0
		}
	})
}

func (v *flatTransferVisitor) VisitOption(inner Type) {
	v.VisitVariant([]VariantCase{
		{Name: "none"},
		{Name: "some", Payload: inner},
	})
}

func (v *flatTransferVisitor) VisitResult(ok, err Type) {
	v.VisitVariant([]VariantCase{
		{Name: "ok", Payload: ok},
		{Name: "err", Payload: err},
	})
}
