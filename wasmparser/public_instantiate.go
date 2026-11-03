package wasmparser

import (
	"fmt"
	"slices"
)

// ComponentType is an opaque handle to a component's type signature —
// imports and exports, resolved against the arena that parsed it.
//
// Obtain one from (*ValidatingParser).TopLevelComponentType after parsing
// finishes. Zero value is not usable.
type ComponentType struct {
	arena *TypeArena
	id    ComponentTypeID
}

// InstanceType is an opaque handle to a fresh instance of a ComponentType.
// Two *InstanceType values represent the same instance (carrying the same
// resource identities under an (eq $r) subtype constraint) if and only if
// they are the same pointer.
//
// Obtain one from (*ComponentType).NewInstance. The handle holds only a
// pointer to its source ComponentType; fresh resource identities are
// induced by pointer identity during CheckInstantiation and are never
// materialized into any arena. No shared state mutation occurs when a
// handle is created or used.
//
// resourceOrigin records, for each ResourceID this instance exports
// that is an alias of another instance's resource (rather than locally
// defined), the originating *InstanceType and the corresponding
// ResourceID in that instance's source arena. The SubtypeChecker
// resolves resource identity through this map so that re-exports
// through a chain of components share both scope and ID with the
// defining instance — the structural mechanism behind WIT `use` and
// outer aliasing. The remapping is necessary because each parsed
// component's import declarations freshen the imported resource IDs
// in its own arena, so an alias-side ResID is rarely equal to the
// defining-side ResID.
type InstanceType struct {
	source         *ComponentType
	resourceOrigin map[ResourceID]ResourceOriginSpec
}

// ResourceOriginSpec records that a placeholder ResourceID exported by
// an *InstanceType corresponds to a resource defined elsewhere. Lender
// is the *InstanceType supplying the resource; LenderResID is that
// resource's ID in the lender's source arena.
type ResourceOriginSpec struct {
	Lender      *InstanceType
	LenderResID ResourceID
}

// FuncType is an opaque handle to a component-level function type signature
// in a validator arena. Obtain one from a ValidatedComponentExport /
// ValidatedComponentImport item after parsing; it is not constructible
// directly. Zero value is not usable.
type FuncType struct {
	arena *TypeArena
	id    ComponentFuncTypeID
}

// ModuleType is an opaque handle to a core-module type in a validator
// arena. Obtain one from a ValidatedModuleSectionPayload or a
// ValidatedComponentExport / ValidatedComponentImport item after parsing;
// it is not constructible directly. Zero value is not usable.
type ModuleType struct {
	arena *TypeArena
	id    CoreModuleTypeID
}

// NewInstance returns a fresh instance-type handle. Each call allocates
// a new pointer; no arena is touched. Under the checker's resource-
// identity rules, two separate NewInstance calls produce instances with
// distinct resources; supplying the same handle twice shares identity.
func (c *ComponentType) NewInstance() *InstanceType {
	return &InstanceType{source: c}
}

// SetResourceOrigin records, for each ResourceID exported by this
// InstanceType that is an alias of a resource defined by another
// instance, the lender *InstanceType and that resource's ID in the
// lender's arena. CheckInstantiation walks these bindings while
// testing resource identity so that an instance re-exporting an
// aliased resource shares scope and ID with the defining instance.
// Resources locally defined by this InstanceType (i.e., freshened per
// spec) must not appear as keys; only aliased exports belong here.
// Replaces any previous binding.
func (i *InstanceType) SetResourceOrigin(origin map[ResourceID]ResourceOriginSpec) {
	if len(origin) == 0 {
		i.resourceOrigin = nil
		return
	}
	i.resourceOrigin = origin
}

// ComputeResourceOrigins walks this ComponentType's resource exports, including
// those of nested instances, and, for each export that is structurally an alias
// of a resource defined by an instance import (rather than locally defined),
// records the originating import's supplied *InstanceType and the resource's ID
// in that lender's arena. Pass the result to (*InstanceType).SetResourceOrigin on
// the instance handle minted from this ComponentType.
//
// suppliedImports maps each instance-import name on this ComponentType
// to the *InstanceType supplied for that import. Names not present in
// the map (or whose supplied value is nil) cause the corresponding
// aliased exports to be omitted from the result, in which case the
// SubtypeChecker falls back to using this instance's own scope.
func (c *ComponentType) ComputeResourceOrigins(suppliedImports map[string]*InstanceType) map[ResourceID]ResourceOriginSpec {
	if c == nil {
		return nil
	}
	ct := c.arena.ComponentTypes[c.id]
	// Build a reverse index of the resources instance imports define. Resources
	// NOT in this index are locally defined.
	lenders := make(map[ResourceID]resourceLender)
	for impName, impEnt := range ct.Imports {
		if impEnt.Kind == EntityInstance {
			c.collectResourceLenders(impEnt.InstID, impName, nil, lenders)
		}
	}
	out := make(map[ResourceID]ResourceOriginSpec)
	c.collectResourceOrigins(ct.Exports, lenders, suppliedImports, out)
	if len(out) == 0 {
		return nil
	}
	return out
}

