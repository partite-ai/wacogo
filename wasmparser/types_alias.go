package wasmparser

// ComponentAlias is the union of alias definitions in a component.
// Variants: AliasInstanceExport, AliasCoreInstanceExport, AliasOuter.
type ComponentAlias interface {
	componentAlias()
}

// AliasInstanceExport aliases an export from a component instance.
type AliasInstanceExport struct {
	Kind     ComponentExternalKind
	Instance uint32
	Name     string
}

func (AliasInstanceExport) componentAlias() {}

// AliasCoreInstanceExport aliases an export from a core instance.
type AliasCoreInstanceExport struct {
	Kind     CoreSort
	Instance uint32
	Name     string
}

func (AliasCoreInstanceExport) componentAlias() {}

// AliasOuter aliases a definition from an outer component.
type AliasOuter struct {
	Kind  ComponentOuterAliasKind
	Count uint32
	Index uint32
}

func (AliasOuter) componentAlias() {}

// readComponentAlias reads a ComponentAlias from the binary reader.
// The binary format reads the sort byte(s) first, then the discriminant (target).
// Format: byte1 [byte2 if byte1==0x00] discriminant ...
func readComponentAlias(r *BinaryReader) (ComponentAlias, error) {
	// Read sort byte1
	byte1, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	// If byte1 is 0x00, read byte2 for core sort disambiguation
	var byte2 byte
	hasByte2 := false
	if byte1 == 0x00 {
		byte2, err = r.ReadByte()
		if err != nil {
			return nil, err
		}
		hasByte2 = true
	}

	// Read discriminant (target)
	disc, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	switch disc {
	case 0x00:
		// Instance export alias
		kind, err := componentExternalKindFromBytes(byte1, byte2, hasByte2, r.Offset())
		if err != nil {
			return nil, err
		}
		inst, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		name, err := r.ReadString()
		if err != nil {
			return nil, err
		}
		return AliasInstanceExport{Kind: kind, Instance: inst, Name: name}, nil
	case 0x01:
		// Core instance export alias
		// byte1 must be 0x00 for core things, byte2 is the ExternalKind
		if !hasByte2 {
			return nil, errfAt(r.Offset()-1, "expected core sort prefix 0x00 for core instance export alias, got 0x%02x", byte1)
		}
		sort, err := coreSortFromByte(byte2, r.Offset())
		if err != nil {
			return nil, err
		}
		inst, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		name, err := r.ReadString()
		if err != nil {
			return nil, err
		}
		return AliasCoreInstanceExport{Kind: sort, Instance: inst, Name: name}, nil
	case 0x02:
		// Outer alias
		kind, err := componentOuterAliasKindFromBytes(byte1, byte2, hasByte2, r.Offset())
		if err != nil {
			return nil, err
		}
		count, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		idx, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		return AliasOuter{Kind: kind, Count: count, Index: idx}, nil
	default:
		return nil, errfAt(r.Offset()-1, "unknown alias discriminant: 0x%02x", disc)
	}
}

// componentExternalKindFromBytes converts the pre-read sort bytes to a ComponentExternalKind.
func componentExternalKindFromBytes(byte1, byte2 byte, hasByte2 bool, offset uint64) (ComponentExternalKind, error) {
	switch byte1 {
	case 0x00:
		if !hasByte2 {
			return 0, errfAt(offset, "expected second byte for core external kind")
		}
		if byte2 == 0x11 {
			return ExternalKindModule, nil
		}
		return 0, errfAt(offset, "unknown core external kind byte: 0x%02x", byte2)
	case 0x01:
		return ExternalKindFunc, nil
	case 0x02:
		return ExternalKindValue, nil
	case 0x03:
		return ExternalKindType, nil
	case 0x04:
		return ExternalKindComponent, nil
	case 0x05:
		return ExternalKindInstance, nil
	default:
		return 0, errfAt(offset, "unknown component external kind: 0x%02x", byte1)
	}
}

// componentOuterAliasKindFromBytes converts the pre-read sort bytes to a ComponentOuterAliasKind.
func componentOuterAliasKindFromBytes(byte1, byte2 byte, hasByte2 bool, offset uint64) (ComponentOuterAliasKind, error) {
	switch byte1 {
	case 0x00:
		if !hasByte2 {
			return 0, errfAt(offset, "expected second byte for core outer alias kind")
		}
		switch byte2 {
		case 0x10:
			return OuterAliasKindCoreType, nil
		case 0x11:
			return OuterAliasKindCoreModule, nil
		default:
			return 0, errfAt(offset, "unknown core outer alias kind: 0x%02x", byte2)
		}
	case 0x03:
		return OuterAliasKindType, nil
	case 0x04:
		return OuterAliasKindComponent, nil
	default:
		return 0, errfAt(offset, "unknown component outer alias kind: 0x%02x", byte1)
	}
}

// coreSortFromByte converts a byte2 value to a CoreSort for core instance export aliases.
func coreSortFromByte(b byte, offset uint64) (CoreSort, error) {
	switch b {
	case 0x00:
		return CoreSortFunc, nil
	case 0x01:
		return CoreSortTable, nil
	case 0x02:
		return CoreSortMemory, nil
	case 0x03:
		return CoreSortGlobal, nil
	case 0x10:
		return CoreSortType, nil
	case 0x11:
		return CoreSortModule, nil
	case 0x12:
		return CoreSortInstance, nil
	default:
		return 0, errfAt(offset, "unknown core sort: 0x%02x", b)
	}
}
