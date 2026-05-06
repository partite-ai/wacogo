package wasmparser

// Core module section types for parsing actual wasm module sections
// (as opposed to component-level core type declarations).

// ModuleImport represents a single import in a core wasm module.
type ModuleImport struct {
	Module string
	Name   string
	Desc   ImportDesc
}

// ImportDesc is the union of import descriptor types.
type ImportDesc interface {
	importDesc()
}

// ImportDescFunc describes a function import by type index.
type ImportDescFunc struct {
	TypeIndex uint32
}

func (ImportDescFunc) importDesc() {}

// ImportDescTable describes a table import.
type ImportDescTable struct {
	Type CoreTableType
}

func (ImportDescTable) importDesc() {}

// ImportDescMemory describes a memory import.
type ImportDescMemory struct {
	Type CoreMemoryType
}

func (ImportDescMemory) importDesc() {}

// ImportDescGlobal describes a global import.
type ImportDescGlobal struct {
	Type CoreGlobalType
}

func (ImportDescGlobal) importDesc() {}

// ImportDescTag describes a tag import by type index.
type ImportDescTag struct {
	TypeIndex uint32
}

func (ImportDescTag) importDesc() {}

// ModuleExport represents a single export in a core wasm module.
type ModuleExport struct {
	Name  string
	Kind  byte // 0x00=func, 0x01=table, 0x02=memory, 0x03=global, 0x04=tag
	Index uint32
}

// readModuleImport reads a single import entry from a core module import section.
func readModuleImport(r *BinaryReader) (ModuleImport, error) {
	module, err := r.ReadString()
	if err != nil {
		return ModuleImport{}, err
	}
	name, err := r.ReadString()
	if err != nil {
		return ModuleImport{}, err
	}
	desc, err := readImportDesc(r)
	if err != nil {
		return ModuleImport{}, err
	}
	return ModuleImport{Module: module, Name: name, Desc: desc}, nil
}

func readImportDesc(r *BinaryReader) (ImportDesc, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch b {
	case 0x00:
		// Function: type index
		idx, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		return ImportDescFunc{TypeIndex: idx}, nil
	case 0x01:
		// Table
		tt, err := readModuleTableType(r)
		if err != nil {
			return nil, err
		}
		return ImportDescTable{Type: tt}, nil
	case 0x02:
		// Memory
		mem, err := readMemoryType(r)
		if err != nil {
			return nil, err
		}
		return ImportDescMemory{Type: mem}, nil
	case 0x03:
		// Global
		gt, err := readModuleGlobalType(r)
		if err != nil {
			return nil, err
		}
		return ImportDescGlobal{Type: gt}, nil
	case 0x04:
		// Tag: attribute byte (0x00=exception) then type index
		_, err := r.ReadByte() // attribute
		if err != nil {
			return nil, err
		}
		idx, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		return ImportDescTag{TypeIndex: idx}, nil
	default:
		return nil, errfAt(r.Offset()-1, "unknown import descriptor kind: 0x%02x", b)
	}
}

func readModuleTableType(r *BinaryReader) (CoreTableType, error) {
	elemByte, err := r.ReadByte()
	if err != nil {
		return CoreTableType{}, err
	}
	var elemType CoreValType
	switch elemByte {
	case 0x70:
		elemType = CoreValTypeFuncRef
	case 0x6f:
		elemType = CoreValTypeExternRef
	default:
		return CoreTableType{}, errfAt(r.Offset()-1, "unknown table element type: 0x%02x", elemByte)
	}
	min, max, err := readLimits(r)
	if err != nil {
		return CoreTableType{}, err
	}
	return CoreTableType{
		ElemType: elemType,
		Min:      uint32(min),
		Max:      max,
	}, nil
}

func readModuleGlobalType(r *BinaryReader) (CoreGlobalType, error) {
	vt, err := r.ReadByte()
	if err != nil {
		return CoreGlobalType{}, err
	}
	cvt, err := coreValTypeFromByte(vt)
	if err != nil {
		return CoreGlobalType{}, err
	}
	mut, err := r.ReadByte()
	if err != nil {
		return CoreGlobalType{}, err
	}
	return CoreGlobalType{ValType: cvt, Mutable: mut == 1}, nil
}

// readModuleExport reads a single export entry from a core module export section.
func readModuleExport(r *BinaryReader) (ModuleExport, error) {
	name, err := r.ReadString()
	if err != nil {
		return ModuleExport{}, err
	}
	kind, err := r.ReadByte()
	if err != nil {
		return ModuleExport{}, err
	}
	idx, err := r.ReadU32()
	if err != nil {
		return ModuleExport{}, err
	}
	return ModuleExport{Name: name, Kind: kind, Index: idx}, nil
}

// readModuleFuncTypeIndex reads a single type index from the function section.
func readModuleFuncTypeIndex(r *BinaryReader) (uint32, error) {
	return r.ReadU32()
}

// readModuleTableItem reads a single table type from the table section.
func readModuleTableItem(r *BinaryReader) (CoreTableType, error) {
	return readModuleTableType(r)
}

// readModuleMemoryItem reads a single memory type from the memory section.
func readModuleMemoryItem(r *BinaryReader) (CoreMemoryType, error) {
	return readMemoryType(r)
}

// readModuleCoreFuncType reads a core function type (prefixed with 0x60) from the type section.
func readModuleCoreFuncType(r *BinaryReader) (*CoreFuncType, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch b {
	case 0x60:
		return readCoreFuncType(r)
	case 0x4e:
		// Rec group
		count, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		var last *CoreFuncType
		for range count {
			ft, err := readModuleSubType(r)
			if err != nil {
				return nil, err
			}
			last = ft
		}
		if last == nil {
			return nil, errfAt(r.Offset(), "empty rec group")
		}
		return last, nil
	case 0x4f:
		// Sub type
		return readModuleSubTypeBody(r)
	default:
		return nil, errfAt(r.Offset()-1, "expected function type (0x60) or rec group, got 0x%02x", b)
	}
}

func readModuleSubType(r *BinaryReader) (*CoreFuncType, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch b {
	case 0x4f:
		return readModuleSubTypeBody(r)
	case 0x60:
		return readCoreFuncType(r)
	default:
		return nil, errfAt(r.Offset()-1, "expected sub type (0x4f) or function type (0x60), got 0x%02x", b)
	}
}

func readModuleSubTypeBody(r *BinaryReader) (*CoreFuncType, error) {
	// sub type: supertype count, supertypes..., composite type
	count, err := r.ReadU32()
	if err != nil {
		return nil, err
	}
	for range count {
		if _, err := r.ReadU32(); err != nil {
			return nil, err
		}
	}
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch b {
	case 0x60:
		return readCoreFuncType(r)
	default:
		return nil, errfAt(r.Offset()-1, "expected function type in sub type, got 0x%02x", b)
	}
}
