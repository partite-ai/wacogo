package wasmparser

import (
	"fmt"
	"strings"
)

// ComponentKind indicates what kind of component context we're validating.
type ComponentKind uint8

const (
	ComponentKindComponent    ComponentKind = iota // real component
	ComponentKindInstanceType                      // inside (type (instance ...))
	ComponentKindComponentType                     // inside (type (component ...))
)

// ComponentState holds the validation state for a single component (or nested component/module).
type ComponentState struct {
	kind     ComponentKind
	features FeatureSet
	arena    *TypeArena

	// Core index spaces
	coreTypes     []CoreAnyTypeID
	coreFuncs     []CoreFuncTypeID
	coreTables    []CoreTableType
	coreMemories  []CoreMemoryType
	coreGlobals   []CoreGlobalType
	coreModules   []CoreModuleTypeID
	coreInstances []CoreInstanceTypeID

	// Component index spaces
	types      []ComponentAnyTypeID
	funcs      []ComponentFuncTypeID
	values     []valueEntry
	instances  []ComponentInstanceTypeID
	components []ComponentTypeID

	// Import/export tracking
	imports     map[string]ComponentEntityType
	importOrder []string // preserves insertion order
	exports     map[string]ComponentEntityType
	exportOrder []string // preserves insertion order
	importNameSet map[string]string // lowercase name -> original name for conflict detection
	exportNameSet map[string]string // lowercase name -> original name for conflict detection
	// importResourceNames tracks resource names referenced by bracket-prefixed imports.
	// Maps lowercase resource name -> original import name that introduced it.
	importResourceNames map[string]string

	// localResources tracks resource IDs that were locally defined (via resource type)
	// as opposed to imported.
	localResources map[ResourceID]bool

	// locallyDefinedSubResources tracks resource IDs introduced inside this
	// component state via `(type (sub resource))` bounds — i.e. abstract
	// resources declared by this scope itself, not aliased in from an outer
	// scope. Used by instance type declarations to correctly report their
	// DefinedResources without including outer-aliased resources.
	locallyDefinedSubResources map[ResourceID]bool

	// resourceNames maps resource ID -> all extern names that refer to this
	// resource in the current context (from imports, exports, or re-exports).
	// A resource can have multiple names if it is exported/imported under
	// several aliases, e.g. `(export "r" (type $r))` plus
	// `(export "r-alt" (type $r))`. Bracket-prefixed func names
	// (`[constructor]X`, `[method]X.Y`, `[static]X.Y`) match if X equals any
	// entry in the slice.
	resourceNames map[ResourceID][]string

	// exportResourceNamesMap is the subset of resourceNames contributed by
	// export declarations. Exports may only reference resources that appear
	// here (imports can reference anything in resourceNames).
	exportResourceNamesMap map[ResourceID][]string

	// importedResourceIDs tracks resource IDs that have been imported (or are accessible
	// through imports). Imported resources are valid for use in both imports and exports.
	importedResourceIDs map[ResourceID]bool
	// exportedResourceIDs tracks resource IDs that have been exported.
	// Exported resources are only valid for use in exports.
	exportedResourceIDs map[ResourceID]bool

	// importedDefinedTypes tracks defined-type arena IDs that have been imported.
	// Required-name types (record/variant/enum/flags) referenced inside imports must
	// appear here. Imports populate both this and exportedDefinedTypes.
	importedDefinedTypes map[ComponentDefinedTypeID]bool
	// exportedDefinedTypes tracks defined-type arena IDs that have been exported.
	exportedDefinedTypes map[ComponentDefinedTypeID]bool

	hasStart bool
}

type valueEntry struct {
	ty ValTypeDesc
}

func newComponentState(features FeatureSet, arena *TypeArena, kind ComponentKind) *ComponentState {
	return &ComponentState{
		kind:                kind,
		features:            features,
		arena:               arena,
		imports:             make(map[string]ComponentEntityType),
		exports:             make(map[string]ComponentEntityType),
		importNameSet:       make(map[string]string),
		exportNameSet:       make(map[string]string),
		importResourceNames: make(map[string]string),
		localResources:             make(map[ResourceID]bool),
		locallyDefinedSubResources: make(map[ResourceID]bool),
		resourceNames:          make(map[ResourceID][]string),
		exportResourceNamesMap: make(map[ResourceID][]string),
		importedResourceIDs:    make(map[ResourceID]bool),
		exportedResourceIDs:    make(map[ResourceID]bool),
		importedDefinedTypes:   make(map[ComponentDefinedTypeID]bool),
		exportedDefinedTypes:   make(map[ComponentDefinedTypeID]bool),
	}
}

// ---------- Index space bounds checking ----------

func (cs *ComponentState) getCoreType(idx uint32) (CoreAnyTypeID, error) {
	if idx >= uint32(len(cs.coreTypes)) {
		return CoreAnyTypeID{}, fmt.Errorf("core type index out of bounds")
	}
	return cs.coreTypes[idx], nil
}

