package canon

import (
	"context"
	"fmt"
	"math"
)

// flatToValVisitor emits gocallLiftStep closures that consume data from
// gcc.registers and return the lifted Val. Each Accept call appends exactly
// one step; composite visits (record, tuple, list, variant, option, result)
// compile their children into child visitors and emit a single outer step
// that invokes the sub-steps and wraps their returned Vals into the
// appropriate shape (*ValRecord, *ValVariant, *ValOption, *ValResult,
// *ValList).
type flatToValVisitor struct {
	slot uint32
	out  []gocallLiftStep
}

func (v *flatToValVisitor) assignSlot() uint32    { s := v.slot; v.slot++; return s }
func (v *flatToValVisitor) emit(s gocallLiftStep) { v.out = append(v.out, s) }

// --- Primitives ---

func (v *flatToValVisitor) VisitBool() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, _ uint32) (Val, error) {
		return ValBool(gcc.registers[slot] != 0), nil
	})
}

func (v *flatToValVisitor) VisitU8() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, _ uint32) (Val, error) {
		return ValU8(uint8(gcc.registers[slot])), nil
	})
}

func (v *flatToValVisitor) VisitU16() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, _ uint32) (Val, error) {
		return ValU16(uint16(gcc.registers[slot])), nil
	})
}

func (v *flatToValVisitor) VisitU32() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, _ uint32) (Val, error) {
		return ValU32(uint32(gcc.registers[slot])), nil
	})
}

func (v *flatToValVisitor) VisitU64() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, _ uint32) (Val, error) {
		return ValU64(gcc.registers[slot]), nil
	})
}

func (v *flatToValVisitor) VisitS8() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, _ uint32) (Val, error) {
		return ValS8(int8(uint8(gcc.registers[slot]))), nil
	})
}

func (v *flatToValVisitor) VisitS16() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, _ uint32) (Val, error) {
		return ValS16(int16(uint16(gcc.registers[slot]))), nil
	})
}

func (v *flatToValVisitor) VisitS32() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, _ uint32) (Val, error) {
		return ValS32(int32(uint32(gcc.registers[slot]))), nil
	})
}

func (v *flatToValVisitor) VisitS64() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, _ uint32) (Val, error) {
		return ValS64(int64(gcc.registers[slot])), nil
	})
}

func (v *flatToValVisitor) VisitF32() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, _ uint32) (Val, error) {
		return ValF32(math.Float32frombits(uint32(gcc.registers[slot]))), nil
	})
}

func (v *flatToValVisitor) VisitF64() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, _ uint32) (Val, error) {
		return ValF64(math.Float64frombits(gcc.registers[slot])), nil
	})
}

func (v *flatToValVisitor) VisitChar() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, _ uint32) (Val, error) {
		cp := uint32(gcc.registers[slot])
		if err := validateChar(rune(cp)); err != nil {
			return nil, err
		}
		return ValChar(rune(cp)), nil
	})
}

// --- String / List ---

func (v *flatToValVisitor) VisitString() {
	ptrSlot := v.assignSlot()
	lenSlot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, _ uint32) (val Val, err error) {
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
		ptr := uint32(gcc.registers[ptrSlot])
		coded := uint32(gcc.registers[lenSlot])
		enc := gcc.callee.StringEncoding
		s := decodeString(memShim{gcc.callee.Memory}, ptr, coded, enc)
		return ValString(s), nil
	})
}

func (v *flatToValVisitor) VisitList(elem Type) {
	ptrSlot := v.assignSlot()
	lenSlot := v.assignSlot()

	// List elements are always laid out in memory; compile via a mem visitor
	// which returns a single step producing the element Val.
	child := &memToValVisitor{}
	elem.Accept(child)
	elemSize := alignUp(child.byteOff, child.maxAlign)
	elemAlign := child.maxAlign
	if elemAlign == 0 {
		elemAlign = 1
	}
	elemStep := child.out[0]

	v.emit(func(ctx context.Context, gcc *gocallContext, _ uint32) (Val, error) {
		ptr := uint32(gcc.registers[ptrSlot])
		n := uint32(gcc.registers[lenSlot])
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

func (v *flatToValVisitor) VisitRecord(fields []RecordField) {
	child := &flatToValVisitor{slot: v.slot}
	for _, f := range fields {
		f.Type.Accept(child)
	}
	v.slot = child.slot
	fieldSteps := child.out

	fieldNames := make([]string, len(fields))
	for i, f := range fields {
		fieldNames[i] = f.Name
	}

	v.emit(func(ctx context.Context, gcc *gocallContext, base uint32) (Val, error) {
		recFields := make([]Field, len(fieldSteps))
		for i, step := range fieldSteps {
			fv, err := step(ctx, gcc, base)
			if err != nil {
				return nil, err
			}
			recFields[i] = Field{Name: fieldNames[i], Val: fv}
		}
		return NewValRecord(recFields...), nil
	})
}

func (v *flatToValVisitor) VisitTuple(types []Type) {
	child := &flatToValVisitor{slot: v.slot}
	for _, t := range types {
		t.Accept(child)
	}
	v.slot = child.slot
	fieldSteps := child.out

	// Tuple fields are positionally named "0", "1", "2", etc.
	fieldNames := make([]string, len(types))
	for i := range types {
		fieldNames[i] = fmt.Sprintf("%d", i)
	}

	v.emit(func(ctx context.Context, gcc *gocallContext, base uint32) (Val, error) {
		recFields := make([]Field, len(fieldSteps))
		for i, step := range fieldSteps {
			fv, err := step(ctx, gcc, base)
			if err != nil {
				return nil, err
			}
			recFields[i] = Field{Name: fieldNames[i], Val: fv}
		}
		return NewValRecord(recFields...), nil
	})
}

// --- Flags / Enum ---

func (v *flatToValVisitor) VisitFlags(names []string) {
	numLabels := uint32(len(names))
	if err := validateFlagsLabelCount(numLabels); err != nil {
		panic(&Trap{msg: err.Error()})
	}
	// names and index are captured once at plan-compile time and shared
	// across every lifted *ValFlags produced by this closure.
	index := buildFlagsIndex(names)
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, _ uint32) (Val, error) {
		bits := uint32(gcc.registers[slot])
		if numLabels < 32 {
			bits &= (uint32(1) << numLabels) - 1
		}
		return newLiftedValFlags(names, index, bits), nil
	})
}

func (v *flatToValVisitor) VisitEnum(numCases uint32) {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, _ uint32) (Val, error) {
		disc := uint32(gcc.registers[slot])
		if disc >= numCases {
			return nil, fmt.Errorf("enum: out-of-range discriminant %d (cases=%d)", disc, numCases)
		}
		return NewValEnum(disc), nil
	})
}

