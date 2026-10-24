package canon

import (
	"math"
	"unsafe"
)

// generatedInlineStorage bounds an element of a witgen-generated Go slice.
// This follows the generated representation, not the canonical wire layout:
// variants/results are interfaces, lists are slice headers, and options retain
// their inline payload even when absent. Each struct field gets a full scalar
// alignment allowance, plus a trailing allowance for zero-sized fields.
// These conservative charges also cover 32-bit hosts without target metadata.
func generatedInlineStorage(t Type) uint64 {
	v := &generatedStorageVisitor{}
	t.Accept(v)
	return v.size
}

type generatedStorageVisitor struct{ size uint64 }

func (v *generatedStorageVisitor) VisitBool()                 { v.size = 8 }
func (v *generatedStorageVisitor) VisitU8()                   { v.size = 8 }
func (v *generatedStorageVisitor) VisitU16()                  { v.size = 8 }
func (v *generatedStorageVisitor) VisitU32()                  { v.size = 8 }
func (v *generatedStorageVisitor) VisitU64()                  { v.size = 8 }
func (v *generatedStorageVisitor) VisitS8()                   { v.size = 8 }
func (v *generatedStorageVisitor) VisitS16()                  { v.size = 8 }
func (v *generatedStorageVisitor) VisitS32()                  { v.size = 8 }
func (v *generatedStorageVisitor) VisitS64()                  { v.size = 8 }
func (v *generatedStorageVisitor) VisitF32()                  { v.size = 8 }
func (v *generatedStorageVisitor) VisitF64()                  { v.size = 8 }
func (v *generatedStorageVisitor) VisitChar()                 { v.size = 8 }
func (v *generatedStorageVisitor) VisitEnum(uint32)           { v.size = 8 }
func (v *generatedStorageVisitor) VisitFlags([]string)        { v.size = 8 }
func (v *generatedStorageVisitor) VisitOwn(ResourceType)      { v.size = uint64(unsafe.Sizeof(uintptr(0))) }
func (v *generatedStorageVisitor) VisitBorrow(ResourceType)   { v.VisitOwn(nil) }
func (v *generatedStorageVisitor) VisitString()               { v.size = uint64(unsafe.Sizeof("")) }
func (v *generatedStorageVisitor) VisitList(Type)             { v.size = uint64(unsafe.Sizeof([]byte{})) }
func (v *generatedStorageVisitor) VisitVariant([]VariantCase) { v.size = valStorage }
func (v *generatedStorageVisitor) VisitResult(Type, Type)     { v.size = valStorage }

func (v *generatedStorageVisitor) field(t Type) {
	size := generatedInlineStorage(t)
	if size > math.MaxUint64-8 || v.size > math.MaxUint64-size-8 {
		v.size = math.MaxUint64
		return
	}
	v.size += size + 8
}

func (v *generatedStorageVisitor) VisitRecord(fields []RecordField) {
	v.size = 8
	for _, f := range fields {
		v.field(f.Type)
	}
}

func (v *generatedStorageVisitor) VisitTuple(types []Type) {
	v.size = 8
	for _, t := range types {
		v.field(t)
	}
}

func (v *generatedStorageVisitor) VisitOption(inner Type) {
	v.size = 8 // IsSome plus padding.
	v.field(inner)
}
