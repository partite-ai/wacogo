package canon

import (
	"context"
	"fmt"
	"math"

	"github.com/tetratelabs/wazero/api"
)

// valToMemVisitor emits gocallLowerStep closures for Go → component calls
// in mem (memory) mode. Each Accept call appends exactly one step; record
// and variant composites wrap their children in a single outer step that
// applies the record/payload offset to the incoming base.
//
// Byte offsets are baked at compile time relative to the enclosing memory
// region's origin; at run time, the step's `base` arg supplies that origin
// in the callee's linear memory. There is no source memory on the Go side
// — data lives in Val form, passed as the `v` step arg.
type valToMemVisitor struct {
	byteOff  uint32
	maxAlign uint32
	out      []gocallLowerStep
}

func (v *valToMemVisitor) assignBytes(size, align uint32) uint32 {
	v.byteOff = alignUp(v.byteOff, align)
	off := v.byteOff
	v.byteOff += size
	if align > v.maxAlign {
		v.maxAlign = align
	}
	return off
}

func (v *valToMemVisitor) emit(s gocallLowerStep) { v.out = append(v.out, s) }

// writeDiscErr mirrors writeDisc but returns an error instead of
// panicking with a Trap, for gocall step closures.
func writeDiscErr(m api.Memory, off, size, disc uint32) error {
	switch size {
	case 1:
		if !m.WriteByte(off, uint8(disc)) {
			return fmt.Errorf("oob disc write")
		}
	case 2:
		if !m.WriteUint16Le(off, uint16(disc)) {
			return fmt.Errorf("oob disc write")
		}
	case 4:
		if !m.WriteUint32Le(off, disc) {
			return fmt.Errorf("oob disc write")
		}
	default:
		return fmt.Errorf("unknown disc size %d", size)
	}
	return nil
}

// readDiscErr mirrors readDisc but returns an error instead of
// panicking with a Trap. Used by memToValVisitor.
func readDiscErr(m api.Memory, off, size uint32) (uint32, error) {
	switch size {
	case 1:
		b, ok := m.ReadByte(off)
		if !ok {
			return 0, fmt.Errorf("oob disc read")
		}
		return uint32(b), nil
	case 2:
		u, ok := m.ReadUint16Le(off)
		if !ok {
			return 0, fmt.Errorf("oob disc read")
		}
		return uint32(u), nil
	case 4:
		u, ok := m.ReadUint32Le(off)
		if !ok {
			return 0, fmt.Errorf("oob disc read")
		}
		return u, nil
	}
	return 0, fmt.Errorf("unknown disc size %d", size)
}

// --- Primitives ---

