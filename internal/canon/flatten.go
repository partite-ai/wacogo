package canon

import "github.com/tetratelabs/wazero/api"

// coreValueType is a core WebAssembly value type byte used in the canonical
// ABI flat representation (i32=0x7f, i64=0x7e, f32=0x7d, f64=0x7c).
type coreValueType byte

const (
	coreI32 coreValueType = 0x7f
	coreI64 coreValueType = 0x7e
	coreF32 coreValueType = 0x7d
	coreF64 coreValueType = 0x7c
)

// flattenTypeVisitor collects coreValueType slices by walking a Type tree.
// Used by flattenTypeOf to compute the canonical-ABI flat representation.
type flattenTypeVisitor struct {
	out []coreValueType
}

func (fv *flattenTypeVisitor) add(vt coreValueType) { fv.out = append(fv.out, vt) }

func (fv *flattenTypeVisitor) VisitBool()       { fv.add(coreI32) }
func (fv *flattenTypeVisitor) VisitU8()         { fv.add(coreI32) }
func (fv *flattenTypeVisitor) VisitU16()        { fv.add(coreI32) }
func (fv *flattenTypeVisitor) VisitU32()        { fv.add(coreI32) }
func (fv *flattenTypeVisitor) VisitU64()        { fv.add(coreI64) }
func (fv *flattenTypeVisitor) VisitS8()         { fv.add(coreI32) }
func (fv *flattenTypeVisitor) VisitS16()        { fv.add(coreI32) }
func (fv *flattenTypeVisitor) VisitS32()        { fv.add(coreI32) }
func (fv *flattenTypeVisitor) VisitS64()        { fv.add(coreI64) }
func (fv *flattenTypeVisitor) VisitF32()        { fv.add(coreF32) }
func (fv *flattenTypeVisitor) VisitF64()        { fv.add(coreF64) }
func (fv *flattenTypeVisitor) VisitChar()       { fv.add(coreI32) }
func (fv *flattenTypeVisitor) VisitString()     { fv.add(coreI32); fv.add(coreI32) }
func (fv *flattenTypeVisitor) VisitList(_ Type) { fv.add(coreI32); fv.add(coreI32) }
func (fv *flattenTypeVisitor) VisitFlags(names []string) {
	numLabels := uint32(len(names))
	if err := validateFlagsLabelCount(numLabels); err != nil {
		// Construction-time violation; surface as a trap since there is no
		// compile-time error channel on the visitor interface.
		panic(&Trap{msg: err.Error()})
	}
	fv.add(coreI32)
}
func (fv *flattenTypeVisitor) VisitOwn(_ ResourceType)    { fv.add(coreI32) }
func (fv *flattenTypeVisitor) VisitBorrow(_ ResourceType) { fv.add(coreI32) }
func (fv *flattenTypeVisitor) VisitEnum(_ uint32)         { fv.add(coreI32) }

func (fv *flattenTypeVisitor) VisitRecord(fields []RecordField) {
	for _, f := range fields {
		sub := &flattenTypeVisitor{}
		f.Type.Accept(sub)
		fv.out = append(fv.out, sub.out...)
	}
}

func (fv *flattenTypeVisitor) VisitTuple(types []Type) {
	for _, t := range types {
		sub := &flattenTypeVisitor{}
		t.Accept(sub)
		fv.out = append(fv.out, sub.out...)
	}
}

func (fv *flattenTypeVisitor) VisitVariant(cases []VariantCase) {
	// discriminant always i32
	fv.add(coreI32)
	// payload: element-wise join across all case payloads
	var joined []coreValueType
	for _, c := range cases {
		if c.Payload == nil {
			continue
		}
		sub := &flattenTypeVisitor{}
		c.Payload.Accept(sub)
		joined = joinCoreValueTypeLists(joined, sub.out)
	}
	fv.out = append(fv.out, joined...)
}

func (fv *flattenTypeVisitor) VisitOption(inner Type) {
	fv.VisitVariant([]VariantCase{{Name: "none"}, {Name: "some", Payload: inner}})
}

func (fv *flattenTypeVisitor) VisitResult(ok, err Type) {
	fv.VisitVariant([]VariantCase{{Name: "ok", Payload: ok}, {Name: "err", Payload: err}})
}

// flattenTypeOf returns the canonical-ABI flat representation of t as a
// sequence of coreValueType values. Mirrors root's flatten(Type).
func flattenTypeOf(t Type) []coreValueType {
	fv := &flattenTypeVisitor{}
	t.Accept(fv)
	return fv.out
}

// joinCoreValueTypeLists computes the element-wise join of two flat type
// lists, extending to the length of the longer list. Mirrors root's
// joinFlatLists.
func joinCoreValueTypeLists(a, b []coreValueType) []coreValueType {
	if len(b) > len(a) {
		a, b = b, a
	}
	// a is now the longer one
	result := make([]coreValueType, len(a))
	copy(result, a)
	for i := range len(b) {
		result[i] = joinCoreValueType(result[i], b[i])
	}
	return result
}

// joinCoreValueType returns the "join" of two core value types per canonical
// ABI rules. Mirrors root's joinCoreType.
//
// Join table:
//
//	i32 join i64 → i64
//	i32 join f32 → i32
//	i32 join f64 → i64
//	i64 join f32 → i64
//	i64 join f64 → i64
//	f32 join f64 → i64
func joinCoreValueType(a, b coreValueType) coreValueType {
	if a == b {
		return a
	}
	// i32 join f32 → i32 (both are 32-bit; keep as i32 to avoid widening)
	if (a == coreI32 && b == coreF32) || (a == coreF32 && b == coreI32) {
		return coreI32
	}
	// All other mixed-type combinations → i64
	return coreI64
}

// coreValueTypeToAPI maps a coreValueType to a wazero api.ValueType.
// Mirrors root's coreTypeToAPI.
func coreValueTypeToAPI(v coreValueType) api.ValueType {
	switch v {
	case coreI32:
		return api.ValueTypeI32
	case coreI64:
		return api.ValueTypeI64
	case coreF32:
		return api.ValueTypeF32
	case coreF64:
		return api.ValueTypeF64
	default:
		return api.ValueTypeI32
	}
}

// coreValueTypeToWasm returns the raw wasm byte for a coreValueType.
// The byte values are already identical (0x7f, 0x7e, 0x7d, 0x7c), so this
// is a simple cast.
func coreValueTypeToWasm(v coreValueType) byte {
	return byte(v)
}