func (cs *ComponentState) getCoreFunc(idx uint32) (CoreFuncTypeID, error) {
	if idx >= uint32(len(cs.coreFuncs)) {
		return 0, fmt.Errorf("core function index out of bounds")
	}
	return cs.coreFuncs[idx], nil
}

func (cs *ComponentState) getCoreTable(idx uint32) (CoreTableType, error) {
	if idx >= uint32(len(cs.coreTables)) {
		return CoreTableType{}, fmt.Errorf("core table index out of bounds")
	}
	return cs.coreTables[idx], nil
}

func (cs *ComponentState) getCoreMemory(idx uint32) (CoreMemoryType, error) {
	if idx >= uint32(len(cs.coreMemories)) {
		return CoreMemoryType{}, fmt.Errorf("core memory index out of bounds")
	}
	return cs.coreMemories[idx], nil
}

func (cs *ComponentState) getCoreGlobal(idx uint32) (CoreGlobalType, error) {
	if idx >= uint32(len(cs.coreGlobals)) {
		return CoreGlobalType{}, fmt.Errorf("core global index out of bounds")
	}
	return cs.coreGlobals[idx], nil
}

func (cs *ComponentState) getCoreModule(idx uint32) (CoreModuleTypeID, error) {
	if idx >= uint32(len(cs.coreModules)) {
		return 0, fmt.Errorf("unknown module %d: module index out of bounds", idx)
	}
	return cs.coreModules[idx], nil
}

func (cs *ComponentState) getCoreInstance(idx uint32) (CoreInstanceTypeID, error) {
	if idx >= uint32(len(cs.coreInstances)) {
		return 0, fmt.Errorf("instance index out of bounds")
	}
	return cs.coreInstances[idx], nil
}

func (cs *ComponentState) getType(idx uint32) (ComponentAnyTypeID, error) {
	if idx >= uint32(len(cs.types)) {
		return ComponentAnyTypeID{}, fmt.Errorf("type index out of bounds")
	}
	return cs.types[idx], nil
}

func (cs *ComponentState) getFunc(idx uint32) (ComponentFuncTypeID, error) {
	if idx >= uint32(len(cs.funcs)) {
		return 0, fmt.Errorf("function index out of bounds")
	}
	return cs.funcs[idx], nil
}

func (cs *ComponentState) getValue(idx uint32) (*valueEntry, error) {
	if idx >= uint32(len(cs.values)) {
		return nil, fmt.Errorf("value index out of bounds")
	}
	return &cs.values[idx], nil
}

func (cs *ComponentState) getInstance(idx uint32) (ComponentInstanceTypeID, error) {
	if idx >= uint32(len(cs.instances)) {
		return 0, fmt.Errorf("instance index out of bounds")
	}
	return cs.instances[idx], nil
}

func (cs *ComponentState) getComponent(idx uint32) (ComponentTypeID, error) {
	if idx >= uint32(len(cs.components)) {
		return 0, fmt.Errorf("unknown component %d: component index out of bounds", idx)
	}
	return cs.components[idx], nil
}

// ---------- Name validation ----------

// importNameKey returns the key used for import name uniqueness checking.
// Import-only name formats (integrity, URL, dep) are case-sensitive.
// Other names (kebab, interface, bracket) are case-insensitive.
func importNameKey(name string) string {
	if strings.HasPrefix(name, "integrity=") ||
		strings.HasPrefix(name, "url=") ||
		strings.HasPrefix(name, "locked-dep=") ||
		strings.HasPrefix(name, "unlocked-dep=") {
		return name
	}
	return strings.ToLower(name)
}

// checkImportName validates import name uniqueness and format.
func (cs *ComponentState) checkImportName(name string) error {
	key := importNameKey(name)
	if prev, ok := cs.importNameSet[key]; ok {
		return fmt.Errorf("import name `%s` conflicts with previous name `%s`", name, prev)
	}

	// Check resource/method name conflicts per Component Model spec:
	// A plain label "x" conflicts with [method]x.x or [static]x.x
	// (when both the resource name and method name equal the label).
	resName, methodName := extractMethodResourceNames(name)
	if resName != "" && methodName != "" {
		lowerRes := strings.ToLower(resName)
		lowerMethod := strings.ToLower(methodName)
		// [method]R.M or [static]R.M conflicts with plain label if R==M==label
		if lowerRes == lowerMethod {
			if prev, ok := cs.importNameSet[lowerRes]; ok {
				return fmt.Errorf("import name `%s` conflicts with previous name `%s`", name, prev)
			}
		}
		// Register for reverse lookups
		if lowerRes == lowerMethod {
			cs.importResourceNames[lowerRes] = name
		}
	} else if !strings.HasPrefix(name, "[") &&
		!strings.HasPrefix(name, "integrity=") &&
		!strings.HasPrefix(name, "url=") &&
		!strings.HasPrefix(name, "locked-dep=") &&
		!strings.HasPrefix(name, "unlocked-dep=") {
		// Plain name — check if any [method]x.x or [static]x.x was previously imported
		lower := strings.ToLower(name)
		if prev, ok := cs.importResourceNames[lower]; ok {
			return fmt.Errorf("import name `%s` conflicts with previous name `%s`", name, prev)
		}
	}

	cs.importNameSet[key] = name
	return nil
}

