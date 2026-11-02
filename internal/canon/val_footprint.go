package canon

import (
	"math"
	"math/bits"
	"unsafe"
)

// valSlotSize is the Go size of one Val interface slot, as held by a
// []Val list backing array or a variant/option/result payload.
const valSlotSize = uint64(unsafe.Sizeof(Val(nil)))

// valFootprint returns the Go heap bytes one lifted Val of type t
// occupies, excluding the interface slot that holds it and excluding the
// contents of any lists and strings it contains: list steps charge their
// elements and string steps their bytes when they run, so those are only
// known at lift time. The result saturates at math.MaxUint64.
func valFootprint(t Type) uint64 {
	fv := &footprintVisitor{}
	t.Accept(fv)
	return fv.size
}

// listElemCharge returns what a lifted list of n elements with the given
// per-element footprint charges to the lift budget: one interface slot
// plus the footprint per element. Saturates at math.MaxUint64.
func listElemCharge(n uint32, elemFootprint uint64) uint64 {
	return satMul(uint64(n), satAdd(valSlotSize, elemFootprint))
}

func satAdd(a, b uint64) uint64 {
	s, carry := bits.Add64(a, b, 0)
	if carry != 0 {
		return math.MaxUint64
	}
	return s
}

func satMul(a, b uint64) uint64 {
	hi, lo := bits.Mul64(a, b)
	if hi != 0 {
		return math.MaxUint64
	}
	return lo
}

// footprintVisitor computes valFootprint. Scalars are charged their boxed
// size even where the runtime can avoid the allocation for small values,
// so the estimate errs high.
type footprintVisitor struct {
	size uint64
}

func (v *footprintVisitor) add(n uint64) { v.size = satAdd(v.size, n) }

func (v *footprintVisitor) VisitBool()   { v.add(uint64(unsafe.Sizeof(ValBool(false)))) }
func (v *footprintVisitor) VisitU8()     { v.add(uint64(unsafe.Sizeof(ValU8(0)))) }
func (v *footprintVisitor) VisitU16()    { v.add(uint64(unsafe.Sizeof(ValU16(0)))) }
func (v *footprintVisitor) VisitU32()    { v.add(uint64(unsafe.Sizeof(ValU32(0)))) }
func (v *footprintVisitor) VisitU64()    { v.add(uint64(unsafe.Sizeof(ValU64(0)))) }
func (v *footprintVisitor) VisitS8()     { v.add(uint64(unsafe.Sizeof(ValS8(0)))) }
func (v *footprintVisitor) VisitS16()    { v.add(uint64(unsafe.Sizeof(ValS16(0)))) }
func (v *footprintVisitor) VisitS32()    { v.add(uint64(unsafe.Sizeof(ValS32(0)))) }
func (v *footprintVisitor) VisitS64()    { v.add(uint64(unsafe.Sizeof(ValS64(0)))) }
func (v *footprintVisitor) VisitF32()    { v.add(uint64(unsafe.Sizeof(ValF32(0)))) }
func (v *footprintVisitor) VisitF64()    { v.add(uint64(unsafe.Sizeof(ValF64(0)))) }
func (v *footprintVisitor) VisitChar()   { v.add(uint64(unsafe.Sizeof(ValChar(0)))) }
func (v *footprintVisitor) VisitString() { v.add(uint64(unsafe.Sizeof(ValString("")))) }

func (v *footprintVisitor) VisitList(Type) { v.add(uint64(unsafe.Sizeof(ValList{}))) }

func (v *footprintVisitor) VisitRecord(fields []RecordField) {
	v.add(uint64(unsafe.Sizeof(ValRecord{})))
	for _, f := range fields {
		v.add(uint64(unsafe.Sizeof(Field{})))
		f.Type.Accept(v)
	}
}

func (v *footprintVisitor) VisitTuple(types []Type) {
	v.add(uint64(unsafe.Sizeof(ValRecord{})))
	for _, t := range types {
		v.add(uint64(unsafe.Sizeof(Field{})))
		t.Accept(v)
	}
}

func (v *footprintVisitor) VisitVariant(cases []VariantCase) {
	v.add(uint64(unsafe.Sizeof(ValVariant{})))
	var largest uint64
	for _, c := range cases {
		if c.Payload != nil {
			largest = max(largest, valFootprint(c.Payload))
		}
	}
	v.add(largest)
}

func (v *footprintVisitor) VisitOption(inner Type) {
	v.add(uint64(unsafe.Sizeof(ValOption{})))
	inner.Accept(v)
}

func (v *footprintVisitor) VisitResult(ok, err Type) {
	v.add(uint64(unsafe.Sizeof(ValResult{})))
	var largest uint64
	if ok != nil {
		largest = valFootprint(ok)
	}
	if err != nil {
		largest = max(largest, valFootprint(err))
	}
	v.add(largest)
}

func (v *footprintVisitor) VisitFlags([]string) {
	// The struct plus its one-word bits backing array; names and index
	// are shared across the plan.
	v.add(uint64(unsafe.Sizeof(ValFlags{})) + uint64(unsafe.Sizeof(uint32(0))))
}

func (v *footprintVisitor) VisitEnum(uint32)         { v.add(uint64(unsafe.Sizeof(ValEnum{}))) }
func (v *footprintVisitor) VisitOwn(ResourceType)    { v.add(uint64(unsafe.Sizeof(ValOwnHandle{}))) }
func (v *footprintVisitor) VisitBorrow(ResourceType) { v.add(uint64(unsafe.Sizeof(ValOwnHandle{}))) }
