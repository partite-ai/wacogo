package wasmparser

// ComponentExportName represents the name of a component export.
// Uses the same encoding as import names.
type ComponentExportName struct {
	Kind ComponentImportNameKind
	Name string
}

func (n *ComponentExportName) unmarshalBinary(r *BinaryReader) error {
	b, err := r.ReadByte()
	if err != nil {
		return err
	}
	switch b {
	case 0x00:
		n.Kind = ImportNamePlain
	case 0x01:
		n.Kind = ImportNameScoped
	case 0x02:
		n.Kind = ImportNameVersioned
	default:
		return errfAt(r.Offset()-1, "unknown export name kind: 0x%02x", b)
	}
	name, err := r.ReadString()
	if err != nil {
		return err
	}
	n.Name = name
	return nil
}

// ComponentExport represents a component-level export.
type ComponentExport struct {
	Name          ComponentExportName
	Kind          ComponentExternalKind
	Index         uint32
	AscribedType Optional[ComponentTypeRef]
}

func (ce *ComponentExport) unmarshalBinary(r *BinaryReader) error {
	if err := ce.Name.unmarshalBinary(r); err != nil {
		return err
	}
	kind, err := readComponentExternalKind(r)
	if err != nil {
		return err
	}
	ce.Kind = kind
	idx, err := r.ReadU32()
	if err != nil {
		return err
	}
	ce.Index = idx
	// Optional type ascription: 0x00 = no type; 0x01 = type ref follows
	b, err := r.ReadByte()
	if err != nil {
		return err
	}
	switch b {
	case 0x00:
		// No ascribed type
		return nil
	case 0x01:
		tr, err := readComponentTypeRef(r)
		if err != nil {
			return err
		}
		ce.AscribedType = Some(tr)
		return nil
	default:
		return errfAt(r.Offset()-1, "invalid leading byte (0x%x) for optional component export type", b)
	}
}

// readComponentExportNoType reads a ComponentExport without the optional type ascription.
// Used in instance-from-exports where exports don't have type ascriptions.
func readComponentExportNoType(r *BinaryReader) (ComponentExport, error) {
	var exp ComponentExport
	if err := exp.Name.unmarshalBinary(r); err != nil {
		return exp, err
	}
	kind, err := readComponentExternalKind(r)
	if err != nil {
		return exp, err
	}
	exp.Kind = kind
	idx, err := r.ReadU32()
	if err != nil {
		return exp, err
	}
	exp.Index = idx
	// No type ascription
	return exp, nil
}

