package wasmparser

// ComponentValType is the union of value types in the component model.
// Variants: PrimitiveValType, TypeIndexValType.
type ComponentValType interface {
	componentValType()
}

// PrimitiveValType represents a built-in value type.
type PrimitiveValType uint8

func (PrimitiveValType) componentValType()     {}
func (PrimitiveValType) componentDefinedType() {}

const (
	PrimBool PrimitiveValType = iota
	PrimS8
	PrimU8
	PrimS16
	PrimU16
	PrimS32
	PrimU32
	PrimS64
	PrimU64
	PrimF32
	PrimF64
	PrimChar
	PrimString
	PrimErrorContext
)

// Binary opcode mapping:
// 0x7f=Bool, 0x7e=S8, 0x7d=U8, 0x7c=S16, 0x7b=U16, 0x7a=S32,
// 0x79=U32, 0x78=S64, 0x77=U64, 0x76=F32, 0x75=F64, 0x74=Char,
// 0x73=String, 0x64=ErrorContext
var primFromByte = map[byte]PrimitiveValType{
	0x7f: PrimBool, 0x7e: PrimS8, 0x7d: PrimU8, 0x7c: PrimS16,
	0x7b: PrimU16, 0x7a: PrimS32, 0x79: PrimU32, 0x78: PrimS64,
	0x77: PrimU64, 0x76: PrimF32, 0x75: PrimF64, 0x74: PrimChar,
	0x73: PrimString, 0x64: PrimErrorContext,
}

func (p *PrimitiveValType) unmarshalBinary(r *BinaryReader) error {
	b, err := r.ReadByte()
	if err != nil {
		return err
	}
	pv, ok := primFromByte[b]
	if !ok {
		return errfAt(r.Offset()-1, "unknown primitive value type: 0x%02x", b)
	}
	*p = pv
	return nil
}

// TypeIndexValType is a reference to a defined type by index.
type TypeIndexValType uint32

func (TypeIndexValType) componentValType()     {}
func (TypeIndexValType) componentDefinedType() {}

// readComponentValType reads a ComponentValType from the reader.
// If the byte is a known primitive opcode, returns PrimitiveValType.
// Otherwise interprets it as an s33 type index.
func readComponentValType(r *BinaryReader) (ComponentValType, error) {
	b, err := r.Peek()
	if err != nil {
		return nil, err
	}
	if pv, ok := primFromByte[b]; ok {
		_, _ = r.ReadByte() // already validated by Peek
		return pv, nil
	}
	idx, err := r.ReadS33()
	if err != nil {
		return nil, err
	}
	if idx < 0 {
		return nil, errfAt(r.Offset(), "invalid type index: %d", idx)
	}
	return TypeIndexValType(idx), nil
}
