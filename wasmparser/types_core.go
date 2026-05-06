package wasmparser

// ComponentExternalKind identifies the kind of a component-level external definition.
type ComponentExternalKind uint8

const (
	ExternalKindModule    ComponentExternalKind = iota // 0x00 0x11
	ExternalKindFunc                                   // 0x01
	ExternalKindValue                                  // 0x02
	ExternalKindType                                   // 0x03
	ExternalKindComponent                              // 0x04
	ExternalKindInstance                               // 0x05
)

// readComponentExternalKind reads a ComponentExternalKind from the binary reader.
func readComponentExternalKind(r *BinaryReader) (ComponentExternalKind, error) {
	b, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	switch b {
	case 0x00:
		b2, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		if b2 != 0x11 {
			return 0, errfAt(r.Offset()-1, "expected 0x11 after 0x00 for module external kind, got 0x%02x", b2)
		}
		return ExternalKindModule, nil
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
		return 0, errfAt(r.Offset()-1, "unknown component external kind: 0x%02x", b)
	}
}

// CoreSort identifies a core WebAssembly sort used in core aliases and instances.
type CoreSort uint8

const (
	CoreSortFunc     CoreSort = iota // 0x00
	CoreSortTable                    // 0x01
	CoreSortMemory                   // 0x02
	CoreSortGlobal                   // 0x03
	CoreSortType                     // 0x10
	CoreSortModule                   // 0x11
	CoreSortInstance                 // 0x12
)

// readCoreSort reads a CoreSort from the binary reader.
func readCoreSort(r *BinaryReader) (CoreSort, error) {
	b, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
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
		return 0, errfAt(r.Offset()-1, "unknown core sort: 0x%02x", b)
	}
}

// ComponentOuterAliasKind identifies the kind of an outer alias.
type ComponentOuterAliasKind uint8

const (
	OuterAliasKindCoreType   ComponentOuterAliasKind = iota // 0x00 0x10
	OuterAliasKindCoreModule                                // 0x00 0x11
	OuterAliasKindType                                      // 0x03
	OuterAliasKindComponent                                 // 0x04
)

// CoreType is the union of types that can appear in a core type section within a component.
// Variants: CoreModuleType, CoreFuncType.
type CoreType interface {
	coreType()
}

// CoreValParam represents a single core wasm value type parameter, which may be
// a simple byte or a ref type with an index.
type CoreValParam struct {
	Byte     byte   // the raw value type byte (e.g. 0x7f for i32, 0x64 for ref)
	RefIndex uint32 // type index for ref types (only valid when Byte == 0x64)
}

// CoreFuncType represents a core WebAssembly function type (0x60).
type CoreFuncType struct {
	Params  []CoreValParam
	Results []CoreValParam
}

func (*CoreFuncType) coreType() {}

// ModuleTypeDeclaration is the union of declarations within a core module type.
type ModuleTypeDeclaration interface {
	moduleTypeDeclaration()
}

// ModuleDeclType is a function type declaration within a module type.
type ModuleDeclType struct {
	Params  []CoreValParam
	Results []CoreValParam
}

func (*ModuleDeclType) moduleTypeDeclaration() {}

// CoreEntityKind constants are in validator_types.go

// CoreEntityTypeRef is a parsed reference to a core entity type.
type CoreEntityTypeRef struct {
	Kind      CoreEntityKind
	TypeIndex uint32
	Table     CoreTableType
	Memory    CoreMemoryType
	Global    CoreGlobalType
}

// ModuleDeclImport is an import declaration within a module type.
type ModuleDeclImport struct {
	Module     string
	Name       string
	EntityType CoreEntityTypeRef
}

func (*ModuleDeclImport) moduleTypeDeclaration() {}

// ModuleDeclExport is an export declaration within a module type.
type ModuleDeclExport struct {
	Name       string
	EntityType CoreEntityTypeRef
}

func (*ModuleDeclExport) moduleTypeDeclaration() {}

// ModuleDeclOuterAlias is an outer alias declaration within a module type.
type ModuleDeclOuterAlias struct {
	Kind  ComponentOuterAliasKind
	Count uint32
	Index uint32
}