func (v *valToMemVisitor) VisitBool() {
	off := v.assignBytes(1, 1)
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, base uint32) error {
		b, ok := val.(ValBool)
		if !ok {
			return fmt.Errorf("expected ValBool, got %T", val)
		}
		out := uint8(0)
		if b {
			out = 1
		}
		if !gcc.callee.Memory.WriteByte(base+off, out) {
			return fmt.Errorf("oob bool write")
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitU8() {
	off := v.assignBytes(1, 1)
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, base uint32) error {
		u, ok := val.(ValU8)
		if !ok {
			return fmt.Errorf("expected ValU8, got %T", val)
		}
		if !gcc.callee.Memory.WriteByte(base+off, uint8(u)) {
			return fmt.Errorf("oob u8 write")
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitU16() {
	off := v.assignBytes(2, 2)
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, base uint32) error {
		u, ok := val.(ValU16)
		if !ok {
			return fmt.Errorf("expected ValU16, got %T", val)
		}
		if !gcc.callee.Memory.WriteUint16Le(base+off, uint16(u)) {
			return fmt.Errorf("oob u16 write")
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitU32() {
	off := v.assignBytes(4, 4)
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, base uint32) error {
		u, ok := val.(ValU32)
		if !ok {
			return fmt.Errorf("expected ValU32, got %T", val)
		}
		if !gcc.callee.Memory.WriteUint32Le(base+off, uint32(u)) {
			return fmt.Errorf("oob u32 write")
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitU64() {
	off := v.assignBytes(8, 8)
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, base uint32) error {
		u, ok := val.(ValU64)
		if !ok {
			return fmt.Errorf("expected ValU64, got %T", val)
		}
		if !gcc.callee.Memory.WriteUint64Le(base+off, uint64(u)) {
			return fmt.Errorf("oob u64 write")
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitS8() {
	off := v.assignBytes(1, 1)
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, base uint32) error {
		s, ok := val.(ValS8)
		if !ok {
			return fmt.Errorf("expected ValS8, got %T", val)
		}
		if !gcc.callee.Memory.WriteByte(base+off, uint8(int8(s))) {
			return fmt.Errorf("oob s8 write")
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitS16() {
	off := v.assignBytes(2, 2)
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, base uint32) error {
		s, ok := val.(ValS16)
		if !ok {
			return fmt.Errorf("expected ValS16, got %T", val)
		}
		if !gcc.callee.Memory.WriteUint16Le(base+off, uint16(int16(s))) {
			return fmt.Errorf("oob s16 write")
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitS32() {
	off := v.assignBytes(4, 4)
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, base uint32) error {
		s, ok := val.(ValS32)
		if !ok {
			return fmt.Errorf("expected ValS32, got %T", val)
		}
		if !gcc.callee.Memory.WriteUint32Le(base+off, uint32(int32(s))) {
			return fmt.Errorf("oob s32 write")
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitS64() {
	off := v.assignBytes(8, 8)
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, base uint32) error {
		s, ok := val.(ValS64)
		if !ok {
			return fmt.Errorf("expected ValS64, got %T", val)
		}
		if !gcc.callee.Memory.WriteUint64Le(base+off, uint64(int64(s))) {
			return fmt.Errorf("oob s64 write")
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitF32() {
	off := v.assignBytes(4, 4)
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, base uint32) error {
		f, ok := val.(ValF32)
		if !ok {
			return fmt.Errorf("expected ValF32, got %T", val)
		}
		if !gcc.callee.Memory.WriteUint32Le(base+off, math.Float32bits(float32(f))) {
			return fmt.Errorf("oob f32 write")
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitF64() {
	off := v.assignBytes(8, 8)
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, base uint32) error {
		f, ok := val.(ValF64)
		if !ok {
			return fmt.Errorf("expected ValF64, got %T", val)
		}
		if !gcc.callee.Memory.WriteUint64Le(base+off, math.Float64bits(float64(f))) {
			return fmt.Errorf("oob f64 write")
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitChar() {
	off := v.assignBytes(4, 4)
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, base uint32) error {
		c, ok := val.(ValChar)
		if !ok {
			return fmt.Errorf("expected ValChar, got %T", val)
		}
		if err := validateChar(rune(c)); err != nil {
			return err
		}
		if !gcc.callee.Memory.WriteUint32Le(base+off, uint32(rune(c))) {
			return fmt.Errorf("oob char write")
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitString() {
	ptrOff := v.assignBytes(4, 4)
	lenOff := v.assignBytes(4, 4)
	v.emit(func(ctx context.Context, gcc *gocallContext, val Val, base uint32) error {
		s, ok := val.(ValString)
		if !ok {
			return fmt.Errorf("expected ValString, got %T", val)
		}
		enc := gcc.callee.StringEncoding
		bytes, coded := encodeString(string(s), enc)
		ptr, err := callRealloc(ctx, gcc.callee.Realloc, gcc.callee.Memory, 0, 0, stringAlign(enc), uint32(len(bytes)))
		if err != nil {
			return err
		}
		if !gcc.callee.Memory.Write(ptr, bytes) {
			return fmt.Errorf("oob string bytes write")
		}
		if !gcc.callee.Memory.WriteUint32Le(base+ptrOff, ptr) {
			return fmt.Errorf("oob string ptr write")
		}
		if !gcc.callee.Memory.WriteUint32Le(base+lenOff, coded) {
			return fmt.Errorf("oob string len write")
		}
		return nil
	})
}

// --- Composites ---

// VisitRecord compiles each field via a child visitor starting at byteOff=0,
// reserves the record's contiguous slab in the parent layout, and emits a
// single composite step that forwards fields to their sub-steps with the
// record's base offset applied to the incoming base.
func (v *valToMemVisitor) VisitRecord(fields []RecordField) {
	child := &valToMemVisitor{}
	for _, f := range fields {
		f.Type.Accept(child)
	}
	fieldSteps := child.out
	recordSize := alignUp(child.byteOff, child.maxAlign)
	recordAlign := child.maxAlign
	if recordAlign == 0 {
		recordAlign = 1
	}
	recordOff := v.assignBytes(recordSize, recordAlign)

	v.emit(func(ctx context.Context, gcc *gocallContext, val Val, base uint32) error {
		rec, ok := val.(*ValRecord)
		if !ok {
			return fmt.Errorf("expected *ValRecord, got %T", val)
		}
		recBase := base + recordOff
		for i, step := range fieldSteps {
			if err := step(ctx, gcc, rec.FieldByIndex(i), recBase); err != nil {
				return err
			}
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitTuple(types []Type) {
	child := &valToMemVisitor{}
	for _, t := range types {
		t.Accept(child)
	}
	fieldSteps := child.out
	recordSize := alignUp(child.byteOff, child.maxAlign)
	recordAlign := child.maxAlign
	if recordAlign == 0 {
		recordAlign = 1
	}
	recordOff := v.assignBytes(recordSize, recordAlign)

	v.emit(func(ctx context.Context, gcc *gocallContext, val Val, base uint32) error {
		rec, ok := val.(*ValRecord)
		if !ok {
			return fmt.Errorf("expected *ValRecord for tuple, got %T", val)
		}
		recBase := base + recordOff
		for i, step := range fieldSteps {
			if err := step(ctx, gcc, rec.FieldByIndex(i), recBase); err != nil {
				return err
			}
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitList(elem Type) {
	ptrOff := v.assignBytes(4, 4)
	lenOff := v.assignBytes(4, 4)

	child := &valToMemVisitor{}
	elem.Accept(child)
	elemSize := alignUp(child.byteOff, child.maxAlign)
	elemAlign := child.maxAlign
	if elemAlign == 0 {
		elemAlign = 1
	}
	elemStep := child.out[0]

	v.emit(func(ctx context.Context, gcc *gocallContext, val Val, base uint32) error {
		list, ok := val.(*ValList)
		if !ok {
			return fmt.Errorf("expected *ValList, got %T", val)
		}
		n := uint32(list.Len())
		if err := checkListLen(n, elemSize); err != nil {
			return err
		}

		dstPtr, err := callRealloc(ctx, gcc.callee.Realloc, gcc.callee.Memory, 0, 0, elemAlign, n*elemSize)
		if err != nil {
			return err
		}

		for i := range n {
			if err := elemStep(ctx, gcc, list.Get(int(i)), dstPtr+i*elemSize); err != nil {
				return err
			}
		}

		if !gcc.callee.Memory.WriteUint32Le(base+ptrOff, dstPtr) {
			return fmt.Errorf("oob list ptr write")
		}
		if !gcc.callee.Memory.WriteUint32Le(base+lenOff, n) {
			return fmt.Errorf("oob list len write")
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitFlags(names []string) {
	numLabels := uint32(len(names))
	if err := validateFlagsLabelCount(numLabels); err != nil {
		// Construction-time violation; surface as trap (no compile error channel).
		panic(&Trap{msg: err.Error()})
	}
	size := flagsByteSize(numLabels)
	off := v.assignBytes(size, size)
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, base uint32) error {
		flags, ok := val.(*ValFlags)
		if !ok {
			return fmt.Errorf("expected *ValFlags, got %T", val)
		}
		bits := flags.packedBits()
		if numLabels < 32 {
			bits &= (uint32(1) << numLabels) - 1
		}
		return writeFlagsBits(gcc.callee.Memory, base+off, size, bits)
	})
}

func (v *valToMemVisitor) VisitEnum(numCases uint32) {
	size := discByteSize(numCases)
	off := v.assignBytes(size, size)
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, base uint32) error {
		e, ok := val.(*ValEnum)
		if !ok {
			return fmt.Errorf("expected *ValEnum, got %T", val)
		}
		disc := e.Discriminant()
		if disc >= numCases {
			return fmt.Errorf("enum: out-of-range case %d (cases=%d)", disc, numCases)
		}
		return writeDiscErr(gcc.callee.Memory, base+off, size, disc)
	})
}

func (v *valToMemVisitor) VisitOwn(rt ResourceType) {
	off := v.assignBytes(4, 4)
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, base uint32) error {
		h, ok := val.(*ValOwnHandle)
		if !ok {
			return fmt.Errorf("expected *ValOwnHandle, got %T", val)
		}
		if got := h.Type(); got != rt {
			return fmt.Errorf("resource type mismatch: handle %v, expected %v", got, rt)
		}
		dst, err := h.TransferOwn(gcc.callee.ResourceTable)
		if err != nil {
			return err
		}
		if !gcc.callee.Memory.WriteUint32Le(base+off, dst.HandleID()) {
			return fmt.Errorf("oob own write")
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitBorrow(rt ResourceType) {
	off := v.assignBytes(4, 4)
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, base uint32) error {
		h, ok := val.(*ValOwnHandle)
		if !ok {
			return fmt.Errorf("expected *ValOwnHandle for borrow lower, got %T", val)
		}
		if got := h.Type(); got != rt {
			return fmt.Errorf("resource type mismatch: handle %v, expected %v", got, rt)
		}
		dst, err := h.LendTo(gcc.callee.ResourceTable, &gcc.Task)
		if err != nil {
			return err
		}
		if !gcc.callee.Memory.WriteUint32Le(base+off, dst.HandleID()) {
			return fmt.Errorf("oob borrow write")
		}
		return nil
	})
}

func (v *valToMemVisitor) VisitVariant(cases []VariantCase) {
	numCases := uint32(len(cases))
	discSize := discByteSize(numCases)

	entries := make([]gocallLowerStep, len(cases))
	var maxSize, maxPayloadAlign uint32
	for i, c := range cases {
		if c.Payload == nil {
			continue
		}
		child := &valToMemVisitor{}
		c.Payload.Accept(child)
		sz := alignUp(child.byteOff, child.maxAlign)
		if sz > maxSize {
			maxSize = sz
		}
		if child.maxAlign > maxPayloadAlign {
			maxPayloadAlign = child.maxAlign
		}
		entries[i] = child.out[0]
	}
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

	v.emit(func(ctx context.Context, gcc *gocallContext, val Val, base uint32) error {
		variant, ok := val.(*ValVariant)
		if !ok {
			return fmt.Errorf("expected *ValVariant, got %T", val)
		}
		disc := variant.Discriminant()
		if disc >= numCases {
			return fmt.Errorf("variant: out-of-range case %d", disc)
		}
		if err := writeDiscErr(gcc.callee.Memory, base+discOff, discSize, disc); err != nil {
			return err
		}
		if entries[disc] == nil {
			return nil
		}
		payloadBase := base + discOff + memPayloadOffset
		return entries[disc](ctx, gcc, variant.Val(), payloadBase)
	})
}

func (v *valToMemVisitor) VisitOption(inner Type) {
	// Layout is variant{none; some(inner)} but with *ValOption on the Go side.
	discSize := discByteSize(2) // 1 byte

	child := &valToMemVisitor{}
	inner.Accept(child)
	payloadSize := alignUp(child.byteOff, child.maxAlign)
	payloadAlign := child.maxAlign
	childStep := child.out[0]

	overallAlign := discSize
	if payloadAlign > overallAlign {
		overallAlign = payloadAlign
	}
	discOff := v.assignBytes(discSize, overallAlign)
	if payloadAlign > 0 {
		v.byteOff = alignUp(v.byteOff, payloadAlign)
	}
	payloadOff := v.byteOff
	memPayloadOffset := payloadOff - discOff
	v.byteOff += payloadSize

	v.emit(func(ctx context.Context, gcc *gocallContext, val Val, base uint32) error {
		opt, ok := val.(*ValOption)
		if !ok {
			return fmt.Errorf("expected *ValOption, got %T", val)
		}
		var disc uint32
		if !opt.IsNone() {
			disc = 1
		}
		if err := writeDiscErr(gcc.callee.Memory, base+discOff, discSize, disc); err != nil {
			return err
		}
		if disc == 0 {
			return nil
		}
		payloadBase := base + discOff + memPayloadOffset
		return childStep(ctx, gcc, opt.Val(), payloadBase)
	})
}

func (v *valToMemVisitor) VisitResult(okT, errT Type) {
	discSize := discByteSize(2)

	var maxSize, maxAlignP uint32
	compileCase := func(t Type) gocallLowerStep {
		if t == nil {
			return nil
		}
		child := &valToMemVisitor{}
		t.Accept(child)
		sz := alignUp(child.byteOff, child.maxAlign)
		if sz > maxSize {
			maxSize = sz
		}
		if child.maxAlign > maxAlignP {
			maxAlignP = child.maxAlign
		}
		return child.out[0]
	}
	okStep := compileCase(okT)
	errStep := compileCase(errT)

	overallAlign := discSize
	if maxAlignP > overallAlign {
		overallAlign = maxAlignP
	}
	discOff := v.assignBytes(discSize, overallAlign)
	if maxAlignP > 0 {
		v.byteOff = alignUp(v.byteOff, maxAlignP)
	}
	payloadOff := v.byteOff
	memPayloadOffset := payloadOff - discOff
	v.byteOff += maxSize

	v.emit(func(ctx context.Context, gcc *gocallContext, val Val, base uint32) error {
		res, ok := val.(*ValResult)
		if !ok {
			return fmt.Errorf("expected *ValResult, got %T", val)
		}
		var disc uint32
		var step gocallLowerStep
		var payload Val
		if res.IsOk() {
			disc = 0
			step = okStep
			if okT != nil {
				payload = res.Ok()
			}
		} else {
			disc = 1
			step = errStep
			if errT != nil {
				payload = res.Err()
			}
		}
		if err := writeDiscErr(gcc.callee.Memory, base+discOff, discSize, disc); err != nil {
			return err
		}
		if step == nil {
			return nil
		}
		payloadBase := base + discOff + memPayloadOffset
		return step(ctx, gcc, payload, payloadBase)
	})
}
