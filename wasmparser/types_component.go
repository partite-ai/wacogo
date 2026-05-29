package wasmparser

// ValType represents a core value type used in resource representations.
type ValType uint8

const (
	ValTypeI32 ValType = iota
	ValTypeI64
)

// ComponentTypeDef is the union of all top-level entries in a component
// type section. Variants: *ComponentDefinedTypeEntry, *ComponentFuncType,
// *ResourceType, *ComponentTypeDecl, *InstanceTypeDecl. (Distinct from
// the public *ComponentType handle returned by
// (*ValidatingParser).TopLevelComponentType, which is a post-parse
// reference to a fully-resolved component type.)
type ComponentTypeDef interface {
	componentTypeDef()
}

// ComponentDefinedTypeEntry wraps a ComponentDefinedType as a ComponentTypeDef.
type ComponentDefinedTypeEntry struct {
	Type ComponentDefinedType
}

func (*ComponentDefinedTypeEntry) componentTypeDef() {}

// FuncParam is a named parameter or result in a component function type.
type FuncParam struct {
	Name string
	Type ComponentValType
}

// ComponentFuncType represents a component function type with params and results.
type ComponentFuncType struct {
	Params  []FuncParam
	Results []FuncParam
}

func (*ComponentFuncType) componentTypeDef() {}

func (ft *ComponentFuncType) unmarshalBinary(r *BinaryReader) error {
	// Params: count, then (name, valtype) for each
	paramCount, err := r.ReadU32()
	if err != nil {
		return err
	}
	if paramCount > MaxFunctionParams {
		return errfAt(r.Offset(), "func param count %d exceeds maximum %d", paramCount, MaxFunctionParams)
	}
	ft.Params = make([]FuncParam, 0, paramCount)
	for range paramCount {
		name, err := r.ReadString()
		if err != nil {
			return err
		}
		vt, err := readComponentValType(r)
		if err != nil {
			return err
		}
		ft.Params = append(ft.Params, FuncParam{Name: name, Type: vt})
	}
	// Results encoding:
	// 0x00 valtype = single unnamed result
	// 0x01 0x00    = named result list with 0 entries (no results)
	resultTag, err := r.ReadByte()
	if err != nil {
		return err
	}
	switch resultTag {
	case 0x00:
		vt, err := readComponentValType(r)
		if err != nil {
			return err
		}
		ft.Results = []FuncParam{{Name: "", Type: vt}}
	case 0x01:
		inner, err := r.ReadByte()
		if err != nil {
			return err
		}
		if inner != 0x00 {
			return errfAt(r.Offset()-1, "invalid leading byte (0x%x) for number of results", inner)
		}
		ft.Results = nil
	default:
		return errfAt(r.Offset()-1, "invalid leading byte (0x%x) for component function results", resultTag)
	}
	return nil
}

// ResourceType represents a resource type with a representation and optional destructor.
type ResourceType struct {
	Rep  ValType
	Dtor Optional[uint32]
}

func (*ResourceType) componentTypeDef() {}

func (rt *ResourceType) unmarshalBinary(r *BinaryReader) error {
	repByte, err := r.ReadByte()
	if err != nil {
		return err
	}
	switch repByte {
	case 0x7f:
		rt.Rep = ValTypeI32
	case 0x7e:
		rt.Rep = ValTypeI64
	default:
		return errfAt(r.Offset()-1, "unknown resource rep type: 0x%02x", repByte)
	}
	dtorFlag, err := r.ReadByte()
	if err != nil {
		return err
	}
	switch dtorFlag {
	case 0x00:
		// No destructor
	case 0x01:
		idx, err := r.ReadU32()
		if err != nil {
			return err
		}
		rt.Dtor = Some(idx)
	default:
		return errfAt(r.Offset()-1, "unknown resource dtor flag: 0x%02x", dtorFlag)
	}
	return nil
}

// InstanceTypeDeclaration is the union of declarations within an instance type.
// Variants: InstanceDeclCoreType, InstanceDeclType, InstanceDeclAlias, InstanceDeclExport.
type InstanceTypeDeclaration interface {
	instanceTypeDeclaration()
}