func (*ModuleDeclOuterAlias) moduleTypeDeclaration() {}

// CoreModuleType represents a core module type declaration.
type CoreModuleType struct {
	Declarations []ModuleTypeDeclaration
}

func (*CoreModuleType) coreType() {}

// readCoreType reads a CoreType from the reader.
func readCoreType(r *BinaryReader) (CoreType, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch b {
	case 0x60:
		// Core function type
		return readCoreFuncType(r)
	case 0x50:
		// Module type
		return readCoreModuleType(r)
	default:
		return nil, errfAt(r.Offset()-1, "unknown core type kind: 0x%02x", b)
	}
}

func readCoreValParam(r *BinaryReader) (CoreValParam, error) {
	b, err := r.ReadByte()
	if err != nil {
		return CoreValParam{}, err
	}
	if b == 0x64 {
		// ref type: 0x64 followed by type index
		idx, err := r.ReadU32()
		if err != nil {
			return CoreValParam{}, err
		}
		return CoreValParam{Byte: b, RefIndex: idx}, nil
	}
	return CoreValParam{Byte: b}, nil
}

func readCoreFuncType(r *BinaryReader) (*CoreFuncType, error) {
	// Params: vector of value types
	paramCount, err := r.ReadU32()
	if err != nil {
		return nil, err
	}
	if paramCount > MaxFunctionParams {
		return nil, errfAt(r.Offset(), "function param count %d exceeds maximum %d", paramCount, MaxFunctionParams)
	}
	params := make([]CoreValParam, paramCount)
	for i := range paramCount {
		p, err := readCoreValParam(r)
		if err != nil {
			return nil, err
		}
		params[i] = p
	}

	// Results: vector of value types
	resultCount, err := r.ReadU32()
	if err != nil {
		return nil, err
	}
	if resultCount > MaxFunctionResults {
		return nil, errfAt(r.Offset(), "function result count %d exceeds maximum %d", resultCount, MaxFunctionResults)
	}
	results := make([]CoreValParam, resultCount)
	for i := range resultCount {
		p, err := readCoreValParam(r)
		if err != nil {
			return nil, err
		}
		results[i] = p
	}

	return &CoreFuncType{Params: params, Results: results}, nil
}

func readCoreModuleType(r *BinaryReader) (*CoreModuleType, error) {
	count, err := r.ReadU32()
	if err != nil {
		return nil, err
	}
	if count > MaxModuleTypeDeclarations {
		return nil, errfAt(r.Offset(), "module type declaration count %d exceeds maximum %d", count, MaxModuleTypeDeclarations)
	}

	decls := make([]ModuleTypeDeclaration, 0, count)
	for range count {
		decl, err := readModuleTypeDeclaration(r)
		if err != nil {
			return nil, err
		}
		decls = append(decls, decl)
	}
	return &CoreModuleType{Declarations: decls}, nil
}

func readModuleTypeDeclaration(r *BinaryReader) (ModuleTypeDeclaration, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch b {
	case 0x00:
		// Import
		return readModuleDeclImport(r)
	case 0x01:
		// Type (sub-type rec group or function type)
		return readModuleDeclType(r)
	case 0x02:
		// Outer alias: 0x02 then kind(0x10=type) then 0x01 then count then index
		return readModuleDeclOuterAlias2(r)
	case 0x03:
		// Export
		return readModuleDeclExport(r)
	default:
		return nil, errfAt(r.Offset()-1, "unknown module type declaration kind: 0x%02x", b)
	}
}