// resourceLender locates a resource defined by an instance import: exportPath is
// its path through that instance's exports.
type resourceLender struct {
	importName string
	exportPath []string
}

func (c *ComponentType) collectResourceLenders(id ComponentInstanceTypeID, importName string, path []string, lenders map[ResourceID]resourceLender) {
	it := c.arena.InstanceTypes[id]
	for _, res := range it.DefinedResources {
		lenders[res.ID] = resourceLender{importName: importName, exportPath: slices.Concat(path, []string{res.Export})}
	}
	for name, et := range it.Exports {
		if et.Kind == EntityInstance {
			c.collectResourceLenders(et.InstID, importName, slices.Concat(path, []string{name}), lenders)
		}
	}
}

func (c *ComponentType) collectResourceOrigins(exports map[string]ComponentEntityType, lenders map[ResourceID]resourceLender, suppliedImports map[string]*InstanceType, out map[ResourceID]ResourceOriginSpec) {
	for _, ent := range exports {
		if ent.Kind == EntityInstance {
			c.collectResourceOrigins(c.arena.InstanceTypes[ent.InstID].Exports, lenders, suppliedImports, out)
			continue
		}
		if ent.Kind != EntityType || ent.TypeRef.Kind != AnyTypeResource {
			continue
		}
		rid := ent.TypeRef.ResID
		lender, ok := lenders[rid]
		if !ok {
			// The export may have been minted as a fresh "created" identity with
			// the import-bound resource as its defining source.
			if lender, ok = lenders[c.arena.ResolveResourceAlias(rid)]; !ok {
				continue
			}
		}
		supplied := suppliedImports[lender.importName]
		if supplied == nil {
			continue
		}
		lenderResID, ok := supplied.resourceAt(lender.exportPath)
		if !ok {
			continue
		}
		out[rid] = ResourceOriginSpec{Lender: supplied, LenderResID: lenderResID}
	}
}

// resourceAt returns the ResourceID of the resource at path through this
// instance's exports.
func (i *InstanceType) resourceAt(path []string) (ResourceID, bool) {
	if i == nil || i.source == nil {
		return 0, false
	}
	arena := i.source.arena
	exports := arena.ComponentTypes[i.source.id].Exports
	for _, name := range path[:len(path)-1] {
		ent, ok := exports[name]
		if !ok || ent.Kind != EntityInstance {
			return 0, false
		}
		exports = arena.InstanceTypes[ent.InstID].Exports
	}
	ent, ok := exports[path[len(path)-1]]
	if !ok || ent.Kind != EntityType || ent.TypeRef.Kind != AnyTypeResource {
		return 0, false
	}
	return ent.TypeRef.ResID, true
}

// ExportedResourceID returns the ResourceID of the resource type this
// instance exports under name. Reports false if name is not a resource
// type export. Used by host code constructing per-instance ComponentTypes
// that need to align their own ResIDs with a lender instance's at
// Instantiate time.
func (i *InstanceType) ExportedResourceID(name string) (ResourceID, bool) {
	return i.resourceAt([]string{name})
}