// InstanceDeclCoreType is a core type declaration within an instance type.
type InstanceDeclCoreType struct {
	Type CoreType
}

func (InstanceDeclCoreType) instanceTypeDeclaration()  {}
func (InstanceDeclCoreType) componentTypeDeclaration() {}

// InstanceDeclType is a type declaration within an instance type.
type InstanceDeclType struct {
	Type ComponentTypeDef
}

func (InstanceDeclType) instanceTypeDeclaration()  {}
func (InstanceDeclType) componentTypeDeclaration() {}

// InstanceDeclAlias is an alias declaration within an instance type.
type InstanceDeclAlias struct {
	Alias ComponentAlias
}

func (InstanceDeclAlias) instanceTypeDeclaration()  {}
func (InstanceDeclAlias) componentTypeDeclaration() {}

// InstanceDeclExport is an export declaration within an instance type.
type InstanceDeclExport struct {
	Export ComponentExport
}

func (InstanceDeclExport) instanceTypeDeclaration()  {}
func (InstanceDeclExport) componentTypeDeclaration() {}

// InstanceTypeDecl represents an instance type declaration (a list of declarations).
type InstanceTypeDecl struct {
	Declarations []InstanceTypeDeclaration
}

func (*InstanceTypeDecl) componentTypeDef() {}

func (it *InstanceTypeDecl) unmarshalBinary(r *BinaryReader) error {
	r.nesting++
	defer func() { r.nesting-- }()
	if r.nesting > MaxNestingDepth {
		return errfAt(r.Offset(), "instance type declaration nesting depth %d exceeds maximum %d", r.nesting, MaxNestingDepth)
	}
	count, err := r.ReadU32()
	if err != nil {
		return err
	}
	if count > MaxInstanceTypeDeclarations {
		return errfAt(r.Offset(), "instance type declaration count %d exceeds maximum %d", count, MaxInstanceTypeDeclarations)
	}
	it.Declarations = make([]InstanceTypeDeclaration, 0, count)
	for range count {
		decl, err := readInstanceTypeDeclaration(r)
		if err != nil {
			return err
		}
		it.Declarations = append(it.Declarations, decl)
	}
	return nil
}

// readInstanceTypeDeclaration reads a single InstanceTypeDeclaration.
func readInstanceTypeDeclaration(r *BinaryReader) (InstanceTypeDeclaration, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch b {
	case 0x00:
		// Core type
		ct, err := readCoreType(r)
		if err != nil {
			return nil, err
		}
		return InstanceDeclCoreType{Type: ct}, nil
	case 0x01:
		// Type
		ct, err := readComponentTypeDef(r)
		if err != nil {
			return nil, err
		}
		return InstanceDeclType{Type: ct}, nil
	case 0x02:
		// Alias in type declaration - uses the same format as top-level aliases
		alias, err := readComponentAlias(r)
		if err != nil {
			return nil, err
		}
		return InstanceDeclAlias{Alias: alias}, nil
	case 0x04:
		// Export in type declaration: exportname typeref
		// This is different from top-level exports which have sortidx + optional type
		var name ComponentExportName
		if err := name.unmarshalBinary(r); err != nil {
			return nil, err
		}
		tr, err := readComponentTypeRef(r)
		if err != nil {
			return nil, err
		}
		// Synthesize a ComponentExport with the type ref as the ascribed type
		// and dummy kind/index since in type declarations the export IS the type ref
		exp := ComponentExport{
			Name:         name,
			Kind:         typeRefToExternalKind(tr),
			Index:        0, // not used in type decl context
			AscribedType: Some(tr),
		}
		return InstanceDeclExport{Export: exp}, nil
	default:
		return nil, errfAt(r.Offset()-1, "unknown instance type declaration kind: 0x%02x", b)
	}
}

// ComponentTypeDeclaration is the union of declarations within a component type.
// All InstanceTypeDeclaration variants plus ComponentDeclImport.
type ComponentTypeDeclaration interface {
	componentTypeDeclaration()
}

// ComponentDeclImport is an import declaration within a component type.
type ComponentDeclImport struct {
	Import ComponentImport
}

func (ComponentDeclImport) componentTypeDeclaration() {}

