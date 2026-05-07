package canon

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// memTransferVisitor emits transferPlanStep closures for component→component
// transfers in mem (memory-to-memory) mode. Byte offsets are baked into
// each closure at compile time relative to the enclosing memory region's
// origin; at run time, the srcBase and dstBase closure args supply those
// origins (caller-side read address and callee-side write address).
type memTransferVisitor struct {
	byteOff  uint32
	maxAlign uint32
	out      []transferPlanStep
}

func (v *memTransferVisitor) assignBytes(size, align uint32) uint32 {
	v.byteOff = alignUp(v.byteOff, align)
	off := v.byteOff
	v.byteOff += size
	if align > v.maxAlign {
		v.maxAlign = align
	}
	return off
}

func (v *memTransferVisitor) emit(s transferPlanStep) { v.out = append(v.out, s) }

// Primitives: each closure reads from tc.caller.Memory[srcBase+off] and
// writes to tc.callee.Memory[dstBase+off]. Signed sub-word values use the
// raw-byte read/write (bit pattern passes through; sign interpretation is
// the reader's concern).

func (v *memTransferVisitor) VisitU8() {
	off := v.assignBytes(1, 1)
	v.emit(func(_ context.Context, tc *transferContext, srcBase, dstBase uint32) {
		b, ok := tc.caller.Memory.ReadByte(srcBase + off)
		if !ok {
			panic(&Trap{msg: "oob u8 read"})
		}
		if !tc.callee.Memory.WriteByte(dstBase+off, b) {
			panic(&Trap{msg: "oob u8 write"})
		}
	})
}

func (v *memTransferVisitor) VisitU16() {
	off := v.assignBytes(2, 2)
	v.emit(func(_ context.Context, tc *transferContext, srcBase, dstBase uint32) {
		u, ok := tc.caller.Memory.ReadUint16Le(srcBase + off)
		if !ok {
			panic(&Trap{msg: "oob u16 read"})
		}
		if !tc.callee.Memory.WriteUint16Le(dstBase+off, u) {
			panic(&Trap{msg: "oob u16 write"})
		}
	})
}

func (v *memTransferVisitor) VisitU32() {
	off := v.assignBytes(4, 4)
	v.emit(func(_ context.Context, tc *transferContext, srcBase, dstBase uint32) {
		u, ok := tc.caller.Memory.ReadUint32Le(srcBase + off)
		if !ok {
			panic(&Trap{msg: "oob u32 read"})
		}
		if !tc.callee.Memory.WriteUint32Le(dstBase+off, u) {
			panic(&Trap{msg: "oob u32 write"})
		}
	})
}

func (v *memTransferVisitor) VisitU64() {
	off := v.assignBytes(8, 8)
	v.emit(func(_ context.Context, tc *transferContext, srcBase, dstBase uint32) {
		u, ok := tc.caller.Memory.ReadUint64Le(srcBase + off)
		if !ok {
			panic(&Trap{msg: "oob u64 read"})
		}
		if !tc.callee.Memory.WriteUint64Le(dstBase+off, u) {
			panic(&Trap{msg: "oob u64 write"})
		}
	})
}

// Signed sub-words: bit pattern pass-through (reader interprets as signed).
func (v *memTransferVisitor) VisitS8()  { v.VisitU8() }
func (v *memTransferVisitor) VisitS16() { v.VisitU16() }
func (v *memTransferVisitor) VisitS32() { v.VisitU32() }
func (v *memTransferVisitor) VisitS64() { v.VisitU64() }

// Floats: same as raw 32/64-bit read/write (wazero's Le helpers handle endianness).
func (v *memTransferVisitor) VisitF32() { v.VisitU32() }
func (v *memTransferVisitor) VisitF64() { v.VisitU64() }

