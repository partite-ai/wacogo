package core

import (
	"context"

	"github.com/partite-ai/wacogo/internal/canon"
)

// canonicalOptions holds resolved canonical options for a lift/lower.
type canonicalOptions struct {
	stringEncoding canon.StringEncoding
	memory         uint32
	realloc        uint32
	postReturn     uint32
	hasMemory      bool
	hasRealloc     bool
	hasPostReturn  bool
}

// planStep is the union of instantiation plan steps.
type planStep interface {
	execute(ctx context.Context, s *instantiationState) error
}

// instanceKind distinguishes instance types in import bindings.
type instanceKind uint8

const (
	instanceKindCore instanceKind = iota
	instanceKindAdapter
	instanceKindRuntime
)

// importBinding identifies where an import is satisfied from.
type importBinding struct {
	instanceKind instanceKind
	instanceIdx  uint32
	exportName   string
}

// funcRef references a component-level function.
type funcRef struct{ funcIdx uint32 }

// Sort identifies definition kinds.
type Sort uint8

const (
	SortCoreFunc Sort = iota
	SortCoreTable
	SortCoreMemory
	SortCoreGlobal
	SortCoreModule
	SortCoreInstance
	SortFunc
	SortValue
	SortType
	SortComponent
	SortInstance
)

// aliasSource is the union of alias source types.
type aliasSource interface{ isAliasSource() }

// aliasExport is an alias sourced from a component instance export.
type aliasExport struct {
	instanceIdx uint32
	name        string
}

func (aliasExport) isAliasSource() {}

// aliasCoreExport is an alias sourced from a core module instance export.
type aliasCoreExport struct {
	instanceIdx uint32
	name        string
}

func (aliasCoreExport) isAliasSource() {}

// aliasOuter is an alias sourced from an outer component scope.
type aliasOuter struct {
	count uint32
	index uint32
}

func (aliasOuter) isAliasSource() {}

// planInstantiateModule instantiates a core module with the given imports.
type planInstantiateModule struct {
	moduleIndex uint32
	imports     []importBinding
}

// planAlias adds an alias of a definition from another scope.
type planAlias struct {
	source     aliasSource
	targetSort Sort
	// targetIndex, when >= 0, indicates the index in the target index space
	// (compiledModules or subComponents) where the alias result should be
	// stored. This is used when a placeholder was reserved at load time.
	// When < 0, the result is appended to the end of the target index space.
	targetIndex int
}

// planLift lifts a core function into a component function.
// funcTypeID is the component type index space slot holding the lifted
// function's *FuncType; resolved at execute time via inst.types.
type planLift struct {
	coreFuncIndex uint32
	funcTypeID    uint32
	options       canonicalOptions
}

// planLower lowers a component function into a core function. The *Func at
// s.funcs[funcRef.funcIdx] was built by the planLift or planImportFunc step
// that originally created it; this step only copies its Callee into an
// adapter core func.
type planLower struct {
	funcRef funcRef
	options canonicalOptions
}

// planResourceNew creates a new resource handle.
// typeID points at the resource type in the component type index space; at
// execute time inst.types[typeID] yields the *TypeResource used as the
// resourceTable entry's type discriminator.
type planResourceNew struct {
	typeID uint32
}

// planResourceDrop drops a resource handle.
type planResourceDrop struct {
	// typeID is the resource type's TypeID — resolved at execute time to
	// pass as the table's type discriminator.
	typeID uint32
	// dtorFuncIndex, when >= 0, is the core func index of the destructor
	// to call with the rep value when the handle is dropped.
	dtorFuncIndex int
}

// planResourceRep returns the representation of a resource handle.
type planResourceRep struct {
	typeID uint32
}

// planImportFunc records a component function import.
// At instantiate time this is resolved from the WithImport map.
// funcTypeID is the component type index space slot holding the import's
// expected *FuncType; resolved at execute time via inst.types.
type planImportFunc struct {
	name       string
	funcTypeID uint32
}

// planImportInstance records a component instance import.
// At instantiate time this is resolved from the WithImport map. When
// runtimeEmpty is true the import's declared instance type carries no
// runtime data (all type exports, no fresh resources) and an empty
// placeholder is substituted if the host omits the arg.
type planImportInstance struct {
	name         string
	runtimeEmpty bool
}

// coreExportBinding binds a name to a core definition (func, memory, etc.) by sort and index.
type coreExportBinding struct {
	name  string
	sort  Sort
	index uint32
}

// planInstantiateFromExports creates a core instance from inline exports.
type planInstantiateFromExports struct {
	exports []coreExportBinding
}

// planInstantiateComponent instantiates a nested sub-component with the given args.
type planInstantiateComponent struct {
	componentIndex uint32
	args           []instantiateArg
}

// instantiateArg maps a named import to a definition in the parent's index space.
type instantiateArg struct {
	name  string
	kind  Sort
	index uint32
}

// planComponentInstantiateFromExports creates a component instance from inline exports.
type planComponentInstantiateFromExports struct {
	exports []componentExportDesc
}

// componentExportDesc describes an export for component-level instantiate-from-exports.
type componentExportDesc struct {
	name  string
	kind  Sort
	index uint32
}

// planImportModule records a core module import.
// At instantiate time this is resolved from the parent's compiled modules.
type planImportModule struct {
	name  string
	index uint32 // index in the compiledModules slice to fill
}

// planImportComponent records a component import.
// At instantiate time this is resolved from the parent's sub-components.
type planImportComponent struct {
	name  string
	index uint32 // index in the subComponents slice to fill
}

// planStart calls the component start function.
type planStart struct {
	funcIndex   uint32
	args        []uint32
	resultCount uint32
}

// planResolveType runs the resolver at component.typeResolvers[typeID]
// against the current instance and stores the resulting Type in
// instance.types[typeID]. Plan steps for type-allocating definitions
// (type decls, alias-instance-export of SortType, alias-outer of SortType,
// direct type imports) emit one planResolveType in declaration order so
// the instance's type table is built incrementally alongside the instance
// and core index spaces.
type planResolveType struct {
	typeID uint32
}

// planExportRebind mirrors a component-level export's source entry into
// the same-sort index space. The component model treats each export as
// a re-binding: a component function/instance/module/component export
// appears again at a fresh index in the enclosing scope's index space,
// so subsequent items (inline instance exports, alias targets, further
// canonical function refs) can reference the rebound index.
//
// targetIndex is the pre-allocated slot index for sorts whose load-time
// scope was grown by processExportSection (instance, core module,
// component). For SortFunc the load-time scope is not pre-grown and the
// runtime appends; in that case targetIndex is -1.
type planExportRebind struct {
	sort        Sort
	sourceIndex uint32
	targetIndex int
}
