package wasmparser

import "fmt"

// ---------- Typed IDs ----------

// CoreFuncTypeID identifies a core function type in the type arena.
type CoreFuncTypeID uint32

// CoreModuleTypeID identifies a core module type in the type arena.
type CoreModuleTypeID uint32

// CoreInstanceTypeID identifies a core instance type in the type arena.
type CoreInstanceTypeID uint32

// ComponentDefinedTypeID identifies a component defined type in the type arena.
type ComponentDefinedTypeID uint32

// ComponentFuncTypeID identifies a component function type in the type arena.
type ComponentFuncTypeID uint32

// ComponentInstanceTypeID identifies a component instance type in the type arena.
type ComponentInstanceTypeID uint32

// ComponentTypeID identifies a component type (with imports/exports) in the type arena.
type ComponentTypeID uint32

// ResourceID identifies a unique resource.
type ResourceID uint32

// ---------- Component-level type enum for index spaces ----------

// ComponentAnyTypeID is the union of all component type IDs that go in the `types` index space.
type ComponentAnyTypeID struct {
	Kind    ComponentAnyTypeKind
	Index   uint32 // arena index for defined/func/instance/component
	ResID   ResourceID
}

type ComponentAnyTypeKind uint8

const (
	AnyTypeDefined   ComponentAnyTypeKind = iota
	AnyTypeFunc
	AnyTypeInstance
	AnyTypeComponent
	AnyTypeResource
)

func (id ComponentAnyTypeID) Desc() string {
	switch id.Kind {
	case AnyTypeDefined:
		return "defined type"
	case AnyTypeFunc:
		return "func"
	case AnyTypeInstance:
		return "instance"
	case AnyTypeComponent:
		return "component"
	case AnyTypeResource:
		return "resource"
	}
	return "unknown"
}

// ---------- Core type enum for core_types index space ----------

type CoreAnyTypeID struct {
	Kind  CoreAnyTypeKind
	Index uint32
}

type CoreAnyTypeKind uint8

const (
	CoreAnyTypeFunc   CoreAnyTypeKind = iota
	CoreAnyTypeModule
)

// ---------- Entity types ----------

// ComponentEntityType is what an import or export resolves to.
type ComponentEntityType struct {
	Kind     EntityKind
	FuncID   ComponentFuncTypeID
	ModuleID CoreModuleTypeID
	InstID   ComponentInstanceTypeID
	CompID   ComponentTypeID
	ValType  ValTypeDesc
	TypeRef  ComponentAnyTypeID // for Kind==EntityType
}

type EntityKind uint8

const (
	EntityModule    EntityKind = iota
	EntityFunc
	EntityValue
	EntityType
	EntityInstance
	EntityComponent
)

// entityExpectedName returns the name the spec error messages use when this
// kind appears on the "expected" side of a type-mismatch error.
func entityExpectedName(k EntityKind) string {
	switch k {
	case EntityModule:
		return "module"
	case EntityFunc:
		return "function"
	case EntityValue:
		return "value"
	case EntityType:
		return "resource"
	case EntityInstance:
		return "instance"
	case EntityComponent:
		return "component"
	}
	return "unknown"
}

// entityFoundName returns the name the spec error messages use when this
// kind appears on the "found" side of a type-mismatch error.
func entityFoundName(k EntityKind) string {
	switch k {
	case EntityModule:
		return "module"
	case EntityFunc:
		return "func"
	case EntityValue:
		return "value"
	case EntityType:
		return "type"
	case EntityInstance:
		return "instance"
	case EntityComponent:
		return "component"
	}
	return "unknown"
}

func (e ComponentEntityType) Desc() string {
	switch e.Kind {
	case EntityModule:
		return "module"
	case EntityFunc:
		return "func"
	case EntityValue:
		return "value"
	case EntityType:
		return "type"
	case EntityInstance:
		return "instance"
	case EntityComponent:
		return "component"
	}
	return "unknown"
}

// ---------- Internal value types ----------

// ValTypeDesc is the resolved form of ComponentValType.
type ValTypeDesc struct {
	IsPrimitive bool
	Primitive   PrimitiveValType
	TypeID      ComponentDefinedTypeID
}