func (v *memTransferVisitor) VisitBool() {
	off := v.assignBytes(1, 1)
	v.emit(func(_ context.Context, tc *transferContext, srcBase, dstBase uint32) {
		b, ok := tc.caller.Memory.ReadByte(srcBase + off)
		if !ok {
			panic(&Trap{msg: "oob bool read"})
		}
		out := uint8(0)
		if b != 0 {
			out = 1
		}
		if !tc.callee.Memory.WriteByte(dstBase+off, out) {
			panic(&Trap{msg: "oob bool write"})
		}
	})
}

func (v *memTransferVisitor) VisitChar() {
	off := v.assignBytes(4, 4)
	v.emit(func(_ context.Context, tc *transferContext, srcBase, dstBase uint32) {
		u, ok := tc.caller.Memory.ReadUint32Le(srcBase + off)
		if !ok {
			panic(&Trap{msg: "oob char read"})
		}
		mustTransfer(validateChar(rune(u)))
		if !tc.callee.Memory.WriteUint32Le(dstBase+off, u) {
			panic(&Trap{msg: "oob char write"})
		}
	})
}

func (v *memTransferVisitor) VisitString() {
	ptrOff := v.assignBytes(4, 4)
	lenOff := v.assignBytes(4, 4)
	v.emit(func(ctx context.Context, tc *transferContext, srcBase, dstBase uint32) {
		srcPtr, ok := tc.caller.Memory.ReadUint32Le(srcBase + ptrOff)
		if !ok {
			panic(&Trap{msg: "oob string ptr read"})
		}
		srcCoded, ok := tc.caller.Memory.ReadUint32Le(srcBase + lenOff)
		if !ok {
			panic(&Trap{msg: "oob string len read"})
		}
		dstPtr, outCoded := transferStringContent(ctx, tc, srcPtr, srcCoded)
		if !tc.callee.Memory.WriteUint32Le(dstBase+ptrOff, dstPtr) {
			panic(&Trap{msg: "oob string ptr write"})
		}
		if !tc.callee.Memory.WriteUint32Le(dstBase+lenOff, outCoded) {
			panic(&Trap{msg: "oob string len write"})
		}
	})
}

func (v *memTransferVisitor) VisitList(elem Type) {
	ptrOff := v.assignBytes(4, 4)
	lenOff := v.assignBytes(4, 4)

	child := &memTransferVisitor{}
	elem.Accept(child)
	elemSize := alignUp(child.byteOff, child.maxAlign)
	elemAlign := child.maxAlign
	if elemAlign == 0 {
		elemAlign = 1
	}
	subSteps := child.out

	v.emit(func(ctx context.Context, tc *transferContext, srcBase, dstBase uint32) {
		srcPtr, ok := tc.caller.Memory.ReadUint32Le(srcBase + ptrOff)
		if !ok {
			panic(&Trap{msg: "oob list ptr read"})
		}
		n, ok := tc.caller.Memory.ReadUint32Le(srcBase + lenOff)
		if !ok {
			panic(&Trap{msg: "oob list len read"})
		}
		dstPtr, outN := transferListContent(ctx, tc, srcPtr, n, elemSize, elemAlign, subSteps)
		if !tc.callee.Memory.WriteUint32Le(dstBase+ptrOff, dstPtr) {
			panic(&Trap{msg: "oob list ptr write"})
		}
		if !tc.callee.Memory.WriteUint32Le(dstBase+lenOff, outN) {
			panic(&Trap{msg: "oob list len write"})
		}
	})
}

func (v *memTransferVisitor) VisitRecord(fields []RecordField) {
	for _, f := range fields {
		f.Type.Accept(v)
	}
	// Pad trailing record alignment up to maxAlign for correct layout.
	v.byteOff = alignUp(v.byteOff, v.maxAlign)
}

func (v *memTransferVisitor) VisitTuple(types []Type) {
	for _, t := range types {
		t.Accept(v)
	}
	v.byteOff = alignUp(v.byteOff, v.maxAlign)
}

