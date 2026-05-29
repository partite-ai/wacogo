package canon

import (
	"context"
	"fmt"
	"math"
)

// valToFlatVisitor emits gocallLowerStep closures for Go → component
// calls in flat mode. Each Accept call appends exactly one step, so the
// plan's paramSteps / resultSteps parallel the top-level type list
// element-for-element. Slot indices are baked absolute at compile time;
// the `base` step arg is unused in flat mode.
//
// Composite types (record, tuple, list, variant, option, result) compile
// their children into child visitors and emit a single outer step that
// destructures the input Val and dispatches sub-Vals to the captured
// child step closures.
type valToFlatVisitor struct {
	slot uint32
	out  []gocallLowerStep
}

func (v *valToFlatVisitor) assignSlot() uint32     { s := v.slot; v.slot++; return s }
func (v *valToFlatVisitor) emit(s gocallLowerStep) { v.out = append(v.out, s) }

// --- Primitives ---

func (v *valToFlatVisitor) VisitBool() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, _ uint32) error {
		b, ok := val.(ValBool)
		if !ok {
			return fmt.Errorf("expected ValBool, got %T", val)
		}
		var out uint64
		if b {
			out = 1
		}
		gcc.registers[slot] = out
		return nil
	})
}

func (v *valToFlatVisitor) VisitU8() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, _ uint32) error {
		u, ok := val.(ValU8)
		if !ok {
			return fmt.Errorf("expected ValU8, got %T", val)
		}
		gcc.registers[slot] = uint64(u)
		return nil
	})
}

func (v *valToFlatVisitor) VisitU16() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, _ uint32) error {
		u, ok := val.(ValU16)
		if !ok {
			return fmt.Errorf("expected ValU16, got %T", val)
		}
		gcc.registers[slot] = uint64(u)
		return nil
	})
}

func (v *valToFlatVisitor) VisitU32() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, _ uint32) error {
		u, ok := val.(ValU32)
		if !ok {
			return fmt.Errorf("expected ValU32, got %T", val)
		}
		gcc.registers[slot] = uint64(u)
		return nil
	})
}

func (v *valToFlatVisitor) VisitU64() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, _ uint32) error {
		u, ok := val.(ValU64)
		if !ok {
			return fmt.Errorf("expected ValU64, got %T", val)
		}
		gcc.registers[slot] = uint64(u)
		return nil
	})
}

func (v *valToFlatVisitor) VisitS8() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, _ uint32) error {
		s, ok := val.(ValS8)
		if !ok {
			return fmt.Errorf("expected ValS8, got %T", val)
		}
		// sign-extend s8 → i32 in the slot (low 32 bits sign-extended)
		gcc.registers[slot] = uint64(uint32(int32(s)))
		return nil
	})
}

func (v *valToFlatVisitor) VisitS16() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, _ uint32) error {
		s, ok := val.(ValS16)
		if !ok {
			return fmt.Errorf("expected ValS16, got %T", val)
		}
		gcc.registers[slot] = uint64(uint32(int32(s)))
		return nil
	})
}

func (v *valToFlatVisitor) VisitS32() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, _ uint32) error {
		s, ok := val.(ValS32)
		if !ok {
			return fmt.Errorf("expected ValS32, got %T", val)
		}
		gcc.registers[slot] = uint64(uint32(s))
		return nil
	})
}

func (v *valToFlatVisitor) VisitS64() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, _ uint32) error {
		s, ok := val.(ValS64)
		if !ok {
			return fmt.Errorf("expected ValS64, got %T", val)
		}
		gcc.registers[slot] = uint64(s)
		return nil
	})
}

func (v *valToFlatVisitor) VisitF32() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, _ uint32) error {
		f, ok := val.(ValF32)
		if !ok {
			return fmt.Errorf("expected ValF32, got %T", val)
		}
		gcc.registers[slot] = uint64(math.Float32bits(float32(f)))
		return nil
	})
}

func (v *valToFlatVisitor) VisitF64() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, _ uint32) error {
		f, ok := val.(ValF64)
		if !ok {
			return fmt.Errorf("expected ValF64, got %T", val)
		}
		gcc.registers[slot] = math.Float64bits(float64(f))
		return nil
	})
}

func (v *valToFlatVisitor) VisitChar() {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, _ uint32) error {
		c, ok := val.(ValChar)
		if !ok {
			return fmt.Errorf("expected ValChar, got %T", val)
		}
		if err := validateChar(rune(c)); err != nil {
			return err
		}
		gcc.registers[slot] = uint64(uint32(rune(c)))
		return nil
	})
}