// extractMethodResourceNames extracts the resource and method names from a
// bracket-prefixed [method] or [static] import name. Returns ("", "") for
// non-method/static names (including [constructor]).
func extractMethodResourceNames(name string) (resource, method string) {
	var rest string
	if strings.HasPrefix(name, "[method]") {
		rest = name[len("[method]"):]
	} else if strings.HasPrefix(name, "[static]") {
		rest = name[len("[static]"):]
	} else {
		return "", ""
	}
	resource, method, ok := strings.Cut(rest, ".")
	if !ok {
		return "", ""
	}
	return resource, method
}

// checkExportName validates export name uniqueness and format.
func (cs *ComponentState) checkExportName(name string) error {
	lower := strings.ToLower(name)
	if prev, ok := cs.exportNameSet[lower]; ok {
		return fmt.Errorf("export name `%s` conflicts with previous name `%s`", name, prev)
	}
	cs.exportNameSet[lower] = name
	return nil
}

// ---------- Type ref to entity type ----------

func (cs *ComponentState) typeRefToEntityType(tr ComponentTypeRef) (ComponentEntityType, error) {
	switch ref := tr.(type) {
	case TypeRefModule:
		idx := ref.Index
		ct, err := cs.getCoreType(idx)
		if err != nil {
			return ComponentEntityType{}, err
		}
		if ct.Kind != CoreAnyTypeModule {
			return ComponentEntityType{}, fmt.Errorf("core type index %d is not a module type", idx)
		}
		return ComponentEntityType{Kind: EntityModule, ModuleID: CoreModuleTypeID(ct.Index)}, nil
	case TypeRefFunc:
		idx := ref.Index
		ty, err := cs.getType(idx)
		if err != nil {
			return ComponentEntityType{}, err
		}
		if ty.Kind != AnyTypeFunc {
			return ComponentEntityType{}, fmt.Errorf("type index %d is not a function type", idx)
		}
		return ComponentEntityType{Kind: EntityFunc, FuncID: ComponentFuncTypeID(ty.Index)}, nil
	case TypeRefValue:
		ivt, err := internalValTypeFromParsed(ref.Type, cs)
		if err != nil {
			return ComponentEntityType{}, err
		}
		return ComponentEntityType{Kind: EntityValue, ValType: ivt}, nil
	case TypeRefType:
		switch bounds := ref.Bounds.(type) {
		case TypeBoundsEq:
			ty, err := cs.getType(bounds.Index)
			if err != nil {
				return ComponentEntityType{}, err
			}
			return ComponentEntityType{Kind: EntityType, TypeRef: ty}, nil
		case TypeBoundsSubResource:
			// Sub-resource bound: allocate a new resource
			rid := cs.arena.allocResourceID()
			cs.locallyDefinedSubResources[rid] = true
			return ComponentEntityType{
				Kind:    EntityType,
				TypeRef: ComponentAnyTypeID{Kind: AnyTypeResource, ResID: rid},
			}, nil
		default:
			return ComponentEntityType{}, fmt.Errorf("unknown type bounds %T", ref.Bounds)
		}
	case TypeRefComponent:
		idx := ref.Index
		ty, err := cs.getType(idx)
		if err != nil {
			return ComponentEntityType{}, err
		}
		if ty.Kind != AnyTypeComponent {
			return ComponentEntityType{}, fmt.Errorf("type index %d is not a component type", idx)
		}
		return ComponentEntityType{Kind: EntityComponent, CompID: ComponentTypeID(ty.Index)}, nil
	case TypeRefInstance:
		idx := ref.Index
		ty, err := cs.getType(idx)
		if err != nil {
			return ComponentEntityType{}, err
		}
		if ty.Kind != AnyTypeInstance {
			return ComponentEntityType{}, fmt.Errorf("type index %d is not an instance type", idx)
		}
		return ComponentEntityType{Kind: EntityInstance, InstID: ComponentInstanceTypeID(ty.Index)}, nil
	default:
		return ComponentEntityType{}, fmt.Errorf("unknown type ref %T", tr)
	}
}

// addEntity adds an entity to the appropriate index space.
func (cs *ComponentState) addEntity(et ComponentEntityType) error {
	switch et.Kind {
	case EntityModule:
		cs.coreModules = append(cs.coreModules, et.ModuleID)
	case EntityFunc:
		cs.funcs = append(cs.funcs, et.FuncID)
	case EntityValue:
		cs.values = append(cs.values, valueEntry{ty: et.ValType})
	case EntityType:
		cs.types = append(cs.types, et.TypeRef)
	case EntityInstance:
		cs.instances = append(cs.instances, et.InstID)
	case EntityComponent:
		cs.components = append(cs.components, et.CompID)
	}
	return nil
}