func (v *memTransferVisitor) VisitFlags(names []string) {
	numLabels := uint32(len(names))
	mustTransfer(validateFlagsLabelCount(numLabels))
	size := flagsByteSize(numLabels)
	off := v.assignBytes(size, size)
	v.emit(func(_ context.Context, tc *transferContext, srcBase, dstBase uint32) {
		bits, err := readFlagsBits(tc.caller.Memory, srcBase+off, size)
		if err != nil {
			panic(&Trap{msg: err.Error()})
		}
		if numLabels < 32 {
			bits &= (uint32(1) << numLabels) - 1
		}
		if err := writeFlagsBits(tc.callee.Memory, dstBase+off, size, bits); err != nil {
			panic(&Trap{msg: err.Error()})
		}
	})
}

func (v *memTransferVisitor) VisitOwn(rt ResourceType) {
	off := v.assignBytes(4, 4)
	v.emit(func(ctx context.Context, tc *transferContext, srcBase, dstBase uint32) {
		h, ok := tc.caller.Memory.ReadUint32Le(srcBase + off)
		if !ok {
			panic(&Trap{msg: "oob own read"})
		}
		srcH, err := tc.caller.ResourceTable.LookupOwn(rt, h)
		if err != nil {
			panic(&Trap{msg: err.Error()})
		}
		dstH, err := srcH.TransferOwn(tc.callee.ResourceTable)
		if err != nil {
			panic(&Trap{msg: err.Error()})
		}
		if !tc.callee.Memory.WriteUint32Le(dstBase+off, dstH.HandleID()) {
			panic(&Trap{msg: "oob own write"})
		}
	})
}

func (v *memTransferVisitor) VisitBorrow(rt ResourceType) {
	off := v.assignBytes(4, 4)
	v.emit(func(ctx context.Context, tc *transferContext, srcBase, dstBase uint32) {
		h, ok := tc.caller.Memory.ReadUint32Le(srcBase + off)
		if !ok {
			panic(&Trap{msg: "oob borrow read"})
		}
		srcH, err := tc.caller.ResourceTable.LookupBorrowable(rt, h)
		if err != nil {
			panic(&Trap{msg: err.Error()})
		}
		dstH, err := srcH.LendTo(tc.callee.ResourceTable, &tc.Task)
		if err != nil {
			panic(&Trap{msg: err.Error()})
		}
		if !tc.callee.Memory.WriteUint32Le(dstBase+off, dstH.HandleID()) {
			panic(&Trap{msg: "oob borrow write"})
		}
	})
}

// discByteSize returns the byte size of an enum/variant discriminant based
// on its case count (1/2/4 bytes).
func discByteSize(numCases uint32) uint32 {
	switch {
	case numCases <= 1<<8:
		return 1
	case numCases <= 1<<16:
		return 2
	default:
		return 4
	}
}

func readDisc(m api.Memory, off, size uint32) uint32 {
	switch size {
	case 1:
		b, ok := m.ReadByte(off)
		if !ok {
			panic(&Trap{msg: "oob disc read"})
		}
		return uint32(b)
	case 2:
		u, ok := m.ReadUint16Le(off)
		if !ok {
			panic(&Trap{msg: "oob disc read"})
		}
		return uint32(u)
	case 4:
		u, ok := m.ReadUint32Le(off)
		if !ok {
			panic(&Trap{msg: "oob disc read"})
		}
		return u
	}
	panic(&Trap{msg: "unknown disc size"})
}

func writeDisc(m api.Memory, off, size, val uint32) {
	switch size {
	case 1:
		if !m.WriteByte(off, uint8(val)) {
			panic(&Trap{msg: "oob disc write"})
		}
	case 2:
		if !m.WriteUint16Le(off, uint16(val)) {
			panic(&Trap{msg: "oob disc write"})
		}
	case 4:
		if !m.WriteUint32Le(off, val) {
			panic(&Trap{msg: "oob disc write"})
		}
	default:
		panic(&Trap{msg: "unknown disc size"})
	}
}

