package wasmparser

// ComponentDefinedType is the interface implemented by all defined types in the component model.
type ComponentDefinedType interface {
	componentDefinedType()
}

// Field is a named field in a RecordType.
type Field struct {
	Name string
	Type ComponentValType
}

// RecordType is a record (struct) type with named fields.
type RecordType struct {
	Fields []Field
}

func (*RecordType) componentDefinedType() {}

func (rt *RecordType) unmarshalBinary(r *BinaryReader) error {
	count, err := r.ReadU32()
	if err != nil {
		return err
	}
	if count > MaxRecordFields {
		return errfAt(r.Offset(), "record field count %d exceeds maximum %d", count, MaxRecordFields)
	}
	rt.Fields = make([]Field, 0, count)
	for range count {
		name, err := r.ReadString()
		if err != nil {
			return err
		}
		vt, err := readComponentValType(r)
		if err != nil {
			return err
		}
		rt.Fields = append(rt.Fields, Field{Name: name, Type: vt})
	}
	return nil
}

// VariantCase is a single case in a VariantType.
type VariantCase struct {
	Name    string
	Type    Optional[ComponentValType]
	Refines Optional[uint32]
}

// VariantType is a variant (sum/union) type.
type VariantType struct {
	Cases []VariantCase
}

func (*VariantType) componentDefinedType() {}

func (vt *VariantType) unmarshalBinary(r *BinaryReader) error {
	count, err := r.ReadU32()
	if err != nil {
		return err
	}
	if count > MaxVariantCases {
		return errfAt(r.Offset(), "variant case count %d exceeds maximum %d", count, MaxVariantCases)
	}
	vt.Cases = make([]VariantCase, 0, count)
	for range count {
		name, err := r.ReadString()
		if err != nil {
			return err
		}
		// Optional type: 0x00 = no type, 0x01 = type present
		typeFlag, err := r.ReadByte()
		if err != nil {
			return err
		}
		var caseType Optional[ComponentValType]
		switch typeFlag {
		case 0x00:
			// no type
		case 0x01:
			cv, err := readComponentValType(r)
			if err != nil {
				return err
			}
			caseType = Some(cv)
		default:
			return errfAt(r.Offset()-1, "invalid variant case type flag: 0x%02x", typeFlag)
		}
		// Optional refines: 0x00 = no refines, 0x01 = refines index present
		refinesFlag, err := r.ReadByte()
		if err != nil {
			return err
		}
		var refines Optional[uint32]
		switch refinesFlag {
		case 0x00:
			// no refines
		case 0x01:
			idx, err := r.ReadU32()
			if err != nil {
				return err
			}
			refines = Some(idx)
		default:
			return errfAt(r.Offset()-1, "invalid variant case refines flag: 0x%02x", refinesFlag)
		}
		vt.Cases = append(vt.Cases, VariantCase{Name: name, Type: caseType, Refines: refines})
	}
	return nil
}

// ListType is a list of a single element type.
type ListType struct {
	Element ComponentValType
}

func (*ListType) componentDefinedType() {}

func (lt *ListType) unmarshalBinary(r *BinaryReader) error {
	vt, err := readComponentValType(r)
	if err != nil {
		return err
	}
	lt.Element = vt
	return nil
}

// TupleType is a fixed-length tuple of value types.
type TupleType struct {
	Types []ComponentValType
}

func (*TupleType) componentDefinedType() {}

func (tt *TupleType) unmarshalBinary(r *BinaryReader) error {
	count, err := r.ReadU32()
	if err != nil {
		return err
	}
	if count > MaxTupleTypes {
		return errfAt(r.Offset(), "tuple type count %d exceeds maximum %d", count, MaxTupleTypes)
	}
	tt.Types = make([]ComponentValType, 0, count)
	for range count {
		vt, err := readComponentValType(r)
		if err != nil {
			return err
		}
		tt.Types = append(tt.Types, vt)
	}
	return nil
}

// FlagsType is a flags type with named bit flags.
type FlagsType struct {
	Labels []string
}

func (*FlagsType) componentDefinedType() {}

func (ft *FlagsType) unmarshalBinary(r *BinaryReader) error {
	count, err := r.ReadU32()
	if err != nil {
		return err
	}
	if count > MaxFlagNames {
		return errfAt(r.Offset(), "flags count %d exceeds maximum %d", count, MaxFlagNames)
	}
	ft.Labels = make([]string, 0, count)
	for range count {
		name, err := r.ReadString()
		if err != nil {
			return err
		}
		ft.Labels = append(ft.Labels, name)
	}
	return nil
}

// EnumType is an enumeration type with named cases.
type EnumType struct {
	Labels []string
}

func (*EnumType) componentDefinedType() {}

func (et *EnumType) unmarshalBinary(r *BinaryReader) error {
	count, err := r.ReadU32()
	if err != nil {
		return err
	}
	if count > MaxEnumCases {
		return errfAt(r.Offset(), "enum case count %d exceeds maximum %d", count, MaxEnumCases)
	}
	et.Labels = make([]string, 0, count)
	for range count {
		name, err := r.ReadString()
		if err != nil {
			return err
		}
		et.Labels = append(et.Labels, name)
	}
	return nil
}