// --- Handles ---

func (v *flatToValVisitor) VisitOwn(rt ResourceType) {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, _ uint32) (Val, error) {
		h := uint32(gcc.registers[slot])
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
// function results per the spec. Kept as a defensive panic so a
// validator-skipping path that produces such a type still trips an
// obvious failure rather than silently lifting something that has no
// well-defined drop semantics.
func (v *flatToValVisitor) VisitBorrow(rt ResourceType) {
	v.emit(func(_ context.Context, _ *gocallContext, _ uint32) (Val, error) {
		panic("borrow as result is not permitted by canon ABI")
	})
}

// --- Variant / Option / Result ---

func (v *flatToValVisitor) VisitVariant(cases []VariantCase) {
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

	steps := make([]gocallLiftStep, len(cases))
	for i, c := range cases {
		if c.Payload == nil {
			continue
		}
		child := &flatToValVisitor{slot: payloadStart}
		c.Payload.Accept(child)
		steps[i] = child.out[0]
	}

	v.emit(func(ctx context.Context, gcc *gocallContext, base uint32) (Val, error) {
		disc := uint32(gcc.registers[discSlot])
		if int(disc) >= len(steps) {
			return nil, fmt.Errorf("variant: out-of-range discriminant %d", disc)
		}
		var payload Val
		if steps[disc] != nil {
			pv, err := steps[disc](ctx, gcc, base)
			if err != nil {
				return nil, err
			}
			payload = pv
		}
		return NewValVariant(disc, payload), nil
	})
}

func (v *flatToValVisitor) VisitOption(inner Type) {
	discSlot := v.assignSlot()
	payloadStart := v.slot
	v.slot = payloadStart + flatCountForType(inner)

	child := &flatToValVisitor{slot: payloadStart}
	inner.Accept(child)
	childStep := child.out[0]

	v.emit(func(ctx context.Context, gcc *gocallContext, base uint32) (Val, error) {
		disc := uint32(gcc.registers[discSlot])
		if disc == 0 {
			return ValOptionNone(), nil
		}
		if disc != 1 {
			return nil, fmt.Errorf("option: out-of-range discriminant %d", disc)
		}
		pv, err := childStep(ctx, gcc, base)
		if err != nil {
			return nil, err
		}
		return ValOptionSome(pv), nil
	})
}

func (v *flatToValVisitor) VisitResult(okT, errT Type) {
	discSlot := v.assignSlot()
	payloadStart := v.slot
	var okFlat, errFlat uint32
	if okT != nil {
		okFlat = flatCountForType(okT)
	}
	if errT != nil {
		errFlat = flatCountForType(errT)
	}
	max := okFlat
	if errFlat > max {
		max = errFlat
	}
	v.slot = payloadStart + max

	compileCase := func(t Type) gocallLiftStep {
		if t == nil {
			return nil
		}
		child := &flatToValVisitor{slot: payloadStart}
		t.Accept(child)
		return child.out[0]
	}
	okStep := compileCase(okT)
	errStep := compileCase(errT)

	v.emit(func(ctx context.Context, gcc *gocallContext, base uint32) (Val, error) {
		disc := uint32(gcc.registers[discSlot])
		switch disc {
		case 0:
			if okStep == nil {
				return ValResultOk(nil), nil
			}
			pv, err := okStep(ctx, gcc, base)
			if err != nil {
				return nil, err
			}
			return ValResultOk(pv), nil
		case 1:
			if errStep == nil {
				return ValResultErr(nil), nil
			}
			pv, err := errStep(ctx, gcc, base)
			if err != nil {
				return nil, err
			}
			return ValResultErr(pv), nil
		default:
			return nil, fmt.Errorf("result: out-of-range discriminant %d", disc)
		}
	})
}
