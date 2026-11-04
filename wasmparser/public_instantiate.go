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

// InstanceType is an opaque handle to the actual type of a live instance: what
// it exports, and the scope that gives its resources their identity.
//
// Instantiating a component opens a fresh scope, so its resources differ from
// those of every other instantiation. An instance assembled by that
// instantiation, such as an inline instance or a nested host scope, shares its
// scope, since it names the same resources.
//
// Obtain one from (*ComponentType).NewInstance, (*InstanceType).SubInstance or
// (*InstanceType).ExportedInstance. Fresh resource identities are induced by
// pointer identity during CheckInstantiation and are never materialized into
// any arena.
type InstanceType struct {
	arena   *TypeArena
	exports map[string]ComponentEntityType

	// scope is the InstanceType of the instantiation that opened this type's
	// scope, which is this type itself if it did.
	scope *InstanceType

	// resourceRoots is held by a scope's own InstanceType. It records, for each
	// defining ResourceID that scope knows but does not define, the identity of
	// that resource: an import is whatever was supplied, and a resource created
	// by a sub-instance is that sub-instance's. This is what gives every ID for a
	// resource one identity.
	resourceRoots map[ResourceID]scopedResID

	// instances and funcs are the instance and func index spaces of the
	// instantiation this type describes, if it describes one.
	instances []*InstanceType
	funcs     []ComponentFuncTypeID

	// children and components hold the actual instances and components exported
	// under each name. An ascription can hide them from exports.
	children   map[string]*InstanceType
	components map[string]*componentValue
}

// ResourceFrom names a resource exported by another instance, to bind to a
// placeholder.
type ResourceFrom struct {
	Lender *InstanceType
	Export string
}

// FuncType is an opaque handle to the type of a func in a live instance, which
// knows whose resources its signature names. Obtain one from
// (*InstanceType).FuncType; it is not constructible directly. Zero value is not
// usable.
type FuncType struct {
	arena *TypeArena
	id    ComponentFuncTypeID
	scope *InstanceType
}

// ModuleType is an opaque handle to a core-module type in a validator
// arena. Obtain one from a ValidatedModuleSectionPayload or a
// ValidatedComponentExport / ValidatedComponentImport item after parsing;
// it is not constructible directly. Zero value is not usable.
type ModuleType struct {
	arena *TypeArena
	id    CoreModuleTypeID
}

// InstanceBindings answers what an instantiation binds its instance imports and
// open resources to.
type InstanceBindings interface {
	// Instance returns what was supplied for the named instance import, or nil.
	Instance(name string) *InstanceType
	// Resource returns what stands for a resource the type leaves open.
	Resource(res ImportedResource) (ResourceFrom, bool)
}

// NewInstance returns the type of a fresh instance of this component, including
// those of the instances inside it. A nil bindings supplies nothing.
//
// Each call opens a new scope: two separate NewInstance calls produce instances
// with distinct resources; supplying the same handle twice shares identity.
// Open resources with nothing bound belong to the new instance's scope.
func (c *ComponentType) NewInstance(bindings InstanceBindings) *InstanceType {
	var b evalBindings = noBindings{}
	if bindings != nil {
		b = suppliedBindings{bindings}
	}
	return evalInstance(c.arena, c.id, nil, b)
}

// SubInstance returns the type of the instance at index in the instance index
// space of the component this type is an instantiation of, or nil if there is
// none. It is an instance's actual type, not what any export declares about it.
func (i *InstanceType) SubInstance(index int) *InstanceType {
	if index < 0 || index >= len(i.instances) {
		return nil
	}
	return i.instances[index]
}

// FuncType returns the type of the func at index in the func index space of the
// component this type is an instantiation of, or nil if there is none. The
// resources it names are this instance's.
func (i *InstanceType) FuncType(index int) *FuncType {
	if index < 0 || index >= len(i.funcs) {
		return nil
	}
	return &FuncType{arena: i.arena, id: i.funcs[index], scope: i.scope}
}

