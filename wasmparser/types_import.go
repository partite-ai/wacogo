package wasmparser

// ComponentImportName represents the name of a component import.
type ComponentImportName struct {
	Kind ComponentImportNameKind
	Name string
}

// ComponentImportNameKind identifies how the import name is encoded.
type ComponentImportNameKind uint8

const (
	ImportNamePlain     ComponentImportNameKind = iota // 0x00
	ImportNameScoped                                   // 0x01
	ImportNameVersioned                                // 0x02
)

func (n *ComponentImportName) unmarshalBinary(r *BinaryReader) error {
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
		return errfAt(r.Offset()-1, "unknown import name kind: 0x%02x", b)
	}
	name, err := r.ReadString()
	if err != nil {
		return err
	}
	n.Name = name
	return nil
}

// ComponentTypeRef is the union of type references used in imports.
// Variants: TypeRefModule, TypeRefFunc, TypeRefValue, TypeRefType, TypeRefComponent, TypeRefInstance.
type ComponentTypeRef interface {
	componentTypeRef()
}

// TypeRefModule references a module type by index.
type TypeRefModule struct{ Index uint32 }

func (TypeRefModule) componentTypeRef() {}

// TypeRefFunc references a function type by index.
type TypeRefFunc struct{ Index uint32 }

func (TypeRefFunc) componentTypeRef() {}

// TypeRefValue references a value type.
type TypeRefValue struct{ Type ComponentValType }

func (TypeRefValue) componentTypeRef() {}

// TypeRefType references a type with bounds.
type TypeRefType struct{ Bounds TypeBounds }

func (TypeRefType) componentTypeRef() {}

// TypeRefComponent references a component type by index.
type TypeRefComponent struct{ Index uint32 }

func (TypeRefComponent) componentTypeRef() {}

// TypeRefInstance references an instance type by index.
type TypeRefInstance struct{ Index uint32 }

func (TypeRefInstance) componentTypeRef() {}

// readComponentTypeRef reads a ComponentTypeRef from the binary reader.
func readComponentTypeRef(r *BinaryReader) (ComponentTypeRef, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch b {
	case 0x00:
		// Module: 0x00 0x11 idx
		b2, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b2 != 0x11 {
			return nil, errfAt(r.Offset()-1, "expected 0x11 after 0x00 for module type ref, got 0x%02x", b2)
		}
		idx, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		return TypeRefModule{Index: idx}, nil
	case 0x01:
		idx, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		return TypeRefFunc{Index: idx}, nil
	case 0x02:
		vt, err := readComponentValType(r)
		if err != nil {
			return nil, err
		}
		return TypeRefValue{Type: vt}, nil
	case 0x03:
		bounds, err := readTypeBounds(r)
		if err != nil {
			return nil, err
		}
		return TypeRefType{Bounds: bounds}, nil
	case 0x04:
		idx, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		return TypeRefComponent{Index: idx}, nil
	case 0x05:
		idx, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		return TypeRefInstance{Index: idx}, nil
	default:
		return nil, errfAt(r.Offset()-1, "unknown component type ref kind: 0x%02x", b)
	}
}

// TypeBounds is the union of type bound constraints.
// Variants: TypeBoundsEq, TypeBoundsSubResource.
type TypeBounds interface {
	typeBounds()
}

// TypeBoundsEq constrains a type to be equal to the referenced type.
type TypeBoundsEq struct{ Index uint32 }

func (TypeBoundsEq) typeBounds() {}

// TypeBoundsSubResource constrains a type to be a sub-resource.
type TypeBoundsSubResource struct{}

func (TypeBoundsSubResource) typeBounds() {}

// readTypeBounds reads a TypeBounds from the binary reader.
func readTypeBounds(r *BinaryReader) (TypeBounds, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch b {
	case 0x00:
		idx, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		return TypeBoundsEq{Index: idx}, nil
	case 0x01:
		return TypeBoundsSubResource{}, nil
	default:
		return nil, errfAt(r.Offset()-1, "unknown type bounds kind: 0x%02x", b)
	}
}

// ComponentImport represents a component-level import.
type ComponentImport struct {
	Name ComponentImportName
	Type ComponentTypeRef
}

func (ci *ComponentImport) unmarshalBinary(r *BinaryReader) error {
	if err := ci.Name.unmarshalBinary(r); err != nil {
		return err
	}
	tr, err := readComponentTypeRef(r)
	if err != nil {
		return err
	}
	ci.Type = tr
	return nil
}