// ComponentTypeDecl represents a component type declaration (a list of declarations).
type ComponentTypeDecl struct {
	Declarations []ComponentTypeDeclaration
}

func (*ComponentTypeDecl) componentTypeDef() {}

func (ct *ComponentTypeDecl) unmarshalBinary(r *BinaryReader) error {
	r.nesting++
	defer func() { r.nesting-- }()
	if r.nesting > MaxNestingDepth {
		return errfAt(r.Offset(), "component type declaration nesting depth %d exceeds maximum %d", r.nesting, MaxNestingDepth)
	}
	count, err := r.ReadU32()
	if err != nil {
		return err
	}
	if count > MaxComponentTypeDeclarations {
		return errfAt(r.Offset(), "component type declaration count %d exceeds maximum %d", count, MaxComponentTypeDeclarations)
	}
	ct.Declarations = make([]ComponentTypeDeclaration, 0, count)
	for range count {
		decl, err := readComponentTypeDeclaration(r)
		if err != nil {
			return err
		}
		ct.Declarations = append(ct.Declarations, decl)
	}
	return nil
}

// readComponentTypeDeclaration reads a single ComponentTypeDeclaration.
func readComponentTypeDeclaration(r *BinaryReader) (ComponentTypeDeclaration, error) {
	b, err := r.Peek()
	if err != nil {
		return nil, err
	}
	if b == 0x03 {
		// Import (component type only)
		_, _ = r.ReadByte() // already validated by Peek
		var imp ComponentImport
		if err := imp.unmarshalBinary(r); err != nil {
			return nil, err
		}
		return ComponentDeclImport{Import: imp}, nil
	}
	// Delegate to instance type declaration reader.
	// All InstanceTypeDeclaration concrete types also implement ComponentTypeDeclaration.
	decl, err := readInstanceTypeDeclaration(r)
	if err != nil {
		return nil, err
	}
	// The concrete types (InstanceDeclCoreType, InstanceDeclType, InstanceDeclAlias,
	// InstanceDeclExport) all implement both interfaces, so this assertion is safe.
	return decl.(ComponentTypeDeclaration), nil
}

// typeRefToExternalKind converts a ComponentTypeRef to a ComponentExternalKind.
func typeRefToExternalKind(tr ComponentTypeRef) ComponentExternalKind {
	switch tr.(type) {
	case TypeRefModule:
		return ExternalKindModule
	case TypeRefFunc:
		return ExternalKindFunc
	case TypeRefValue:
		return ExternalKindValue
	case TypeRefType:
		return ExternalKindType
	case TypeRefComponent:
		return ExternalKindComponent
	case TypeRefInstance:
		return ExternalKindInstance
	default:
		return ExternalKindFunc
	}
}

// readCoreType is defined in types_core.go

// readComponentTypeDef reads a ComponentTypeDef from the binary reader.
// Dispatches based on the opcode byte.
func readComponentTypeDef(r *BinaryReader) (ComponentTypeDef, error) {
	b, err := r.Peek()
	if err != nil {
		return nil, err
	}
	switch b {
	case 0x40:
		// Sync func type
		_, _ = r.ReadByte() // already validated by Peek
		ft := &ComponentFuncType{}
		if err := ft.unmarshalBinary(r); err != nil {
			return nil, err
		}
		return ft, nil
	case 0x41:
		// Component type decl
		_, _ = r.ReadByte() // already validated by Peek
		ct := &ComponentTypeDecl{}
		if err := ct.unmarshalBinary(r); err != nil {
			return nil, err
		}
		return ct, nil
	case 0x42:
		// Instance type decl
		_, _ = r.ReadByte() // already validated by Peek
		it := &InstanceTypeDecl{}
		if err := it.unmarshalBinary(r); err != nil {
			return nil, err
		}
		return it, nil
	case 0x3f:
		// Resource type
		_, _ = r.ReadByte() // already validated by Peek
		rt := &ResourceType{}
		if err := rt.unmarshalBinary(r); err != nil {
			return nil, err
		}
		return rt, nil
	default:
		// Try to read as a defined type
		dt, err := readComponentDefinedType(r)
		if err != nil {
			return nil, err
		}
		return &ComponentDefinedTypeEntry{Type: dt}, nil
	}
}
