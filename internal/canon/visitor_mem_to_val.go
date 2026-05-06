package canon

import (
	"context"
	"fmt"
	"math"
)

// memToValVisitor emits gocallLiftStep closures that read bytes from
// gcc.callee.Memory at compile-time offsets relative to the step's `base`
// arg and return the lifted Val. Composite visits (record, tuple, list,
// variant, option, result) compile children into a child visitor and emit
// a single outer step that invokes sub-steps and wraps their Vals into
// the appropriate shape.
type memToValVisitor struct {
	byteOff  uint32
	maxAlign uint32
	out      []gocallLiftStep
}

func (v *memToValVisitor) assignBytes(size, align uint32) uint32 {
	v.byteOff = alignUp(v.byteOff, align)
	off := v.byteOff
	v.byteOff += size
	if align > v.maxAlign {
		v.maxAlign = align
	}
	return off
}

func (v *memToValVisitor) emit(s gocallLiftStep) { v.out = append(v.out, s) }

// --- Primitives ---

func (v *memToValVisitor) VisitBool() {
	off := v.assignBytes(1, 1)
	v.emit(func(_ context.Context, gcc *gocallContext, base uint32) (Val, error) {
		b, ok := gcc.callee.Memory.ReadByte(base + off)
		if !ok {
			return nil, fmt.Errorf("oob bool read")
		}
		return ValBool(b != 0), nil
	})
}

func (v *memToValVisitor) VisitU8() {
	off := v.assignBytes(1, 1)
	v.emit(func(_ context.Context, gcc *gocallContext, base uint32) (Val, error) {
		b, ok := gcc.callee.Memory.ReadByte(base + off)
		if !ok {
			return nil, fmt.Errorf("oob u8 read")
		}
		return ValU8(b), nil
	})
}

func (v *memToValVisitor) VisitU16() {
	off := v.assignBytes(2, 2)
	v.emit(func(_ context.Context, gcc *gocallContext, base uint32) (Val, error) {
		u, ok := gcc.callee.Memory.ReadUint16Le(base + off)
		if !ok {
			return nil, fmt.Errorf("oob u16 read")
		}
		return ValU16(u), nil
	})
}

func (v *memToValVisitor) VisitU32() {
	off := v.assignBytes(4, 4)
	v.emit(func(_ context.Context, gcc *gocallContext, base uint32) (Val, error) {
		u, ok := gcc.callee.Memory.ReadUint32Le(base + off)
		if !ok {
			return nil, fmt.Errorf("oob u32 read")
		}
		return ValU32(u), nil
	})
}

func (v *memToValVisitor) VisitU64() {
	off := v.assignBytes(8, 8)
	v.emit(func(_ context.Context, gcc *gocallContext, base uint32) (Val, error) {
		u, ok := gcc.callee.Memory.ReadUint64Le(base + off)
		if !ok {
			return nil, fmt.Errorf("oob u64 read")
		}
		return ValU64(u), nil
	})
}

func (v *memToValVisitor) VisitS8() {
	off := v.assignBytes(1, 1)
	v.emit(func(_ context.Context, gcc *gocallContext, base uint32) (Val, error) {
		b, ok := gcc.callee.Memory.ReadByte(base + off)
		if !ok {
			return nil, fmt.Errorf("oob s8 read")
		}
		return ValS8(int8(b)), nil
	})
}

func (v *memToValVisitor) VisitS16() {
	off := v.assignBytes(2, 2)
	v.emit(func(_ context.Context, gcc *gocallContext, base uint32) (Val, error) {
		u, ok := gcc.callee.Memory.ReadUint16Le(base + off)
		if !ok {
			return nil, fmt.Errorf("oob s16 read")
		}
		return ValS16(int16(u)), nil
	})
}

func (v *memToValVisitor) VisitS32() {
	off := v.assignBytes(4, 4)
	v.emit(func(_ context.Context, gcc *gocallContext, base uint32) (Val, error) {
		u, ok := gcc.callee.Memory.ReadUint32Le(base + off)
		if !ok {
			return nil, fmt.Errorf("oob s32 read")
		}
		return ValS32(int32(u)), nil
	})
}