// --- Composites ---

// VisitRecord compiles each field via a child visitor sharing this
// visitor's slot space, then emits a single composite step that
// destructures the record and forwards fields to their sub-steps.
func (v *valToFlatVisitor) VisitRecord(fields []RecordField) {
	child := &valToFlatVisitor{slot: v.slot}
	for _, f := range fields {
		f.Type.Accept(child)
	}
	v.slot = child.slot
	fieldSteps := child.out

	v.emit(func(ctx context.Context, gcc *gocallContext, val Val, base uint32) error {
		rec, ok := val.(*ValRecord)
		if !ok {
			return fmt.Errorf("expected *ValRecord, got %T", val)
		}
		for i, step := range fieldSteps {
			if err := step(ctx, gcc, rec.FieldByIndex(i), base); err != nil {
				return err
			}
		}
		return nil
	})
}

// VisitTuple mirrors VisitRecord; the parent package represents tuples as
// *ValRecord too (see val.go).
func (v *valToFlatVisitor) VisitTuple(types []Type) {
	child := &valToFlatVisitor{slot: v.slot}
	for _, t := range types {
		t.Accept(child)
	}
	v.slot = child.slot
	fieldSteps := child.out

	v.emit(func(ctx context.Context, gcc *gocallContext, val Val, base uint32) error {
		rec, ok := val.(*ValRecord)
		if !ok {
			return fmt.Errorf("expected *ValRecord for tuple, got %T", val)
		}
		for i, step := range fieldSteps {
			if err := step(ctx, gcc, rec.FieldByIndex(i), base); err != nil {
				return err
			}
		}
		return nil
	})
}

func (v *valToFlatVisitor) VisitFlags(names []string) {
	numLabels := uint32(len(names))
	if err := validateFlagsLabelCount(numLabels); err != nil {
		// Construction-time violation; surface as a trap since there is no
		// compile-time error channel on the visitor interface.
		panic(&Trap{msg: err.Error()})
	}
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, _ uint32) error {
		flags, ok := val.(*ValFlags)
		if !ok {
			return fmt.Errorf("expected *ValFlags, got %T", val)
		}
		bits := flags.packedBits()
		if numLabels < 32 {
			bits &= (uint32(1) << numLabels) - 1
		}
		gcc.registers[slot] = uint64(bits)
		return nil
	})
}

func (v *valToFlatVisitor) VisitEnum(numCases uint32) {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, _ uint32) error {
		e, ok := val.(*ValEnum)
		if !ok {
			return fmt.Errorf("expected *ValEnum, got %T", val)
		}
		disc := e.Discriminant()
		if disc >= numCases {
			return fmt.Errorf("enum: out-of-range case %d (cases=%d)", disc, numCases)
		}
		gcc.registers[slot] = uint64(disc)
		return nil
	})
}

// --- String / list / variant / option / result / handles ---

func (v *valToFlatVisitor) VisitString() {
	ptrSlot := v.assignSlot()
	lenSlot := v.assignSlot()
	v.emit(func(ctx context.Context, gcc *gocallContext, val Val, _ uint32) error {
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
		gcc.registers[ptrSlot] = uint64(ptr)
		gcc.registers[lenSlot] = uint64(coded)
		return nil
	})
}

func (v *valToFlatVisitor) VisitList(elem Type) {
	ptrSlot := v.assignSlot()
	lenSlot := v.assignSlot()

	// List elements are always laid out in memory; compile via a mem visitor
	// which produces a single step consuming the element Val.
	child := &valToMemVisitor{}
	elem.Accept(child)
	elemSize := alignUp(child.byteOff, child.maxAlign)
	elemAlign := child.maxAlign
	if elemAlign == 0 {
		elemAlign = 1
	}
	elemStep := child.out[0]

	v.emit(func(ctx context.Context, gcc *gocallContext, val Val, _ uint32) error {
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

		gcc.registers[ptrSlot] = uint64(dstPtr)
		gcc.registers[lenSlot] = uint64(n)
		return nil
	})
}

