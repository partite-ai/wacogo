package wasmparser

// ComponentInstance is the union of component instance definitions.
// Variants: Instantiate, InstantiateFromExports.
type ComponentInstance interface {
	componentInstance()
}

// Instantiate creates a component instance by instantiating a component with arguments.
type Instantiate struct {
	ComponentIndex uint32
	Args           []InstantiationArg
}

func (Instantiate) componentInstance() {}

// InstantiateFromExports creates a component instance from inline exports.
type InstantiateFromExports struct {
	Exports []ComponentExport
}

func (InstantiateFromExports) componentInstance() {}

// InstantiationArg is a single argument to a component instantiation.
type InstantiationArg struct {
	Name  string
	Kind  ComponentExternalKind
	Index uint32
}

// readComponentInstance reads a ComponentInstance from the binary reader.
func readComponentInstance(r *BinaryReader) (ComponentInstance, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch b {
	case 0x00:
		// Instantiate: component_idx, arg_count, args...
		compIdx, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		argCount, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		if argCount > MaxInstantiationArgs {
			return nil, errfAt(r.Offset(), "instantiation arg count %d exceeds maximum %d", argCount, MaxInstantiationArgs)
		}
		args := make([]InstantiationArg, 0, argCount)
		for range argCount {
			name, err := r.ReadString()
			if err != nil {
				return nil, err
			}
			kind, err := readComponentExternalKind(r)
			if err != nil {
				return nil, err
			}
			idx, err := r.ReadU32()
			if err != nil {
				return nil, err
			}
			args = append(args, InstantiationArg{Name: name, Kind: kind, Index: idx})
		}
		return Instantiate{ComponentIndex: compIdx, Args: args}, nil
	case 0x01:
		// From exports: count, exports (without type ascription)
		count, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		exports := make([]ComponentExport, 0, count)
		for range count {
			exp, err := readComponentExportNoType(r)
			if err != nil {
				return nil, err
			}
			exports = append(exports, exp)
		}
		return InstantiateFromExports{Exports: exports}, nil
	default:
		return nil, errfAt(r.Offset()-1, "unknown component instance kind: 0x%02x", b)
	}
}

// Instance is the union of core instance definitions (within a component).
// Variants: CoreInstantiate, CoreInstantiateFromExports.
type Instance interface {
	instance()
}

// CoreInstantiate creates a core instance by instantiating a module.
type CoreInstantiate struct {
	ModuleIndex uint32
	Args        []CoreInstantiationArg
}

func (CoreInstantiate) instance() {}

// CoreInstantiateFromExports creates a core instance from inline exports.
type CoreInstantiateFromExports struct {
	Exports []CoreExportItem
}

func (CoreInstantiateFromExports) instance() {}

// CoreInstantiationArg is a single argument to a core module instantiation.
type CoreInstantiationArg struct {
	Name  string
	Kind  CoreSort
	Index uint32
}

// CoreExportItem is a single export in a core instance from-exports.
type CoreExportItem struct {
	Name  string
	Kind  CoreSort
	Index uint32
}

// readInstance reads a core Instance from the binary reader.
func readInstance(r *BinaryReader) (Instance, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch b {
	case 0x00:
		// Instantiate module: module_idx, arg_count, args...
		modIdx, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		argCount, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		if argCount > MaxInstantiationArgs {
			return nil, errfAt(r.Offset(), "core instantiation arg count %d exceeds maximum %d", argCount, MaxInstantiationArgs)
		}
		args := make([]CoreInstantiationArg, 0, argCount)
		for range argCount {
			name, err := r.ReadString()
			if err != nil {
				return nil, err
			}
			kind, err := readCoreSort(r)
			if err != nil {
				return nil, err
			}
			idx, err := r.ReadU32()
			if err != nil {
				return nil, err
			}
			args = append(args, CoreInstantiationArg{Name: name, Kind: kind, Index: idx})
		}
		return CoreInstantiate{ModuleIndex: modIdx, Args: args}, nil
	case 0x01:
		// From exports: count, exports...
		count, err := r.ReadU32()
		if err != nil {
			return nil, err
		}
		exports := make([]CoreExportItem, 0, count)
		for range count {
			name, err := r.ReadString()
			if err != nil {
				return nil, err
			}
			kind, err := readCoreSort(r)
			if err != nil {
				return nil, err
			}
			idx, err := r.ReadU32()
			if err != nil {
				return nil, err
			}
			exports = append(exports, CoreExportItem{Name: name, Kind: kind, Index: idx})
		}
		return CoreInstantiateFromExports{Exports: exports}, nil
	default:
		return nil, errfAt(r.Offset()-1, "unknown core instance kind: 0x%02x", b)
	}
}
