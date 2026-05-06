package wasmparser

import "slices"

import "fmt"

// walkValType recursively visits all DefinedTypeDesc nodes within
// an ValTypeDesc. For primitive types, there is nothing to visit. For defined
// types, it delegates to walkDefinedType. The visitor is called for each defined
// type node; if it returns true, the walk stops immediately (short-circuit).
func (v *Validator) walkValType(ivt ValTypeDesc, visit func(*DefinedTypeDesc) bool) bool {
	if ivt.IsPrimitive {
		return false
	}
	dt := v.arena.DefinedTypes[ivt.TypeID]
	return v.walkDefinedType(&dt, visit)
}

// walkDefinedType recursively visits a defined type and all of its children.
// The visitor is called for each DefinedTypeDesc node encountered.
// If the visitor returns true, the walk stops (short-circuit). After visiting
// the current node, the walker recurses into composite type children (Record
// fields, Variant cases, List element, Tuple elements, Option payload, and
// Result ok/err payloads). Leaf types (Own, Borrow, Flags, Enum) have no
// children so the walk naturally terminates.
func (v *Validator) walkDefinedType(dt *DefinedTypeDesc, visit func(*DefinedTypeDesc) bool) bool {
	if visit(dt) {
		return true
	}
	switch dt.Kind {
	case DefinedKindRecord:
		if dt.Record != nil {
			for _, f := range dt.Record.Fields {
				if v.walkValType(f.Type, visit) {
					return true
				}
			}
		}
	case DefinedKindVariant:
		if dt.Variant != nil {
			for _, c := range dt.Variant.Cases {
				if c.Type.Valid {
					if v.walkValType(c.Type.Value, visit) {
						return true
					}
				}
			}
		}
	case DefinedKindList:
		return v.walkValType(dt.List, visit)
	case DefinedKindTuple:
		for _, t := range dt.Tuple {
			if v.walkValType(t, visit) {
				return true
			}
		}
	case DefinedKindOption:
		return v.walkValType(dt.Option, visit)
	case DefinedKindResult:
		if dt.ResultOk.Valid {
			if v.walkValType(dt.ResultOk.Value, visit) {
				return true
			}
		}
		if dt.ResultErr.Valid {
			if v.walkValType(dt.ResultErr.Value, visit) {
				return true
			}
		}
	}
	return false
}

func (v *Validator) isOwnType(ivt ValTypeDesc) bool {
	if ivt.IsPrimitive {
		return false
	}
	dt := v.arena.DefinedTypes[ivt.TypeID]
	return dt.Kind == DefinedKindOwn
}

func (v *Validator) isBorrowType(ivt ValTypeDesc) bool {
	if ivt.IsPrimitive {
		return false
	}
	dt := v.arena.DefinedTypes[ivt.TypeID]
	return dt.Kind == DefinedKindBorrow
}

func (v *Validator) isResultOfOwn(ivt ValTypeDesc) bool {
	if ivt.IsPrimitive {
		return false
	}
	dt := v.arena.DefinedTypes[ivt.TypeID]
	if dt.Kind != DefinedKindResult {
		return false
	}
	if !dt.ResultOk.Valid {
		return false
	}
	return v.isOwnType(dt.ResultOk.Value)
}

// contextResourceNames returns the appropriate resource name map for the given kind
// (import or export). For imports, both imported and exported names are visible.
// For exports, only exported names are visible.
func contextResourceNames(cs *ComponentState, kind string) map[ResourceID][]string {
	if kind == "import" {
		return cs.resourceNames // imports can see all named resources
	}
	return cs.exportResourceNamesMap // exports can only see export-named resources
}

// addResourceName appends a name to the slice of names for a resource ID if
// it isn't already present. A resource can accumulate multiple names when it
// is exported or imported under several aliases; bracket-prefixed func names
// match against any of them.
//
// When rid is an export-alias half of a fresh→defining pair, the name is
// also recorded under the defining ResourceID so a subsequent lookup that
// resolves an alias to its defining still finds the alias's name.
func addResourceName(arena *TypeArena, m map[ResourceID][]string, rid ResourceID, name string) {
	addOne := func(id ResourceID) {
		if slices.Contains(m[id], name) {
			return
		}
		m[id] = append(m[id], name)
	}
	addOne(rid)
	if arena != nil {
		if defining := arena.resolveResourceAlias(rid); defining != rid {
			addOne(defining)
		}
	}
}