func readModuleDeclType(r *BinaryReader) (*ModuleDeclType, error) {
	// Read the rec group / sub type wrapper
	// In a component's core type section, function types are wrapped in a rec group
	// that starts with 0x4e (rec) count=1, then 0x4f (sub) count=0, then 0x60 (func)
	// Or they can be a bare 0x60 func type.
	// We need to handle the rec group wrapper.
	b, err := r.Peek()
	if err != nil {
		return nil, err
	}

	switch b {
	case 0x60:
		// Bare function type
		_, _ = r.ReadByte() // already validated by Peek
		return readModuleDeclFuncType(r)
	case 0x4e:
		// Rec group: 0x4e count sub-types...
		_, _ = r.ReadByte() // already validated by Peek
		count, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		// Read each sub-type
		var last *ModuleDeclType
		for range count {
			st, err := readSubType(r)
			if err != nil {
				return nil, err
			}
			last = st
		}
		if last == nil {
			return nil, errfAt(r.Offset(), "empty rec group")
		}
		return last, nil
	case 0x4f:
		// Sub type without rec group wrapper
		_, _ = r.ReadByte() // already validated by Peek
		return readSubTypeBody(r)
	default:
		return nil, errfAt(r.Offset(), "expected function type (0x60) or rec group, got 0x%02x", b)
	}
}

func readSubType(r *BinaryReader) (*ModuleDeclType, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch b {
	case 0x4f:
		return readSubTypeBody(r)
	case 0x60:
		return readModuleDeclFuncType(r)
	default:
		return nil, errfAt(r.Offset()-1, "expected sub type (0x4f) or function type (0x60), got 0x%02x", b)
	}
}

func readSubTypeBody(r *BinaryReader) (*ModuleDeclType, error) {
	// sub type: supertype count, supertypes..., composite type
	count, err := r.ReadU32()
	if err != nil {
		return nil, err
	}
	// Skip super types
	for range count {
		_, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
	}
	// Read composite type
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch b {
	case 0x60:
		return readModuleDeclFuncType(r)
	default:
		return nil, errfAt(r.Offset()-1, "expected function type in sub type, got 0x%02x", b)
	}
}

func readModuleDeclFuncType(r *BinaryReader) (*ModuleDeclType, error) {
	paramCount, err := r.ReadU32()
	if err != nil {
		return nil, err
	}
	params := make([]CoreValParam, paramCount)
	for i := range paramCount {
		p, err := readCoreValParam(r)
		if err != nil {
			return nil, err
		}
		params[i] = p
	}
	resultCount, err := r.ReadU32()
	if err != nil {
		return nil, err
	}
	results := make([]CoreValParam, resultCount)
	for i := range resultCount {
		p, err := readCoreValParam(r)
		if err != nil {
			return nil, err
		}
		results[i] = p
	}
	return &ModuleDeclType{Params: params, Results: results}, nil
}

func readModuleDeclImport(r *BinaryReader) (*ModuleDeclImport, error) {
	module, err := r.ReadString()
	if err != nil {
		return nil, err
	}
	name, err := r.ReadString()
	if err != nil {
		return nil, err
	}
	et, err := readCoreEntityTypeRef(r)
	if err != nil {
		return nil, err
	}
	return &ModuleDeclImport{Module: module, Name: name, EntityType: et}, nil
}

func readModuleDeclExport(r *BinaryReader) (*ModuleDeclExport, error) {
	name, err := r.ReadString()
	if err != nil {
		return nil, err
	}
	et, err := readCoreEntityTypeRef(r)
	if err != nil {
		return nil, err
	}
	return &ModuleDeclExport{Name: name, EntityType: et}, nil
}

// readModuleDeclOuterAlias2 reads the Rust-style outer alias within module type declarations.
// Format: 0x10 (kind=Type) then 0x01 (target) then count then index.
func readModuleDeclOuterAlias2(r *BinaryReader) (*ModuleDeclOuterAlias, error) {
	kindByte, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	if kindByte != 0x10 {
		return nil, errfAt(r.Offset()-1, "expected outer alias kind 0x10 (type), got 0x%02x", kindByte)
	}
	targetByte, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	if targetByte != 0x01 {
		return nil, errfAt(r.Offset()-1, "expected outer alias target 0x01, got 0x%02x", targetByte)
	}
	count, err := r.ReadU32()
	if err != nil {
		return nil, err
	}
	idx, err := r.ReadU32()
	if err != nil {
		return nil, err
	}
	return &ModuleDeclOuterAlias{Kind: OuterAliasKindCoreType, Count: count, Index: idx}, nil
}