func readFlagsBits(m api.Memory, off, size uint32) (uint32, error) {
	switch size {
	case 1:
		b, ok := m.ReadByte(off)
		if !ok {
			return 0, fmt.Errorf("oob flags read")
		}
		return uint32(b), nil
	case 2:
		u, ok := m.ReadUint16Le(off)
		if !ok {
			return 0, fmt.Errorf("oob flags read")
		}
		return uint32(u), nil
	case 4:
		u, ok := m.ReadUint32Le(off)
		if !ok {
			return 0, fmt.Errorf("oob flags read")
		}
		return u, nil
	}
	return 0, fmt.Errorf("unknown flags size %d", size)
}

func writeFlagsBits(m api.Memory, off, size, bits uint32) error {
	switch size {
	case 1:
		if !m.WriteByte(off, uint8(bits)) {
			return fmt.Errorf("oob flags write")
		}
	case 2:
		if !m.WriteUint16Le(off, uint16(bits)) {
			return fmt.Errorf("oob flags write")
		}
	case 4:
		if !m.WriteUint32Le(off, bits) {
			return fmt.Errorf("oob flags write")
		}
	default:
		return fmt.Errorf("unknown flags size %d", size)
	}
	return nil
}

func (v *memTransferVisitor) VisitEnum(numCases uint32) {
	size := discByteSize(numCases)
	off := v.assignBytes(size, size)
	v.emit(func(_ context.Context, tc *transferContext, srcBase, dstBase uint32) {
		disc := readDisc(tc.caller.Memory, srcBase+off, size)
		if disc >= numCases {
			panic(&Trap{msg: "enum: out-of-range discriminant"})
		}
		writeDisc(tc.callee.Memory, dstBase+off, size, disc)
	})
}

func (v *memTransferVisitor) VisitVariant(cases []VariantCase) {
	numCases := uint32(len(cases))
	discSize := discByteSize(numCases)

	// Compile per-case sub-plans, measure max payload size/align.
	type caseEntry struct {
		steps []transferPlanStep
	}
	entries := make([]caseEntry, len(cases))
	var maxSize, maxPayloadAlign uint32
	for i, c := range cases {
		if c.Payload == nil {
			continue
		}
		child := &memTransferVisitor{}
		c.Payload.Accept(child)
		sz := alignUp(child.byteOff, child.maxAlign)
		if sz > maxSize {
			maxSize = sz
		}
		if child.maxAlign > maxPayloadAlign {
			maxPayloadAlign = child.maxAlign
		}
		entries[i] = caseEntry{child.out}
	}
	// Per canonical-ABI spec, the variant's overall alignment is
	// max(disc-align, max payload alignment). Aligning the disc to that
	// ensures the variant starts on a properly-aligned boundary in any
	// enclosing record/list/etc.
	overallAlign := discSize
	if maxPayloadAlign > overallAlign {
		overallAlign = maxPayloadAlign
	}
	discOff := v.assignBytes(discSize, overallAlign)
	if maxPayloadAlign > 0 {
		v.byteOff = alignUp(v.byteOff, maxPayloadAlign)
	}
	payloadOff := v.byteOff
	memPayloadOffset := payloadOff - discOff
	v.byteOff += maxSize

	v.emit(func(ctx context.Context, tc *transferContext, srcBase, dstBase uint32) {
		disc := readDisc(tc.caller.Memory, srcBase+discOff, discSize)
		if disc >= numCases {
			panic(&Trap{msg: fmt.Sprintf("invalid variant discriminant %d", disc)})
		}
		writeDisc(tc.callee.Memory, dstBase+discOff, discSize, disc)
		if entries[disc].steps == nil {
			return
		}
		subSrc := srcBase + discOff + memPayloadOffset
		subDst := dstBase + discOff + memPayloadOffset
		for _, step := range entries[disc].steps {
			step(ctx, tc, subSrc, subDst)
		}
	})
}

func (v *memTransferVisitor) VisitOption(inner Type) {
	v.VisitVariant([]VariantCase{
		{Name: "none"},
		{Name: "some", Payload: inner},
	})
}

func (v *memTransferVisitor) VisitResult(ok, err Type) {
	v.VisitVariant([]VariantCase{
		{Name: "ok", Payload: ok},
		{Name: "err", Payload: err},
	})
}