// resourceHasName reports whether any recorded name for rid equals name.
func resourceHasName(names []string, name string) bool {
	return slices.Contains(names, name)
}

// checkFuncResourceName checks that a constructor function's return type resource
// matches the expected resource name.
func (v *Validator) checkFuncResourceName(cs *ComponentState, resourceName string, funcID ComponentFuncTypeID, kind string) error {
	ft := v.arena.FuncTypes[funcID]
	if len(ft.Results) == 0 {
		return nil
	}
	result := ft.Results[0].Type
	if result.IsPrimitive {
		return nil
	}
	dt := v.arena.DefinedTypes[result.TypeID]
	var rid ResourceID
	if dt.Kind == DefinedKindOwn {
		rid = dt.Own
	} else if dt.Kind == DefinedKindResult && dt.ResultOk.Valid && v.isOwnType(dt.ResultOk.Value) {
		innerDT := v.arena.DefinedTypes[dt.ResultOk.Value.TypeID]
		rid = innerDT.Own
	} else {
		return nil
	}
	// Check that the resource has a name in the appropriate context
	nameMap := contextResourceNames(cs, kind)
	names, ok := v.namesForResource(nameMap, rid)
	if !ok {
		return fmt.Errorf("resource used in function does not have a name in this context")
	}
	if !resourceHasName(names, resourceName) {
		return fmt.Errorf("function does not match expected resource name `%s`", names[0])
	}
	return nil
}

// checkMethodResourceName checks that a method's borrow parameter references
// the expected resource name.
func (v *Validator) checkMethodResourceName(cs *ComponentState, resourceName string, borrowType ValTypeDesc, kind string) error {
	if borrowType.IsPrimitive {
		return nil
	}
	dt := v.arena.DefinedTypes[borrowType.TypeID]
	if dt.Kind != DefinedKindBorrow {
		return nil
	}
	rid := dt.Borrow
	nameMap := contextResourceNames(cs, kind)
	names, ok := v.namesForResource(nameMap, rid)
	if !ok {
		return fmt.Errorf("resource used in function does not have a name in this context")
	}
	if !resourceHasName(names, resourceName) {
		return fmt.Errorf("function does not match expected resource name `%s`", names[0])
	}
	return nil
}