func internalValTypeFromParsed(cvt ComponentValType, cs *ComponentState) (ValTypeDesc, error) {
	switch v := cvt.(type) {
	case PrimitiveValType:
		return ValTypeDesc{IsPrimitive: true, Primitive: v}, nil
	case TypeIndexValType:
		idx := uint32(v)
		if idx >= uint32(len(cs.types)) {
			return ValTypeDesc{}, fmt.Errorf("type index out of bounds")
		}
		ty := cs.types[idx]
		if ty.Kind != AnyTypeDefined {
			return ValTypeDesc{}, fmt.Errorf("type index %d is not a defined type", idx)
		}
		return ValTypeDesc{IsPrimitive: false, TypeID: ComponentDefinedTypeID(ty.Index)}, nil
	default:
		return ValTypeDesc{}, fmt.Errorf("unknown component val type %T", cvt)
	}
}

// ---------- Arena stored types ----------

// CoreFuncTypeDesc is the internal representation of a core function type.
type CoreFuncTypeDesc struct {
	Params  []CoreValType
	Results []CoreValType
}

// CoreValType is a core wasm value type.
type CoreValType uint8

const (
	CoreValTypeI32     CoreValType = iota
	CoreValTypeI64
	CoreValTypeF32
	CoreValTypeF64
	CoreValTypeV128
	CoreValTypeFuncRef
	CoreValTypeExternRef
)

// CoreModuleTypeDesc holds the information about a module type.
type CoreModuleTypeDesc struct {
	Imports     map[importKey]CoreEntityType // (module, name) -> entity type
	ImportOrder []importKey                  // preserves insertion order for deterministic iteration
	Exports     map[string]CoreEntityType
}

type importKey struct {
	Module string
	Name   string
}

// CoreEntityType is the type of a core wasm import/export.
type CoreEntityType struct {
	Kind CoreEntityKind
	Func CoreFuncTypeID
	// For tables, memories, globals we store simplified info
	Table  CoreTableType
	Memory CoreMemoryType
	Global CoreGlobalType
}

type CoreEntityKind uint8

const (
	CoreEntityFunc   CoreEntityKind = iota
	CoreEntityTable
	CoreEntityMemory
	CoreEntityGlobal
)

type CoreTableType struct {
	ElemType CoreValType
	Min      uint32
	Max      Optional[uint32]
}

type CoreMemoryType struct {
	Min    uint64
	Max    Optional[uint64]
	Shared bool
	Mem64  bool
}

type CoreGlobalType struct {
	ValType CoreValType
	Mutable bool
}

// CoreInstanceTypeDesc holds exports of a core instance.
type CoreInstanceTypeDesc struct {
	Exports map[string]CoreEntityType
}

// DefinedTypeDesc is the resolved form of a defined type.
type DefinedTypeDesc struct {
	Kind ComponentDefinedTypeKind
	// Fields depend on Kind
	Primitive PrimitiveValType // only valid when Kind == DefinedKindPrimitive
	Record    *RecordTypeDesc
	Variant   *VariantTypeDesc
	List      ValTypeDesc
	Tuple     []ValTypeDesc
	Flags     []string
	Enum      []string
	Option    ValTypeDesc
	ResultOk  Optional[ValTypeDesc]
	ResultErr Optional[ValTypeDesc]
	Own       ResourceID
	Borrow    ResourceID
}

type ComponentDefinedTypeKind uint8

const (
	DefinedKindPrimitive ComponentDefinedTypeKind = iota
	DefinedKindRecord
	DefinedKindVariant
	DefinedKindList
	DefinedKindTuple
	DefinedKindFlags
	DefinedKindEnum
	DefinedKindOption
	DefinedKindResult
	DefinedKindOwn
	DefinedKindBorrow
)

type RecordTypeDesc struct {
	Fields []FieldDesc
}

type FieldDesc struct {
	Name string
	Type ValTypeDesc
}

type VariantTypeDesc struct {
	Cases []VariantCaseDesc
}

type VariantCaseDesc struct {
	Name    string
	Type    Optional[ValTypeDesc]
	Refines Optional[uint32]
}

// FuncTypeDesc is the resolved form of a component function type.
type FuncTypeDesc struct {
	Params  []FuncParamDesc
	Results []FuncParamDesc
}

type FuncParamDesc struct {
	Name string
	Type ValTypeDesc
}

// InstanceTypeDesc holds the exports of a component instance.
type InstanceTypeDesc struct {
	Exports          map[string]ComponentEntityType
	DefinedResources []ResourceID // resources defined (not just referenced) by this instance type
}

// ComponentTypeDesc holds imports and exports of a component type.
type ComponentTypeDesc struct {
	Imports     map[string]ComponentEntityType
	ImportOrder []string // preserves insertion order for deterministic iteration
	Exports     map[string]ComponentEntityType
	ExportOrder []string // preserves insertion order for deterministic iteration
}

// ---------- Type Arena ----------