// ExportedInstance returns the type of the instance this type exports under
// name, or nil if there is none.
func (i *InstanceType) ExportedInstance(name string) *InstanceType {
	if child, ok := i.children[name]; ok {
		return child
	}
	ent, ok := i.exports[name]
	if !ok || ent.Kind != EntityInstance {
		return nil
	}
	return &InstanceType{arena: i.arena, exports: i.arena.InstanceTypes[ent.InstID].Exports, scope: i.scope}
}

func (i *InstanceType) setChild(name string, child *InstanceType) {
	if i.children == nil {
		i.children = make(map[string]*InstanceType)
	}
	i.children[name] = child
}

func (i *InstanceType) setComponent(name string, c *componentValue) {
	if i.components == nil {
		i.components = make(map[string]*componentValue)
	}
	i.components[name] = c
}

// ImportsComponent reports whether this component imports a component, either
// directly or from an imported instance at any depth.
func (c *ComponentType) ImportsComponent() bool {
	return exportsComponent(c.arena, c.arena.ComponentTypes[c.id].Imports)
}

func exportsComponent(arena *TypeArena, entities map[string]ComponentEntityType) bool {
	for _, ent := range entities {
		switch ent.Kind {
		case EntityComponent:
			return true
		case EntityInstance:
			if exportsComponent(arena, arena.InstanceTypes[ent.InstID].Exports) {
				return true
			}
		}
	}
	return false
}

// resourceAt returns the ResourceID of the resource at path through this
// instance's exports.
func (i *InstanceType) resourceAt(path []string) (ResourceID, bool) {
	if i == nil {
		return 0, false
	}
	exports := i.exports
	for _, name := range path[:len(path)-1] {
		ent, ok := exports[name]
		if !ok || ent.Kind != EntityInstance {
			return 0, false
		}
		exports = i.arena.InstanceTypes[ent.InstID].Exports
	}
	ent, ok := exports[path[len(path)-1]]
	if !ok || ent.Kind != EntityType || ent.TypeRef.Kind != AnyTypeResource {
		return 0, false
	}
	return ent.TypeRef.ResID, true
}

// CheckInstantiation verifies that the supplied arguments satisfy the
// component's declared imports. Accepts any of: *InstanceType (for
// instance imports), *ComponentType (for component imports),
// *FuncType (for func imports), *ModuleType (for core-module imports),
// ResourceFrom (for resource type imports). Missing imports are silently
// skipped — callers that require all imports be supplied must enforce
// that separately. Value imports are not checked here.
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
			sc.aArena = a.arena
			sc.aScope = a.scope
			if err := sc.isInstanceSubtypeAgainstExports(a.exports, importType.InstID); err != nil {
				return fmt.Errorf("import %q: %w", importName, err)
			}
		case *FuncType:
			if importType.Kind != EntityFunc {
				return fmt.Errorf("import %q: expected %s found function", importName, entityExpectedName(importType.Kind))
			}
			sc.aArena = a.arena
			sc.aScope = a.scope
			if err := sc.isFuncTypeSubtype(a.id, importType.FuncID); err != nil {
				return fmt.Errorf("import %q: %w", importName, err)
			}
		case ResourceFrom:
			if importType.Kind != EntityType {
				return fmt.Errorf("import %q: expected %s found resource", importName, entityExpectedName(importType.Kind))
			}
			if importType.TypeRef.Kind != AnyTypeResource {
				return fmt.Errorf("import %q: expected %s, found resource", importName, importType.TypeRef.Desc())
			}
			lenderRes, ok := a.Lender.resourceAt([]string{a.Export})
			if !ok {
				return fmt.Errorf("import %q: instance has no resource export `%s`", importName, a.Export)
			}
			sc.aArena = a.Lender.arena
			sc.aScope = a.Lender.scope
			if err := sc.checkResourceMatch(lenderRes, importType.TypeRef.ResID); err != nil {
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