// CheckInstantiation verifies that the supplied arguments satisfy the
// component's declared imports. Accepts any of: *InstanceType (for
// instance imports), *ComponentType (for component imports),
// *FuncType (for func imports), *ModuleType (for core-module imports).
// Missing imports are silently skipped — callers that require all
// imports be supplied must enforce that separately. Value and type
// imports are not checked here.
//
// args may be keyed by either the component's literal import name or
// its canonical interface name (see CanonicalizeImportName); if the
// literal name misses, the canonical form is tried.
//
// Returns nil if every supplied arg is a subtype of its corresponding
// import, or an error describing the first mismatch.
func (c *ComponentType) CheckInstantiation(args map[string]any) error {
	compType := c.arena.ComponentTypes[c.id]
	sc := &SubtypeChecker{
		bArena:          c.arena,
		resourceMapping: make(map[ResourceID]scopedResID),
		hostBinding:     true,
	}

	for _, importName := range compType.ImportOrder {
		arg, ok := args[importName]
		if !ok {
			arg, ok = args[CanonicalizeImportName(importName)]
		}
		if !ok || arg == nil {
			continue
		}
		importType := compType.Imports[importName]
		switch a := arg.(type) {
		case *InstanceType:
			if importType.Kind != EntityInstance {
				return fmt.Errorf("import %q: expected %s found instance", importName, entityExpectedName(importType.Kind))
			}
			sc.aArena = a.source.arena
			sc.aScope = a
			if err := sc.isInstanceSubtypeAgainstExports(a.source.arena.ComponentTypes[a.source.id].Exports, importType.InstID); err != nil {
				return fmt.Errorf("import %q: %w", importName, err)
			}
		case *FuncType:
			if importType.Kind != EntityFunc {
				return fmt.Errorf("import %q: expected %s found function", importName, entityExpectedName(importType.Kind))
			}
			sc.aArena = a.arena
			sc.aScope = nil
			if err := sc.isFuncTypeSubtype(a.id, importType.FuncID); err != nil {
				return fmt.Errorf("import %q: %w", importName, err)
			}
		case *ModuleType:
			if importType.Kind != EntityModule {
				return fmt.Errorf("import %q: expected %s found module", importName, entityExpectedName(importType.Kind))
			}
			sc.aArena = a.arena
			sc.aScope = nil
			if err := sc.isModuleSubtype(a.id, importType.ModuleID); err != nil {
				return fmt.Errorf("import %q: %w", importName, err)
			}
		case *ComponentType:
			if importType.Kind != EntityComponent {
				return fmt.Errorf("import %q: expected %s found component", importName, entityExpectedName(importType.Kind))
			}
			sc.aArena = a.arena
			sc.aScope = nil
			if err := sc.isComponentSubtype(a.id, importType.CompID); err != nil {
				return fmt.Errorf("import %q: %w", importName, err)
			}
		default:
			return fmt.Errorf("import %q: unsupported argument type %T", importName, arg)
		}
	}
	return nil
}

// isInstanceSubtypeAgainstExports checks that the exports of a supplied instance
// are a subtype of the instance type's exports. Used by CheckInstantiation to
// avoid materializing a synthetic instance type.
//
// Unlike isInstanceSubtype, which links components to each other, this links a
// supplied instance and follows wasmtime in letting it omit whatever needs no
// runtime definition. The same rules apply at every instance depth:
//   - Non-resource type exports may be omitted, as the consumer's declaration
//     fully determines them.
//   - A nested instance needing nothing at runtime may be omitted.
//   - Each freshly-introduced resource (listed in DefinedResources) must
//     appear under the export that defines it, so the subtype checker can
//     tie the consumer's abstract ResourceID to the supplied concrete one.
//     Further exports referencing the same resource via `(eq $r)` are
//     aliases and may be omitted; its identity is already pinned.
func (sc *SubtypeChecker) isInstanceSubtypeAgainstExports(aExports map[string]ComponentEntityType, bInstID ComponentInstanceTypeID) error {
	bt := sc.bArena.InstanceTypes[bInstID]

	for name, bExport := range bt.Exports {
		aExport, ok := aExports[name]
		if !ok {
			if bExport.Kind == EntityType || (bExport.Kind == EntityInstance && isInstanceTypeRuntimeEmpty(sc.bArena, bExport.InstID)) {
				continue
			}
			return fmt.Errorf("export `%s` was not found", name)
		}
		var err error
		if aExport.Kind == EntityInstance && bExport.Kind == EntityInstance {
			err = sc.isInstanceSubtypeAgainstExports(sc.aArena.InstanceTypes[aExport.InstID].Exports, bExport.InstID)
		} else {
			err = sc.isSubtype(aExport, bExport)
		}
		if err != nil {
			return fmt.Errorf("type mismatch in instance export `%s`: %w", name, err)
		}
	}

	for _, res := range bt.DefinedResources {
		if _, ok := aExports[res.Export]; !ok {
			return fmt.Errorf("export `%s` was not found", res.Export)
		}
	}
	return nil
}