// TypeArena stores all types across nesting levels.
type TypeArena struct {
	features FeatureSet

	CoreFuncTypes     []CoreFuncTypeDesc
	CoreModuleTypes   []CoreModuleTypeDesc
	CoreInstanceTypes []CoreInstanceTypeDesc

	DefinedTypes  []DefinedTypeDesc
	FuncTypes     []FuncTypeDesc
	InstanceTypes []InstanceTypeDesc
	ComponentTypes []ComponentTypeDesc

	// Per-type effective sizes used to enforce MaxEffectiveTypeSize. Each slice
	// is parallel to the matching descriptor slice above. Sizes are computed
	// when the descriptor is pushed; for component types built via placeholder
	// (validateComponentSection / validateEnd) the entry is overwritten once
	// the descriptor is finalized.
	DefinedTypeSizes   []uint32
	FuncTypeSizes      []uint32
	InstanceTypeSizes  []uint32
	ComponentTypeSizes []uint32

	nextResourceID ResourceID

	// resourceAliases maps a freshly minted "export-alias" ResourceID to the
	// underlying defining ResourceID it aliases. Two-level only — fresh →
	// defining, no chains. Populated by freshenTypeEntityCreated and queried
	// by SubtypeChecker.checkResourceMatch / mintMapped to resolve cross-
	// component re-exports back to the resource they ultimately reference.
	// Keyed globally because ResourceIDs are unique across the arena and the
	// alias relationship is stable for the lifetime of the arena.
	resourceAliases map[ResourceID]ResourceID
}

func newTypeArena(features FeatureSet) *TypeArena {
	return &TypeArena{
		features:        features,
		resourceAliases: make(map[ResourceID]ResourceID),
	}
}

// NewArena returns a standalone *TypeArena suitable for host code that
// constructs component types outside the normal parse flow. The features
// set governs validation of any core-module bytes parsed via
// (*TypeArena).CoreModuleTypeFromBytes.
func NewArena(features FeatureSet) *TypeArena {
	return newTypeArena(features)
}

// resolveResourceAlias returns the defining ResourceID for rid if rid is the
// fresh-export-alias half of a recorded pair, else returns rid unchanged.
// Single hop only — defining IDs never alias further.
func (a *TypeArena) resolveResourceAlias(rid ResourceID) ResourceID {
	if a == nil {
		return rid
	}
	if defining, ok := a.resourceAliases[rid]; ok {
		return defining
	}
	return rid
}

func (a *TypeArena) pushCoreFuncType(t CoreFuncTypeDesc) CoreFuncTypeID {
	id := CoreFuncTypeID(len(a.CoreFuncTypes))
	a.CoreFuncTypes = append(a.CoreFuncTypes, t)
	return id
}

func (a *TypeArena) pushCoreModuleType(t CoreModuleTypeDesc) CoreModuleTypeID {
	id := CoreModuleTypeID(len(a.CoreModuleTypes))
	a.CoreModuleTypes = append(a.CoreModuleTypes, t)
	return id
}

func (a *TypeArena) pushCoreInstanceType(t CoreInstanceTypeDesc) CoreInstanceTypeID {
	id := CoreInstanceTypeID(len(a.CoreInstanceTypes))
	a.CoreInstanceTypes = append(a.CoreInstanceTypes, t)
	return id
}

func (a *TypeArena) pushDefinedType(t DefinedTypeDesc) ComponentDefinedTypeID {
	id := ComponentDefinedTypeID(len(a.DefinedTypes))
	a.DefinedTypes = append(a.DefinedTypes, t)
	a.DefinedTypeSizes = append(a.DefinedTypeSizes, a.computeDefinedTypeSize(&t))
	return id
}

func (a *TypeArena) pushFuncType(t FuncTypeDesc) ComponentFuncTypeID {
	id := ComponentFuncTypeID(len(a.FuncTypes))
	a.FuncTypes = append(a.FuncTypes, t)
	a.FuncTypeSizes = append(a.FuncTypeSizes, a.computeFuncTypeSize(&t))
	return id
}

func (a *TypeArena) pushInstanceType(t InstanceTypeDesc) ComponentInstanceTypeID {
	id := ComponentInstanceTypeID(len(a.InstanceTypes))
	a.InstanceTypes = append(a.InstanceTypes, t)
	a.InstanceTypeSizes = append(a.InstanceTypeSizes, a.computeInstanceTypeSize(&t))
	return id
}