func (v *memToValVisitor) VisitS64() {
	off := v.assignBytes(8, 8)
	v.emit(func(_ context.Context, gcc *gocallContext, base uint32) (Val, error) {
		u, ok := gcc.callee.Memory.ReadUint64Le(base + off)
		if !ok {
			return nil, fmt.Errorf("oob s64 read")
		}
		return ValS64(int64(u)), nil
	})
}

func (v *memToValVisitor) VisitF32() {
	off := v.assignBytes(4, 4)
	v.emit(func(_ context.Context, gcc *gocallContext, base uint32) (Val, error) {
		u, ok := gcc.callee.Memory.ReadUint32Le(base + off)
		if !ok {
			return nil, fmt.Errorf("oob f32 read")
		}
		return ValF32(math.Float32frombits(u)), nil
	})
}

func (v *memToValVisitor) VisitF64() {
	off := v.assignBytes(8, 8)
	v.emit(func(_ context.Context, gcc *gocallContext, base uint32) (Val, error) {
		u, ok := gcc.callee.Memory.ReadUint64Le(base + off)
		if !ok {
			return nil, fmt.Errorf("oob f64 read")
		}
		return ValF64(math.Float64frombits(u)), nil
	})
}

func (v *memToValVisitor) VisitChar() {
	off := v.assignBytes(4, 4)
	v.emit(func(_ context.Context, gcc *gocallContext, base uint32) (Val, error) {
		u, ok := gcc.callee.Memory.ReadUint32Le(base + off)
		if !ok {
			return nil, fmt.Errorf("oob char read")
		}
		if err := validateChar(rune(u)); err != nil {
			return nil, err
		}
		return ValChar(rune(u)), nil
	})
}

// --- String / List ---

func (v *memToValVisitor) VisitString() {
	ptrOff := v.assignBytes(4, 4)
	lenOff := v.assignBytes(4, 4)
	v.emit(func(_ context.Context, gcc *gocallContext, base uint32) (val Val, err error) {
		defer func() {
			if r := recover(); r != nil {
				if t, ok := r.(*Trap); ok {
					val = nil
					err = fmt.Errorf("%s", t.msg)
					return
				}
				panic(r)
			}
		}()
		ptr, ok := gcc.callee.Memory.ReadUint32Le(base + ptrOff)
		if !ok {
			return nil, fmt.Errorf("oob string ptr read")
		}
		coded, ok := gcc.callee.Memory.ReadUint32Le(base + lenOff)
		if !ok {
			return nil, fmt.Errorf("oob string len read")
		}
		enc := gcc.callee.StringEncoding
		s := decodeString(memShim{gcc.callee.Memory}, ptr, coded, enc)
		return ValString(s), nil
	})
}

func (v *memToValVisitor) VisitList(elem Type) {
	ptrOff := v.assignBytes(4, 4)
	lenOff := v.assignBytes(4, 4)

	child := &memToValVisitor{}
	elem.Accept(child)
	elemSize := alignUp(child.byteOff, child.maxAlign)
	elemAlign := child.maxAlign
	if elemAlign == 0 {
		elemAlign = 1
	}
	elemStep := child.out[0]

	v.emit(func(ctx context.Context, gcc *gocallContext, base uint32) (Val, error) {
		ptr, ok := gcc.callee.Memory.ReadUint32Le(base + ptrOff)
		if !ok {
			return nil, fmt.Errorf("oob list ptr read")
		}
		n, ok := gcc.callee.Memory.ReadUint32Le(base + lenOff)
		if !ok {
			return nil, fmt.Errorf("oob list len read")
		}
		if err := checkListLen(n, elemSize); err != nil {
			return nil, err
		}
		if n > 0 && elemAlign > 1 {
			if err := checkAlignment(ptr, elemAlign); err != nil {
				return nil, err
			}
		}

		out := make([]Val, n)
		for i := range n {
			ev, err := elemStep(ctx, gcc, ptr+i*elemSize)
			if err != nil {
				return nil, err
			}
			out[i] = ev
		}
		return newValListFromSlice(out), nil
	})
}

// --- Record / Tuple ---