// OptionType is an optional value type.
type OptionType struct {
	Inner ComponentValType
}

func (*OptionType) componentDefinedType() {}

func (ot *OptionType) unmarshalBinary(r *BinaryReader) error {
	vt, err := readComponentValType(r)
	if err != nil {
		return err
	}
	ot.Inner = vt
	return nil
}

// ResultType is a result type with optional ok and err value types.
type ResultType struct {
	Ok  Optional[ComponentValType]
	Err Optional[ComponentValType]
}

func (*ResultType) componentDefinedType() {}

func (rt *ResultType) unmarshalBinary(r *BinaryReader) error {
	// 0x00 = type absent, 0x01 = type present
	okFlag, err := r.ReadByte()
	if err != nil {
		return err
	}
	switch okFlag {
	case 0x00:
		// absent
	case 0x01:
		vt, err := readComponentValType(r)
		if err != nil {
			return err
		}
		rt.Ok = Some(vt)
	default:
		return errfAt(r.Offset()-1, "invalid result ok flag: 0x%02x", okFlag)
	}

	errFlag, err := r.ReadByte()
	if err != nil {
		return err
	}
	switch errFlag {
	case 0x00:
		// absent
	case 0x01:
		vt, err := readComponentValType(r)
		if err != nil {
			return err
		}
		rt.Err = Some(vt)
	default:
		return errfAt(r.Offset()-1, "invalid result err flag: 0x%02x", errFlag)
	}

	return nil
}

// OwnType is an owned handle to a resource.
type OwnType struct {
	ResourceIndex uint32
}

func (*OwnType) componentDefinedType() {}

func (ot *OwnType) unmarshalBinary(r *BinaryReader) error {
	idx, err := r.ReadU32()
	if err != nil {
		return err
	}
	ot.ResourceIndex = idx
	return nil
}

// BorrowType is a borrowed handle to a resource.
type BorrowType struct {
	ResourceIndex uint32
}

func (*BorrowType) componentDefinedType() {}

func (bt *BorrowType) unmarshalBinary(r *BinaryReader) error {
	idx, err := r.ReadU32()
	if err != nil {
		return err
	}
	bt.ResourceIndex = idx
	return nil
}

// defined type opcodes
const (
	opcodeRecord  = 0x72
	opcodeVariant = 0x71
	opcodeList    = 0x70
	opcodeTuple   = 0x6f
	opcodeFlags   = 0x6e
	opcodeEnum    = 0x6d
	opcodeOption  = 0x6b
	opcodeResult  = 0x6a
	opcodeOwn     = 0x69
	opcodeBorrow  = 0x68
)

// readComponentDefinedType reads a ComponentDefinedType from the reader.
// If the byte is a known primitive opcode, returns PrimitiveValType (which implements ComponentDefinedType).
// Otherwise consumes the byte and dispatches to the appropriate type decoder.
func readComponentDefinedType(r *BinaryReader) (ComponentDefinedType, error) {
	b, err := r.Peek()
	if err != nil {
		return nil, err
	}
	// Primitive opcodes — PrimitiveValType also implements componentDefinedType
	if pv, ok := primFromByte[b]; ok {
		_, _ = r.ReadByte() // already validated by Peek
		return pv, nil
	}
	// Consume the opcode byte (already validated by Peek above)
	_, _ = r.ReadByte()
	switch b {
	case opcodeRecord:
		rt := &RecordType{}
		if err := rt.unmarshalBinary(r); err != nil {
			return nil, err
		}
		return rt, nil
	case opcodeVariant:
		vt := &VariantType{}
		if err := vt.unmarshalBinary(r); err != nil {
			return nil, err
		}
		return vt, nil
	case opcodeList:
		lt := &ListType{}
		if err := lt.unmarshalBinary(r); err != nil {
			return nil, err
		}
		return lt, nil
	case opcodeTuple:
		tt := &TupleType{}
		if err := tt.unmarshalBinary(r); err != nil {
			return nil, err
		}
		return tt, nil
	case opcodeFlags:
		ft := &FlagsType{}
		if err := ft.unmarshalBinary(r); err != nil {
			return nil, err
		}
		return ft, nil
	case opcodeEnum:
		et := &EnumType{}
		if err := et.unmarshalBinary(r); err != nil {
			return nil, err
		}
		return et, nil
	case opcodeOption:
		ot := &OptionType{}
		if err := ot.unmarshalBinary(r); err != nil {
			return nil, err
		}
		return ot, nil
	case opcodeResult:
		rt := &ResultType{}
		if err := rt.unmarshalBinary(r); err != nil {
			return nil, err
		}
		return rt, nil
	case opcodeOwn:
		ot := &OwnType{}
		if err := ot.unmarshalBinary(r); err != nil {
			return nil, err
		}
		return ot, nil
	case opcodeBorrow:
		bt := &BorrowType{}
		if err := bt.unmarshalBinary(r); err != nil {
			return nil, err
		}
		return bt, nil
	default:
		return nil, errfAt(r.Offset()-1, "unknown defined type opcode: 0x%02x", b)
	}
}