func (a *TypeArena) pushComponentType(t ComponentTypeDesc) ComponentTypeID {
	id := ComponentTypeID(len(a.ComponentTypes))
	a.ComponentTypes = append(a.ComponentTypes, t)
	a.ComponentTypeSizes = append(a.ComponentTypeSizes, a.computeComponentTypeSize(&t))
	return id
}

func (a *TypeArena) allocResourceID() ResourceID {
	id := a.nextResourceID
	a.nextResourceID++
	return id
}

// ---------- Effective type size ----------

// satAddU32 returns a+b, saturating at math.MaxUint32 to avoid overflow on
// pathological inputs. Sizes that saturate will exceed MaxEffectiveTypeSize and
// be rejected by the validator.
func satAddU32(a, b uint32) uint32 {
	s := a + b
	if s < a {
		return ^uint32(0)
	}
	return s
}

// valTypeSize returns the effective size of a resolved component val type.
func (a *TypeArena) valTypeSize(vt ValTypeDesc) uint32 {
	if vt.IsPrimitive {
		return 1
	}
	idx := uint32(vt.TypeID)
	if idx >= uint32(len(a.DefinedTypeSizes)) {
		return 1
	}
	return a.DefinedTypeSizes[idx]
}

// entityTypeSize returns the effective size of an import/export entity. Module
// and Type entities are treated as size-1 references because their contents do
// not contribute to the component-model effective-size limit being enforced.
func (a *TypeArena) entityTypeSize(et ComponentEntityType) uint32 {
	switch et.Kind {
	case EntityModule, EntityType:
		return 1
	case EntityFunc:
		idx := uint32(et.FuncID)
		if idx >= uint32(len(a.FuncTypeSizes)) {
			return 1
		}
		return a.FuncTypeSizes[idx]
	case EntityValue:
		return a.valTypeSize(et.ValType)
	case EntityInstance:
		idx := uint32(et.InstID)
		if idx >= uint32(len(a.InstanceTypeSizes)) {
			return 1
		}
		return a.InstanceTypeSizes[idx]
	case EntityComponent:
		idx := uint32(et.CompID)
		if idx >= uint32(len(a.ComponentTypeSizes)) {
			return 1
		}
		return a.ComponentTypeSizes[idx]
	}
	return 1
}

func (a *TypeArena) computeDefinedTypeSize(t *DefinedTypeDesc) uint32 {
	switch t.Kind {
	case DefinedKindPrimitive, DefinedKindFlags, DefinedKindEnum,
		DefinedKindOwn, DefinedKindBorrow:
		return 1
	case DefinedKindRecord:
		size := uint32(1)
		if t.Record != nil {
			for _, f := range t.Record.Fields {
				size = satAddU32(size, a.valTypeSize(f.Type))
			}
		}
		return size
	case DefinedKindVariant:
		size := uint32(1)
		if t.Variant != nil {
			for _, c := range t.Variant.Cases {
				if c.Type.Valid {
					size = satAddU32(size, a.valTypeSize(c.Type.Value))
				} else {
					size = satAddU32(size, 1)
				}
			}
		}
		return size
	case DefinedKindList:
		return satAddU32(1, a.valTypeSize(t.List))
	case DefinedKindTuple:
		size := uint32(1)
		for _, v := range t.Tuple {
			size = satAddU32(size, a.valTypeSize(v))
		}
		return size
	case DefinedKindOption:
		return satAddU32(1, a.valTypeSize(t.Option))
	case DefinedKindResult:
		size := uint32(1)
		if t.ResultOk.Valid {
			size = satAddU32(size, a.valTypeSize(t.ResultOk.Value))
		}
		if t.ResultErr.Valid {
			size = satAddU32(size, a.valTypeSize(t.ResultErr.Value))
		}
		return size
	}
	return 1
}

func (a *TypeArena) computeFuncTypeSize(t *FuncTypeDesc) uint32 {
	size := uint32(1)
	for _, p := range t.Params {
		size = satAddU32(size, a.valTypeSize(p.Type))
	}
	for _, r := range t.Results {
		size = satAddU32(size, a.valTypeSize(r.Type))
	}
	return size
}

func (a *TypeArena) computeInstanceTypeSize(t *InstanceTypeDesc) uint32 {
	size := uint32(1)
	for _, et := range t.Exports {
		size = satAddU32(size, a.entityTypeSize(et))
	}
	return size
}

func (a *TypeArena) computeComponentTypeSize(t *ComponentTypeDesc) uint32 {
	size := uint32(1)
	for _, et := range t.Imports {
		size = satAddU32(size, a.entityTypeSize(et))
	}
	for _, et := range t.Exports {
		size = satAddU32(size, a.entityTypeSize(et))
	}
	return size
}