func (v *memToValVisitor) VisitRecord(fields []RecordField) {
	child := &memToValVisitor{}
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

	fieldNames := make([]string, len(fields))
	for i, f := range fields {
		fieldNames[i] = f.Name
	}

	v.emit(func(ctx context.Context, gcc *gocallContext, base uint32) (Val, error) {
		recBase := base + recordOff
		recFields := make([]Field, len(fieldSteps))
		for i, step := range fieldSteps {
			fv, err := step(ctx, gcc, recBase)
			if err != nil {
				return nil, err
			}
			recFields[i] = Field{Name: fieldNames[i], Val: fv}
		}
		return NewValRecord(recFields...), nil
	})
}

func (v *memToValVisitor) VisitTuple(types []Type) {
	child := &memToValVisitor{}
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

	// Tuple fields are positionally named "0", "1", "2", etc.
	fieldNames := make([]string, len(types))
	for i := range types {
		fieldNames[i] = fmt.Sprintf("%d", i)
	}

	v.emit(func(ctx context.Context, gcc *gocallContext, base uint32) (Val, error) {
		recBase := base + recordOff
		recFields := make([]Field, len(fieldSteps))
		for i, step := range fieldSteps {
			fv, err := step(ctx, gcc, recBase)
			if err != nil {
				return nil, err
			}
			recFields[i] = Field{Name: fieldNames[i], Val: fv}
		}
		return NewValRecord(recFields...), nil
	})
}

// --- Flags / Enum ---

func (v *memToValVisitor) VisitFlags(names []string) {
	numLabels := uint32(len(names))
	if err := validateFlagsLabelCount(numLabels); err != nil {
		panic(&Trap{msg: err.Error()})
	}
	// names and index are captured once at plan-compile time and shared
	// across every lifted *ValFlags produced by this closure.
	index := buildFlagsIndex(names)
	size := flagsByteSize(numLabels)
	off := v.assignBytes(size, size)
	v.emit(func(_ context.Context, gcc *gocallContext, base uint32) (Val, error) {
		bits, err := readFlagsBits(gcc.callee.Memory, base+off, size)
		if err != nil {
			return nil, err
		}
		if numLabels < 32 {
			bits &= (uint32(1) << numLabels) - 1
		}
		return newLiftedValFlags(names, index, bits), nil
	})
}

func (v *memToValVisitor) VisitEnum(numCases uint32) {
	size := discByteSize(numCases)
	off := v.assignBytes(size, size)
	v.emit(func(_ context.Context, gcc *gocallContext, base uint32) (Val, error) {
		disc, err := readDiscErr(gcc.callee.Memory, base+off, size)
		if err != nil {
			return nil, err
		}
		if disc >= numCases {
			return nil, fmt.Errorf("enum: out-of-range discriminant %d (cases=%d)", disc, numCases)
		}
		return NewValEnum(disc), nil
	})
}

// --- Handles ---

func (v *memToValVisitor) VisitOwn(rt ResourceType) {
	off := v.assignBytes(4, 4)
	v.emit(func(_ context.Context, gcc *gocallContext, base uint32) (Val, error) {
		h, ok := gcc.callee.Memory.ReadUint32Le(base + off)
		if !ok {
			return nil, fmt.Errorf("oob own read")
		}
		srcH, err := gcc.callee.ResourceTable.LookupOwn(rt, h)
		if err != nil {
			return nil, err
		}
		val := newValOwnHandleEmpty()
		if _, err := srcH.TransferOwn(val); err != nil {
			return nil, err
		}
		return val, nil
	})
}

// VisitBorrow on the result-direction lift path is unreachable in
// well-typed canonical-ABI programs: borrow handles cannot appear as
// function results per the spec. See flatToValVisitor.VisitBorrow.
func (v *memToValVisitor) VisitBorrow(rt ResourceType) {
	v.emit(func(_ context.Context, _ *gocallContext, _ uint32) (Val, error) {
		panic("borrow as result is not permitted by canon ABI")
	})
}

// --- Variant / Option / Result ---