// namesForResource returns every name recorded for rid, including names
// recorded under other resource IDs that alias to the same defining resource.
// This makes bracket-name validation see all aliases as referring to the
// same underlying resource type, regardless of which specific alias was
// used in the func signature.
func (v *Validator) namesForResource(m map[ResourceID][]string, rid ResourceID) ([]string, bool) {
	defining := v.arena.resolveResourceAlias(rid)
	var out []string
	if names, ok := m[rid]; ok {
		out = append(out, names...)
	}
	if defining != rid {
		if names, ok := m[defining]; ok {
			for _, n := range names {
				if !slices.Contains(out, n) {
					out = append(out, n)
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func (v *Validator) isKnownResourceName(cs *ComponentState, name string) bool {
	for _, names := range cs.resourceNames {
		if resourceHasName(names, name) {
			return true
		}
	}
	return false
}

// checkFuncResourcesHaveNames validates that all resources referenced in a function type
// have names in the current context. This is required for non-bracket import/export names.
func (v *Validator) checkFuncResourcesHaveNames(cs *ComponentState, funcID ComponentFuncTypeID) error {
	ft := v.arena.FuncTypes[funcID]
	for _, p := range ft.Params {
		if err := v.checkValTypeResourcesHaveNames(cs, p.Type); err != nil {
			return err
		}
	}
	for _, r := range ft.Results {
		if err := v.checkValTypeResourcesHaveNames(cs, r.Type); err != nil {
			return err
		}
	}
	return nil
}

func (v *Validator) checkValTypeResourcesHaveNames(cs *ComponentState, ivt ValTypeDesc) error {
	var checkErr error
	v.walkValType(ivt, func(dt *DefinedTypeDesc) bool {
		switch dt.Kind {
		case DefinedKindOwn:
			if _, ok := cs.resourceNames[dt.Own]; !ok {
				checkErr = fmt.Errorf("resource used in function does not have a name in this context")
				return true
			}
		case DefinedKindBorrow:
			if _, ok := cs.resourceNames[dt.Borrow]; !ok {
				checkErr = fmt.Errorf("resource used in function does not have a name in this context")
				return true
			}
		}
		return false
	})
	return checkErr
}

// validateAndRegisterNamedTypes validates that an entity type is valid for use as an
// import or export, and registers any type IDs the entity contributes to the named-type
// sets. For functions and values, every value type must be "named" — meaning either a
// primitive, a structural type whose components are named, a record/variant/flags/enum
// already in the appropriate set, or an own/borrow whose resource is in the resource set.
// For type entities, the referenced type's contents are validated, then the entity's
// type ID is registered. Skipped inside instance-type contexts.
func (v *Validator) validateAndRegisterNamedTypes(cs *ComponentState, et ComponentEntityType, kind string) error {
	// Skip validation for instance type contexts
	if cs.kind == ComponentKindInstanceType {
		return nil
	}
	return v.validateEntityForExtern(cs, et, kind)
}

// validateEntityForExtern walks an entity for import/export validity. The entity-level
// error message ("X not valid to be used as Y") is emitted at the top of the failing
// entity; nested instance recursion re-wraps so the outermost frame describes the
// entity that the caller actually attempted to import/export.
func (v *Validator) validateEntityForExtern(cs *ComponentState, et ComponentEntityType, kind string) error {
	resourceSet, definedSet := v.namedSetsFor(cs, kind)

	switch et.Kind {
	case EntityType:
		if !v.allValTypesNamed(et.TypeRef, definedSet, resourceSet) {
			return fmt.Errorf("type not valid to be used as %s", kind)
		}
		v.registerTypeEntity(cs, et.TypeRef, kind)
		return nil
	case EntityFunc:
		if !v.allValTypesNamedInFunc(et.FuncID, definedSet, resourceSet) {
			return fmt.Errorf("func not valid to be used as %s", kind)
		}
		return nil
	case EntityValue:
		if !v.typeNamedValType(et.ValType, definedSet, resourceSet) {
			return fmt.Errorf("value not valid to be used as %s", kind)
		}
		return nil
	case EntityInstance:
		instType := v.arena.InstanceTypes[et.InstID]
		// Pre-register every type-entity export so the instance's type exports
		// can reference each other independently of map iteration order, and so
		// later func/value exports of the instance see those types as named.
		for _, expET := range instType.Exports {
			if expET.Kind == EntityType {
				v.registerTypeEntity(cs, expET.TypeRef, kind)
			}
		}
		// Validate types and nested instances.
		for _, expET := range instType.Exports {
			if expET.Kind == EntityType || expET.Kind == EntityInstance {
				if err := v.validateEntityForExtern(cs, expET, kind); err != nil {
					return fmt.Errorf("instance not valid to be used as %s", kind)
				}
			}
		}
		// Validate funcs/values now that types are registered.
		for _, expET := range instType.Exports {
			if expET.Kind != EntityType && expET.Kind != EntityInstance {
				if err := v.validateEntityForExtern(cs, expET, kind); err != nil {
					return fmt.Errorf("instance not valid to be used as %s", kind)
				}
			}
		}
		return nil
	case EntityModule, EntityComponent:
		return nil
	}
	return nil
}

func (v *Validator) namedSetsFor(cs *ComponentState, kind string) (map[ResourceID]bool, map[ComponentDefinedTypeID]bool) {
	if kind == "import" {
		return cs.importedResourceIDs, cs.importedDefinedTypes
	}
	return cs.exportedResourceIDs, cs.exportedDefinedTypes
}

// registerTypeEntity inserts the entity's type ID into the appropriate named-type set
// after the entity has been validated. Imports populate both the imported and exported
// sets so an imported type may then be re-exported.
func (v *Validator) registerTypeEntity(cs *ComponentState, ref ComponentAnyTypeID, kind string) {
	switch ref.Kind {
	case AnyTypeResource:
		rid := ref.ResID
		if kind == "import" {
			cs.importedResourceIDs[rid] = true
			cs.exportedResourceIDs[rid] = true
		} else {
			cs.exportedResourceIDs[rid] = true
		}
	case AnyTypeDefined:
		id := ComponentDefinedTypeID(ref.Index)
		if kind == "import" {
			cs.importedDefinedTypes[id] = true
			cs.exportedDefinedTypes[id] = true
		} else {
			cs.exportedDefinedTypes[id] = true
		}
	}
}

// allValTypesNamed checks that the type referenced by anyTypeID is one whose components
// are all named for the given sets. Resources and components are leaf-OK; defined types,
// funcs, and instances recurse one layer into their structural content.
func (v *Validator) allValTypesNamed(anyTypeID ComponentAnyTypeID, definedSet map[ComponentDefinedTypeID]bool, resourceSet map[ResourceID]bool) bool {
	switch anyTypeID.Kind {
	case AnyTypeResource, AnyTypeComponent:
		return true
	case AnyTypeDefined:
		return v.allValTypesNamedInDefined(ComponentDefinedTypeID(anyTypeID.Index), definedSet, resourceSet)
	case AnyTypeFunc:
		return v.allValTypesNamedInFunc(ComponentFuncTypeID(anyTypeID.Index), definedSet, resourceSet)
	case AnyTypeInstance:
		return v.allValTypesNamedInInstance(ComponentInstanceTypeID(anyTypeID.Index), definedSet, resourceSet)
	}
	return true
}

// allValTypesNamedInDefined inspects the immediate components of a defined type without
// requiring the type itself to be in the set. Used when validating the body of a
// type-entity export/import — the type may be anonymous, but its contents must be named.
func (v *Validator) allValTypesNamedInDefined(id ComponentDefinedTypeID, definedSet map[ComponentDefinedTypeID]bool, resourceSet map[ResourceID]bool) bool {
	dt := v.arena.DefinedTypes[id]
	switch dt.Kind {
	case DefinedKindPrimitive, DefinedKindFlags, DefinedKindEnum:
		return true
	case DefinedKindRecord:
		if dt.Record != nil {
			for _, f := range dt.Record.Fields {
				if !v.typeNamedValType(f.Type, definedSet, resourceSet) {
					return false
				}
			}
		}
		return true
	case DefinedKindVariant:
		if dt.Variant != nil {
			for _, c := range dt.Variant.Cases {
				if c.Type.Valid && !v.typeNamedValType(c.Type.Value, definedSet, resourceSet) {
					return false
				}
			}
		}
		return true
	case DefinedKindList:
		return v.typeNamedValType(dt.List, definedSet, resourceSet)
	case DefinedKindTuple:
		for _, t := range dt.Tuple {
			if !v.typeNamedValType(t, definedSet, resourceSet) {
				return false
			}
		}
		return true
	case DefinedKindOption:
		return v.typeNamedValType(dt.Option, definedSet, resourceSet)
	case DefinedKindResult:
		if dt.ResultOk.Valid && !v.typeNamedValType(dt.ResultOk.Value, definedSet, resourceSet) {
			return false
		}
		if dt.ResultErr.Valid && !v.typeNamedValType(dt.ResultErr.Value, definedSet, resourceSet) {
			return false
		}
		return true
	case DefinedKindOwn:
		return resourceSet[dt.Own]
	case DefinedKindBorrow:
		return resourceSet[dt.Borrow]
	}
	return true
}

// allValTypesNamedInFunc checks every param and result of a func type is named.
func (v *Validator) allValTypesNamedInFunc(funcID ComponentFuncTypeID, definedSet map[ComponentDefinedTypeID]bool, resourceSet map[ResourceID]bool) bool {
	ft := v.arena.FuncTypes[funcID]
	for _, p := range ft.Params {
		if !v.typeNamedValType(p.Type, definedSet, resourceSet) {
			return false
		}
	}
	for _, r := range ft.Results {
		if !v.typeNamedValType(r.Type, definedSet, resourceSet) {
			return false
		}
	}
	return true
}

// allValTypesNamedInInstance walks every export of an instance type, requiring each
// to satisfy the same naming rules as a top-level export of the surrounding scope.
func (v *Validator) allValTypesNamedInInstance(id ComponentInstanceTypeID, definedSet map[ComponentDefinedTypeID]bool, resourceSet map[ResourceID]bool) bool {
	instType := v.arena.InstanceTypes[id]
	for _, expET := range instType.Exports {
		switch expET.Kind {
		case EntityType:
			if !v.allValTypesNamed(expET.TypeRef, definedSet, resourceSet) {
				return false
			}
		case EntityFunc:
			if !v.allValTypesNamedInFunc(expET.FuncID, definedSet, resourceSet) {
				return false
			}
		case EntityValue:
			if !v.typeNamedValType(expET.ValType, definedSet, resourceSet) {
				return false
			}
		case EntityInstance:
			if !v.allValTypesNamedInInstance(expET.InstID, definedSet, resourceSet) {
				return false
			}
		}
	}
	return true
}

// typeNamedValType returns true if a ValTypeDesc reference is "named" for the given sets.
// Primitives are always named; defined-type references delegate to typeNamedTypeID.
func (v *Validator) typeNamedValType(ivt ValTypeDesc, definedSet map[ComponentDefinedTypeID]bool, resourceSet map[ResourceID]bool) bool {
	if ivt.IsPrimitive {
		return true
	}
	return v.typeNamedTypeID(ivt.TypeID, definedSet, resourceSet)
}

// typeNamedTypeID is the leaf "is this defined type named?" check. Record / Variant /
// Flags / Enum require their own ID to appear in definedSet. Structural composites
// (list / tuple / option / result) recurse into their components. Own / Borrow check
// that the underlying resource appears in resourceSet.
func (v *Validator) typeNamedTypeID(id ComponentDefinedTypeID, definedSet map[ComponentDefinedTypeID]bool, resourceSet map[ResourceID]bool) bool {
	dt := v.arena.DefinedTypes[id]
	switch dt.Kind {
	case DefinedKindPrimitive:
		return true
	case DefinedKindFlags, DefinedKindEnum, DefinedKindRecord, DefinedKindVariant:
		return definedSet[id]
	case DefinedKindList:
		return v.typeNamedValType(dt.List, definedSet, resourceSet)
	case DefinedKindTuple:
		for _, t := range dt.Tuple {
			if !v.typeNamedValType(t, definedSet, resourceSet) {
				return false
			}
		}
		return true
	case DefinedKindOption:
		return v.typeNamedValType(dt.Option, definedSet, resourceSet)
	case DefinedKindResult:
		if dt.ResultOk.Valid && !v.typeNamedValType(dt.ResultOk.Value, definedSet, resourceSet) {
			return false
		}
		if dt.ResultErr.Valid && !v.typeNamedValType(dt.ResultErr.Value, definedSet, resourceSet) {
			return false
		}
		return true
	case DefinedKindOwn:
		return resourceSet[dt.Own]
	case DefinedKindBorrow:
		return resourceSet[dt.Borrow]
	}
	return true
}

func (v *Validator) resolveExportedEntity(cs *ComponentState, kind ComponentExternalKind, index uint32) (ComponentEntityType, error) {
	switch kind {
	case ExternalKindModule:
		if _, err := cs.getCoreModule(index); err != nil {
			return ComponentEntityType{}, fmt.Errorf("module index out of bounds")
		}
		return ComponentEntityType{Kind: EntityModule, ModuleID: cs.coreModules[index]}, nil
	case ExternalKindFunc:
		if _, err := cs.getFunc(index); err != nil {
			return ComponentEntityType{}, fmt.Errorf("function index out of bounds")
		}
		return ComponentEntityType{Kind: EntityFunc, FuncID: cs.funcs[index]}, nil
	case ExternalKindValue:
		ve, err := cs.getValue(index)
		if err != nil {
			return ComponentEntityType{}, fmt.Errorf("value index out of bounds")
		}
		return ComponentEntityType{Kind: EntityValue, ValType: ve.ty}, nil
	case ExternalKindType:
		ty, err := cs.getType(index)
		if err != nil {
			return ComponentEntityType{}, fmt.Errorf("type index out of bounds")
		}
		return ComponentEntityType{Kind: EntityType, TypeRef: ty}, nil
	case ExternalKindComponent:
		if _, err := cs.getComponent(index); err != nil {
			return ComponentEntityType{}, fmt.Errorf("component index out of bounds")
		}
		return ComponentEntityType{Kind: EntityComponent, CompID: cs.components[index]}, nil
	case ExternalKindInstance:
		if _, err := cs.getInstance(index); err != nil {
			return ComponentEntityType{}, fmt.Errorf("instance index out of bounds")
		}
		return ComponentEntityType{Kind: EntityInstance, InstID: cs.instances[index]}, nil
	default:
		return ComponentEntityType{}, fmt.Errorf("unknown external kind")
	}
}

// ---------- Alias Section ----------

func (v *Validator) validateAliasSection(p *ComponentAliasSectionPayload) error {
	cs := v.current()
	if cs == nil {
		return fmt.Errorf("alias section outside component")
	}
	for alias, err := range p.Items() {
		if err != nil {
			return err
		}
		if err := v.addAlias(cs, alias); err != nil {
			return err
		}
	}
	return nil
}

// checkAliasInTypeDecl validates that an alias within a component/instance type
// declaration only refers to types or instances.
func (v *Validator) checkAliasInTypeDecl(alias ComponentAlias) error {
	switch a := alias.(type) {
	case AliasInstanceExport:
		if a.Kind != ExternalKindType && a.Kind != ExternalKindInstance {
			return fmt.Errorf("aliases in a component or instance type may only refer to types or instances")
		}
	case AliasOuter:
		// Outer aliases to types (core or component) are allowed
		// OuterAliasKindCoreType, OuterAliasKindType, OuterAliasKindComponent are OK
		if a.Kind == OuterAliasKindCoreModule {
			return fmt.Errorf("aliases in a component or instance type may only refer to types or instances")
		}
	case AliasCoreInstanceExport:
		return fmt.Errorf("aliases in a component or instance type may only refer to types or instances")
	}
	return nil
}

func (v *Validator) addAlias(cs *ComponentState, alias ComponentAlias) error {
	switch a := alias.(type) {
	case AliasInstanceExport:
		return v.aliasInstanceExport(cs, a)
	case AliasCoreInstanceExport:
		return v.aliasCoreInstanceExport(cs, a)
	case AliasOuter:
		return v.aliasOuter(cs, a)
	default:
		return fmt.Errorf("unknown alias type %T", alias)
	}
}

func (v *Validator) aliasInstanceExport(cs *ComponentState, a AliasInstanceExport) error {
	instID, err := cs.getInstance(a.Instance)
	if err != nil {
		return err
	}
	instType := v.arena.InstanceTypes[instID]
	et, ok := instType.Exports[a.Name]
	if !ok {
		return fmt.Errorf("instance %d has no export named `%s`", a.Instance, a.Name)
	}
	// Check that the alias kind matches the entity kind
	if err := checkAliasKindMatchesEntity(a.Kind, et, a.Instance, a.Name); err != nil {
		return err
	}
	return cs.addEntity(et)
}

func (v *Validator) aliasCoreInstanceExport(cs *ComponentState, a AliasCoreInstanceExport) error {
	ciID, err := cs.getCoreInstance(a.Instance)
	if err != nil {
		return err
	}
	ciType := v.arena.CoreInstanceTypes[ciID]
	et, ok := ciType.Exports[a.Name]
	if !ok {
		return fmt.Errorf("core instance %d has no export named `%s`", a.Instance, a.Name)
	}
	// Add to appropriate core index space based on kind
	switch a.Kind {
	case CoreSortFunc:
		if et.Kind != CoreEntityFunc {
			return fmt.Errorf("aliased core export is not a function")
		}
		cs.coreFuncs = append(cs.coreFuncs, et.Func)
	case CoreSortTable:
		if et.Kind != CoreEntityTable {
			return fmt.Errorf("aliased core export is not a table")
		}
		cs.coreTables = append(cs.coreTables, et.Table)
	case CoreSortMemory:
		if et.Kind != CoreEntityMemory {
			return fmt.Errorf("aliased core export is not a memory")
		}
		cs.coreMemories = append(cs.coreMemories, et.Memory)
	case CoreSortGlobal:
		if et.Kind != CoreEntityGlobal {
			return fmt.Errorf("aliased core export is not a global")
		}
		cs.coreGlobals = append(cs.coreGlobals, et.Global)
	case CoreSortType:
		// Core type aliases from instance exports - not common
		return fmt.Errorf("core type alias from instance not supported")
	case CoreSortModule:
		return fmt.Errorf("core module alias from instance not supported")
	case CoreSortInstance:
		return fmt.Errorf("core instance alias from instance not supported")
	default:
		return fmt.Errorf("unknown core sort in alias")
	}
	return nil
}

func (v *Validator) aliasOuter(cs *ComponentState, a AliasOuter) error {
	// Walk up the component stack a.Count levels
	stackLen := len(v.components)
	target := stackLen - 1 - int(a.Count)
	if target < 0 {
		return fmt.Errorf("invalid outer alias count of %d", a.Count)
	}
	outer := v.components[target]

	switch a.Kind {
	case OuterAliasKindCoreType:
		ct, err := outer.getCoreType(a.Index)
		if err != nil {
			return err
		}
		cs.coreTypes = append(cs.coreTypes, ct)
	case OuterAliasKindCoreModule:
		mod, err := outer.getCoreModule(a.Index)
		if err != nil {
			return err
		}
		cs.coreModules = append(cs.coreModules, mod)
	case OuterAliasKindType:
		ty, err := outer.getType(a.Index)
		if err != nil {
			return err
		}
		// Check that the aliased type doesn't reference resources defined in
		// ancestor components that are not accessible from the current component.
		if err := v.checkOuterAliasResourceScope(ty, a.Count); err != nil {
			return err
		}
		cs.types = append(cs.types, ty)
	case OuterAliasKindComponent:
		comp, err := outer.getComponent(a.Index)
		if err != nil {
			return err
		}
		cs.components = append(cs.components, comp)
	default:
		return fmt.Errorf("unknown outer alias kind")
	}
	return nil
}

// checkOuterAliasResourceScope validates that an outer-aliased type doesn't reference
// resources defined in ancestor components that are not accessible from the child.
// Only checks across COMPONENT boundaries (not type declaration boundaries).
func (v *Validator) checkOuterAliasResourceScope(ty ComponentAnyTypeID, count uint32) error {
	resourceIDs := v.gatherResourceIDs(ty)
	if len(resourceIDs) == 0 {
		return nil
	}

	// Walk from the current component's position up through ancestors.
	// We need to check if any COMPONENT (not type declaration) boundary
	// has resources that we're crossing.
	stackLen := len(v.components)
	current := v.components[stackLen-1]

	// Walk up from the position just above current to the target (inclusive)
	// and check only actual components (not type declarations)
	for i := stackLen - 2; i >= stackLen-1-int(count); i-- {
		ancestor := v.components[i]
		// Only check boundaries where the child is a real component
		// (crossing from component -> component means resources are scoped)
		if current.kind == ComponentKindComponent {
			for rid := range resourceIDs {
				if ancestor.localResources[rid] {
					return fmt.Errorf("refers to resources not defined in the current component")
				}
				// Resource exports are minted as fresh IDs aliasing the
				// underlying defining ID; resolve once so an outer-aliased
				// type that re-exports a locally defined resource is
				// rejected by its defining ancestor.
				if defining := v.arena.resolveResourceAlias(rid); defining != rid && ancestor.localResources[defining] {
					return fmt.Errorf("refers to resources not defined in the current component")
				}
			}
		}
		current = ancestor
	}

	return nil
}

// gatherResourceIDs returns the set of resource IDs referenced by a type.
func (v *Validator) gatherResourceIDs(ty ComponentAnyTypeID) map[ResourceID]bool {
	set := make(map[ResourceID]bool)
	v.collectResourceIDs(ty, set)
	return set
}

// collectResourceIDs collects all resource IDs referenced by a ComponentAnyTypeID.
func (v *Validator) collectResourceIDs(ty ComponentAnyTypeID, set map[ResourceID]bool) {
	switch ty.Kind {
	case AnyTypeResource:
		set[ty.ResID] = true
	case AnyTypeDefined:
		v.collectResourceIDsFromDefined(ComponentDefinedTypeID(ty.Index), set)
	case AnyTypeFunc:
		v.collectResourceIDsFromFunc(ComponentFuncTypeID(ty.Index), set)
	case AnyTypeInstance:
		v.collectResourceIDsFromInstance(ComponentInstanceTypeID(ty.Index), set)
	case AnyTypeComponent:
		v.collectResourceIDsFromComponent(ComponentTypeID(ty.Index), set)
	}
}

func (v *Validator) collectResourceIDsFromDefined(id ComponentDefinedTypeID, set map[ResourceID]bool) {
	dt := v.arena.DefinedTypes[id]
	v.walkDefinedType(&dt, func(d *DefinedTypeDesc) bool {
		switch d.Kind {
		case DefinedKindOwn:
			set[d.Own] = true
		case DefinedKindBorrow:
			set[d.Borrow] = true
		}
		return false
	})
}

func (v *Validator) collectResourceIDsFromValType(ivt ValTypeDesc, set map[ResourceID]bool) {
	v.walkValType(ivt, func(dt *DefinedTypeDesc) bool {
		switch dt.Kind {
		case DefinedKindOwn:
			set[dt.Own] = true
		case DefinedKindBorrow:
			set[dt.Borrow] = true
		}
		return false
	})
}

func (v *Validator) collectResourceIDsFromFunc(id ComponentFuncTypeID, set map[ResourceID]bool) {
	ft := v.arena.FuncTypes[id]
	for _, p := range ft.Params {
		v.collectResourceIDsFromValType(p.Type, set)
	}
	for _, r := range ft.Results {
		v.collectResourceIDsFromValType(r.Type, set)
	}
}

func (v *Validator) collectResourceIDsFromInstance(id ComponentInstanceTypeID, set map[ResourceID]bool) {
	it := v.arena.InstanceTypes[id]
	for _, et := range it.Exports {
		v.collectResourceIDsFromEntity(et, set)
	}
}

func (v *Validator) collectResourceIDsFromComponent(id ComponentTypeID, set map[ResourceID]bool) {
	ct := v.arena.ComponentTypes[id]
	for _, et := range ct.Imports {
		v.collectResourceIDsFromEntity(et, set)
	}
	for _, et := range ct.Exports {
		v.collectResourceIDsFromEntity(et, set)
	}
}

func (v *Validator) collectResourceIDsFromEntity(et ComponentEntityType, set map[ResourceID]bool) {
	switch et.Kind {
	case EntityType:
		v.collectResourceIDs(et.TypeRef, set)
	case EntityFunc:
		v.collectResourceIDsFromFunc(et.FuncID, set)
	case EntityInstance:
		v.collectResourceIDsFromInstance(et.InstID, set)
	case EntityComponent:
		v.collectResourceIDsFromComponent(et.CompID, set)
	case EntityValue:
		v.collectResourceIDsFromValType(et.ValType, set)
	}
}

func checkAliasKindMatchesEntity(kind ComponentExternalKind, et ComponentEntityType, instanceIdx uint32, name string) error {
	kindName := func() string {
		switch kind {
		case ExternalKindModule:
			return "module"
		case ExternalKindFunc:
			return "func"
		case ExternalKindValue:
			return "value"
		case ExternalKindType:
			return "type"
		case ExternalKindComponent:
			return "component"
		case ExternalKindInstance:
			return "instance"
		}
		return "unknown"
	}()
	switch kind {
	case ExternalKindModule:
		if et.Kind != EntityModule {
			return fmt.Errorf("export `%s` for instance %d is not a %s", name, instanceIdx, kindName)
		}
	case ExternalKindFunc:
		if et.Kind != EntityFunc {
			return fmt.Errorf("export `%s` for instance %d is not a %s", name, instanceIdx, kindName)
		}
	case ExternalKindValue:
		if et.Kind != EntityValue {
			return fmt.Errorf("export `%s` for instance %d is not a %s", name, instanceIdx, kindName)
		}
	case ExternalKindType:
		if et.Kind != EntityType {
			return fmt.Errorf("export `%s` for instance %d is not a %s", name, instanceIdx, kindName)
		}
	case ExternalKindComponent:
		if et.Kind != EntityComponent {
			return fmt.Errorf("export `%s` for instance %d is not a %s", name, instanceIdx, kindName)
		}
	case ExternalKindInstance:
		if et.Kind != EntityInstance {
			return fmt.Errorf("export `%s` for instance %d is not a %s", name, instanceIdx, kindName)
		}
	}
	return nil
}

// ---------- Canonical Section ----------