func readCoreEntityTypeRef(r *BinaryReader) (CoreEntityTypeRef, error) {
	b, err := r.ReadByte()
	if err != nil {
		return CoreEntityTypeRef{}, err
	}
	switch b {
	case 0x00:
		// Function: type index
		idx, err := r.ReadU32()
		if err != nil {
			return CoreEntityTypeRef{}, err
		}
		return CoreEntityTypeRef{Kind: CoreEntityFunc, TypeIndex: idx}, nil
	case 0x01:
		// Table: element type, limits
		elemTypeByte, err := r.ReadByte()
		if err != nil {
			return CoreEntityTypeRef{}, err
		}
		var elemCoreType CoreValType
		switch elemTypeByte {
		case 0x70:
			elemCoreType = CoreValTypeFuncRef
		case 0x6f:
			elemCoreType = CoreValTypeExternRef
		default:
			return CoreEntityTypeRef{}, errfAt(r.Offset()-1, "unknown table element type: 0x%02x", elemTypeByte)
		}
		min, max, err := readLimits(r)
		if err != nil {
			return CoreEntityTypeRef{}, err
		}
		return CoreEntityTypeRef{
			Kind: CoreEntityTable,
			Table: CoreTableType{
				ElemType: elemCoreType,
				Min:      uint32(min),
				Max:      max,
			},
		}, nil
	case 0x02:
		// Memory: limits
		mem, err := readMemoryType(r)
		if err != nil {
			return CoreEntityTypeRef{}, err
		}
		return CoreEntityTypeRef{Kind: CoreEntityMemory, Memory: mem}, nil
	case 0x03:
		// Global: value type, mutability
		vt, err := r.ReadByte()
		if err != nil {
			return CoreEntityTypeRef{}, err
		}
		mut, err := r.ReadByte()
		if err != nil {
			return CoreEntityTypeRef{}, err
		}
		cvt, _ := coreValTypeFromByte(vt)
		return CoreEntityTypeRef{
			Kind: CoreEntityGlobal,
			Global: CoreGlobalType{
				ValType: cvt,
				Mutable: mut == 1,
			},
		}, nil
	default:
		return CoreEntityTypeRef{}, errfAt(r.Offset()-1, "unknown core entity type kind: 0x%02x", b)
	}
}

func readLimits(r *BinaryReader) (uint64, Optional[uint32], error) {
	flags, err := r.ReadByte()
	if err != nil {
		return 0, None[uint32](), err
	}
	min, err := r.ReadU32()
	if err != nil {
		return 0, None[uint32](), err
	}
	if flags&0x01 != 0 {
		max, err := r.ReadU32()
		if err != nil {
			return 0, None[uint32](), err
		}
		return uint64(min), Some(max), nil
	}
	return uint64(min), None[uint32](), nil
}

func readMemoryType(r *BinaryReader) (CoreMemoryType, error) {
	flags, err := r.ReadByte()
	if err != nil {
		return CoreMemoryType{}, err
	}
	shared := flags&0x02 != 0
	mem64 := flags&0x04 != 0
	hasMax := flags&0x01 != 0

	var min uint64
	if mem64 {
		min, err = r.ReadU64()
	} else {
		var m32 uint32
		m32, err = r.ReadU32()
		min = uint64(m32)
	}
	if err != nil {
		return CoreMemoryType{}, err
	}

	var max Optional[uint64]
	if hasMax {
		var maxVal uint64
		if mem64 {
			maxVal, err = r.ReadU64()
		} else {
			var m32 uint32
			m32, err = r.ReadU32()
			maxVal = uint64(m32)
		}
		if err != nil {
			return CoreMemoryType{}, err
		}
		max = Some(maxVal)
	}

	// Validate memory limits
	maxPages := uint64(65536) // 4 GiB / 64 KiB
	if mem64 {
		maxPages = uint64(1) << 48 // 2^48 pages for memory64
	}
	if min > maxPages {
		return CoreMemoryType{}, errfAt(r.Offset(), "memory size must be at most %d pages", maxPages)
	}
	if max.Valid && max.Value > maxPages {
		return CoreMemoryType{}, errfAt(r.Offset(), "memory size must be at most %d pages", maxPages)
	}

	return CoreMemoryType{
		Min:    min,
		Max:    max,
		Shared: shared,
		Mem64:  mem64,
	}, nil
}