func (v *memToValVisitor) VisitVariant(cases []VariantCase) {
	numCases := uint32(len(cases))
	discSize := discByteSize(numCases)
	discOff := v.assignBytes(discSize, discSize)

	steps := make([]gocallLiftStep, len(cases))
	var maxSize, maxPayloadAlign uint32
	for i, c := range cases {
		if c.Payload == nil {
			continue
		}
		child := &memToValVisitor{}
		c.Payload.Accept(child)
		sz := alignUp(child.byteOff, child.maxAlign)
		if sz > maxSize {
			maxSize = sz
		}
		if child.maxAlign > maxPayloadAlign {
			maxPayloadAlign = child.maxAlign
		}
		steps[i] = child.out[0]
	}
	if maxPayloadAlign > 0 {
		v.byteOff = alignUp(v.byteOff, maxPayloadAlign)
		if maxPayloadAlign > v.maxAlign {
			v.maxAlign = maxPayloadAlign
		}
	}
	payloadOff := v.byteOff
	memPayloadOffset := payloadOff - discOff
	v.byteOff += maxSize

	v.emit(func(ctx context.Context, gcc *gocallContext, base uint32) (Val, error) {
		disc, err := readDiscErr(gcc.callee.Memory, base+discOff, discSize)
		if err != nil {
			return nil, err
		}
		if disc >= numCases {
			return nil, fmt.Errorf("variant: out-of-range discriminant %d", disc)
		}
		var payload Val
		if steps[disc] != nil {
			payloadBase := base + discOff + memPayloadOffset
			pv, err := steps[disc](ctx, gcc, payloadBase)
			if err != nil {
				return nil, err
			}
			payload = pv
		}
		return NewValVariant(disc, payload), nil
	})
}

func (v *memToValVisitor) VisitOption(inner Type) {
	discSize := discByteSize(2) // 1 byte
	discOff := v.assignBytes(discSize, discSize)

	child := &memToValVisitor{}
	inner.Accept(child)
	payloadSize := alignUp(child.byteOff, child.maxAlign)
	payloadAlign := child.maxAlign
	childStep := child.out[0]

	if payloadAlign > 0 {
		v.byteOff = alignUp(v.byteOff, payloadAlign)
		if payloadAlign > v.maxAlign {
			v.maxAlign = payloadAlign
		}
	}
	payloadOff := v.byteOff
	memPayloadOffset := payloadOff - discOff
	v.byteOff += payloadSize

	v.emit(func(ctx context.Context, gcc *gocallContext, base uint32) (Val, error) {
		disc, err := readDiscErr(gcc.callee.Memory, base+discOff, discSize)
		if err != nil {
			return nil, err
		}
		if disc == 0 {
			return ValOptionNone(), nil
		}
		if disc != 1 {
			return nil, fmt.Errorf("option: out-of-range discriminant %d", disc)
		}
		payloadBase := base + discOff + memPayloadOffset
		pv, err := childStep(ctx, gcc, payloadBase)
		if err != nil {
			return nil, err
		}
		return ValOptionSome(pv), nil
	})
}

func (v *memToValVisitor) VisitResult(okT, errT Type) {
	discSize := discByteSize(2)
	discOff := v.assignBytes(discSize, discSize)

	var maxSize, maxAlignP uint32
	compileCase := func(t Type) gocallLiftStep {
		if t == nil {
			return nil
		}
		child := &memToValVisitor{}
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

	if maxAlignP > 0 {
		v.byteOff = alignUp(v.byteOff, maxAlignP)
		if maxAlignP > v.maxAlign {
			v.maxAlign = maxAlignP
		}
	}
	payloadOff := v.byteOff
	memPayloadOffset := payloadOff - discOff
	v.byteOff += maxSize

	v.emit(func(ctx context.Context, gcc *gocallContext, base uint32) (Val, error) {
		disc, err := readDiscErr(gcc.callee.Memory, base+discOff, discSize)
		if err != nil {
			return nil, err
		}
		var step gocallLiftStep
		var wrap func(Val) *ValResult
		switch disc {
		case 0:
			step = okStep
			wrap = ValResultOk
			if okT == nil {
				return ValResultOk(nil), nil
			}
		case 1:
			step = errStep
			wrap = ValResultErr
			if errT == nil {
				return ValResultErr(nil), nil
			}
		default:
			return nil, fmt.Errorf("result: out-of-range discriminant %d", disc)
		}
		payloadBase := base + discOff + memPayloadOffset
		pv, err := step(ctx, gcc, payloadBase)
		if err != nil {
			return nil, err
		}
		return wrap(pv), nil
	})
}