func (v *valToFlatVisitor) VisitVariant(cases []VariantCase) {
	discSlot := v.assignSlot()
	payloadStart := v.slot

	// Reserve joined-payload slots (max across payload-bearing cases).
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
		step           gocallLowerStep
		caseLocalSlots uint32
	}
	entries := make([]caseEntry, len(cases))
	for i, c := range cases {
		if c.Payload == nil {
			continue
		}
		child := &valToFlatVisitor{slot: payloadStart}
		c.Payload.Accept(child)
		entries[i] = caseEntry{child.out[0], child.slot - payloadStart}
	}

	v.emit(func(ctx context.Context, gcc *gocallContext, val Val, base uint32) error {
		variant, ok := val.(*ValVariant)
		if !ok {
			return fmt.Errorf("expected *ValVariant, got %T", val)
		}
		disc := variant.Discriminant()
		if disc >= uint32(len(entries)) {
			return fmt.Errorf("variant: out-of-range case %d", disc)
		}
		gcc.registers[discSlot] = uint64(disc)
		if entries[disc].step != nil {
			if err := entries[disc].step(ctx, gcc, variant.Val(), base); err != nil {
				return err
			}
		}
		// Zero unused joined slots.
		for i := entries[disc].caseLocalSlots; i < joinedSlots; i++ {
			gcc.registers[payloadStart+i] = 0
		}
		return nil
	})
}

func (v *valToFlatVisitor) VisitOption(inner Type) {
	discSlot := v.assignSlot()
	payloadStart := v.slot

	joinedSlots := flatCountForType(inner)
	v.slot = payloadStart + joinedSlots

	child := &valToFlatVisitor{slot: payloadStart}
	inner.Accept(child)
	childStep := child.out[0]
	caseLocalSlots := child.slot - payloadStart

	v.emit(func(ctx context.Context, gcc *gocallContext, val Val, base uint32) error {
		opt, ok := val.(*ValOption)
		if !ok {
			return fmt.Errorf("expected *ValOption, got %T", val)
		}
		var disc uint32
		if !opt.IsNone() {
			disc = 1
		}
		gcc.registers[discSlot] = uint64(disc)
		if disc == 1 {
			if err := childStep(ctx, gcc, opt.Val(), base); err != nil {
				return err
			}
			for i := caseLocalSlots; i < joinedSlots; i++ {
				gcc.registers[payloadStart+i] = 0
			}
		} else {
			for i := range joinedSlots {
				gcc.registers[payloadStart+i] = 0
			}
		}
		return nil
	})
}

func (v *valToFlatVisitor) VisitResult(okT, errT Type) {
	discSlot := v.assignSlot()
	payloadStart := v.slot

	var joinedSlots uint32
	if okT != nil {
		if n := flatCountForType(okT); n > joinedSlots {
			joinedSlots = n
		}
	}
	if errT != nil {
		if n := flatCountForType(errT); n > joinedSlots {
			joinedSlots = n
		}
	}
	v.slot = payloadStart + joinedSlots

	var okStep, errStep gocallLowerStep
	var okLocal, errLocal uint32
	if okT != nil {
		child := &valToFlatVisitor{slot: payloadStart}
		okT.Accept(child)
		okStep = child.out[0]
		okLocal = child.slot - payloadStart
	}
	if errT != nil {
		child := &valToFlatVisitor{slot: payloadStart}
		errT.Accept(child)
		errStep = child.out[0]
		errLocal = child.slot - payloadStart
	}

	v.emit(func(ctx context.Context, gcc *gocallContext, val Val, base uint32) error {
		res, ok := val.(*ValResult)
		if !ok {
			return fmt.Errorf("expected *ValResult, got %T", val)
		}
		var disc, local uint32
		var step gocallLowerStep
		var payload Val
		if res.IsOk() {
			disc = 0
			step = okStep
			local = okLocal
			if okT != nil {
				payload = res.Ok()
			}
		} else {
			disc = 1
			step = errStep
			local = errLocal
			if errT != nil {
				payload = res.Err()
			}
		}
		gcc.registers[discSlot] = uint64(disc)
		if step != nil {
			if err := step(ctx, gcc, payload, base); err != nil {
				return err
			}
		}
		for i := local; i < joinedSlots; i++ {
			gcc.registers[payloadStart+i] = 0
		}
		return nil
	})
}

func (v *valToFlatVisitor) VisitOwn(rt ResourceType) {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, _ uint32) error {
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
		gcc.registers[slot] = uint64(dst.HandleID())
		return nil
	})
}

func (v *valToFlatVisitor) VisitBorrow(rt ResourceType) {
	slot := v.assignSlot()
	v.emit(func(_ context.Context, gcc *gocallContext, val Val, _ uint32) error {
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
		gcc.registers[slot] = uint64(dst.HandleID())
		return nil
	})
}
