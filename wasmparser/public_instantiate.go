package wasmparser

import "fmt"


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

// ComputeResourceOrigins walks this ComponentType's resource exports
// and, for each export that is structurally an alias of a resource
// defined by an instance import (rather than locally defined), records
// the originating import's supplied *InstanceType and the resource's
// ID in that lender's arena. Pass the result to
// (*InstanceType).SetResourceOrigin on the instance handle minted from
// this ComponentType.
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
	// Build a reverse index: ResID → (importName, exportName) if the
	// resource is defined by some instance import. Resources NOT in
	// this index are locally defined.
	type lenderRef struct {
		importName string
		exportName string
	}
	lenderByRID := make(map[ResourceID]lenderRef)
	for impName, impEnt := range ct.Imports {
		if impEnt.Kind != EntityInstance {
			continue
		}
		impInst := c.arena.InstanceTypes[impEnt.InstID]
		definedSet := make(map[ResourceID]bool, len(impInst.DefinedResources))
		for _, rid := range impInst.DefinedResources {
			definedSet[rid] = true
		}
		for name, ent := range impInst.Exports {
			if ent.Kind != EntityType || ent.TypeRef.Kind != AnyTypeResource {
				continue
			}
			rid := ent.TypeRef.ResID
			if !definedSet[rid] {
				continue
			}
			if _, exists := lenderByRID[rid]; !exists {
				lenderByRID[rid] = lenderRef{importName: impName, exportName: name}
			}
		}
	}
	out := make(map[ResourceID]ResourceOriginSpec)
	for _, ent := range ct.Exports {
		if ent.Kind != EntityType || ent.TypeRef.Kind != AnyTypeResource {
			continue
		}
		rid := ent.TypeRef.ResID
		ref, ok := lenderByRID[rid]
		if !ok {
			// Try once through a resource alias — the export may have
			// been minted as a fresh "created" identity with the
			// import-bound resource as its defining source.
			defining := c.arena.resolveResourceAlias(rid)
			if defining == rid {
				continue // locally defined
			}
			ref, ok = lenderByRID[defining]
			if !ok {
				continue
			}
		}
		lender := suppliedImports[ref.importName]
		if lender == nil {
			continue
		}
		lenderResID, ok := lender.ExportedResourceID(ref.exportName)
		if !ok {
			continue
		}
		out[rid] = ResourceOriginSpec{Lender: lender, LenderResID: lenderResID}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ExportedResourceID returns the ResourceID of the resource type this
// instance exports under name. Reports false if name is not a resource
// type export. Used by host code constructing per-instance ComponentTypes
// that need to align their own ResIDs with a lender instance's at
// Instantiate time.
func (i *InstanceType) ExportedResourceID(name string) (ResourceID, bool) {
	if i == nil || i.source == nil {
		return 0, false
	}
	ct := i.source.arena.ComponentTypes[i.source.id]
	ent, ok := ct.Exports[name]
	if !ok || ent.Kind != EntityType || ent.TypeRef.Kind != AnyTypeResource {
		return 0, false
	}
	return ent.TypeRef.ResID, true
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
			if err := sc.isInstanceSubtypeAgainstExports(a.source.id, importType.InstID); err != nil {
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

// isInstanceSubtypeAgainstExports checks that the component's exports
// (treated as an instance's exports) are a subtype of the instance type's
// exports. Used by CheckInstantiation to avoid materializing a synthetic
// instance type.
//
// Type exports whose identity is statically determined without host
// cooperation may be absent on the host. Concretely:
//   - Non-resource type exports (structural types) never require a host
//     provider — their identity is fully determined by the consumer.
//   - For each freshly-introduced resource (listed in DefinedResources),
//     at least one export referencing it must appear on both sides so
//     the subtype checker can tie the consumer's abstract ResourceID to
//     the host's concrete one. Further exports referencing the same
//     resource via `(eq $r)` are aliases and may be omitted by the
//     host; its identity is already pinned.
func (sc *SubtypeChecker) isInstanceSubtypeAgainstExports(aCompID ComponentTypeID, bInstID ComponentInstanceTypeID) error {
	at := sc.aArena.ComponentTypes[aCompID]
	bt := sc.bArena.InstanceTypes[bInstID]

	providedResources := make(map[ResourceID]bool)
	for name, bExport := range bt.Exports {
		aExport, ok := at.Exports[name]
		if !ok {
			if bExport.Kind == EntityType {
				continue
			}
			return fmt.Errorf("export `%s` was not found", name)
		}
		if err := sc.isSubtype(aExport, bExport); err != nil {
			return fmt.Errorf("type mismatch in instance export `%s`: %w", name, err)
		}
		if bExport.Kind == EntityType && bExport.TypeRef.Kind == AnyTypeResource {
			providedResources[bExport.TypeRef.ResID] = true
		}
	}

	for _, rid := range bt.DefinedResources {
		if providedResources[rid] {
			continue
		}
		for name, bExport := range bt.Exports {
			if bExport.Kind == EntityType && bExport.TypeRef.Kind == AnyTypeResource && bExport.TypeRef.ResID == rid {
				return fmt.Errorf("export `%s` was not found", name)
			}
		}
	}
	return nil
}
