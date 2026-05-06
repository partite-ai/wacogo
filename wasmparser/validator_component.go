package wasmparser

import (
	"fmt"
	"strings"
)

func (v *Validator) validateTypeSection(p *ComponentTypeSectionPayload) error {
	cs := v.current()
	if cs == nil {
		return fmt.Errorf("type section outside component")
	}
	for ct, err := range p.Items() {
		if err != nil {
			return err
		}
		if err := v.addType(cs, ct); err != nil {
			return err
		}
	}
	return nil
}

func (v *Validator) addType(cs *ComponentState, ct ComponentTypeDef) error {
	switch t := ct.(type) {
	case *ComponentDefinedTypeEntry:
		return v.addDefinedType(cs, t.Type)
	case *ComponentFuncType:
		return v.addFuncType(cs, t)
	case *ComponentTypeDecl:
		return v.addComponentTypeDecl(cs, t)
	case *InstanceTypeDecl:
		return v.addInstanceTypeDecl(cs, t)
	case *ResourceType:
		return v.addResourceType(cs, t)
	default:
		return fmt.Errorf("unknown component type %T", ct)
	}
}

// maxDefinedTypeNesting bounds recursive descent through defined-type trees.
// Matches wasm-tools's limit so spec tests that deliberately exceed the bound
// produce the expected validation error.
const maxDefinedTypeNesting = 100

func (v *Validator) addDefinedType(cs *ComponentState, dt ComponentDefinedType) error {
	idt, err := v.resolveDefinedType(cs, dt)
	if err != nil {
		return err
	}
	if depth := v.definedTypeDepth(&idt); depth > maxDefinedTypeNesting {
		return fmt.Errorf("type nesting is too deep")
	}
	id := v.arena.pushDefinedType(idt)
	if v.arena.DefinedTypeSizes[id] > MaxEffectiveTypeSize {
		return fmt.Errorf("effective type size exceeds the limit")
	}
	cs.types = append(cs.types, ComponentAnyTypeID{Kind: AnyTypeDefined, Index: uint32(id)})
	return nil
}

// definedTypeDepth computes the maximum nesting depth of a defined type. The
// depth of a leaf (primitive/enum/flags/own/borrow) is 1; composite types add
// 1 to the max depth of their children.
func (v *Validator) definedTypeDepth(dt *DefinedTypeDesc) int {
	maxChild := 0
	switch dt.Kind {
	case DefinedKindRecord:
		if dt.Record != nil {
			for _, f := range dt.Record.Fields {
				if d := v.valTypeDepth(f.Type); d > maxChild {
					maxChild = d
				}
			}
		}
	case DefinedKindVariant:
		if dt.Variant != nil {
			for _, c := range dt.Variant.Cases {
				if c.Type.Valid {
					if d := v.valTypeDepth(c.Type.Value); d > maxChild {
						maxChild = d
					}
				}
			}
		}
	case DefinedKindList:
		maxChild = v.valTypeDepth(dt.List)
	case DefinedKindTuple:
		for _, t := range dt.Tuple {
			if d := v.valTypeDepth(t); d > maxChild {
				maxChild = d
			}
		}
	case DefinedKindOption:
		maxChild = v.valTypeDepth(dt.Option)
	case DefinedKindResult:
		if dt.ResultOk.Valid {
			if d := v.valTypeDepth(dt.ResultOk.Value); d > maxChild {
				maxChild = d
			}
		}
		if dt.ResultErr.Valid {
			if d := v.valTypeDepth(dt.ResultErr.Value); d > maxChild {
				maxChild = d
			}
		}
	}
	return 1 + maxChild
}

func (v *Validator) valTypeDepth(ivt ValTypeDesc) int {
	if ivt.IsPrimitive {
		return 1
	}
	dt := v.arena.DefinedTypes[ivt.TypeID]
	return v.definedTypeDepth(&dt)
}

func (v *Validator) resolveDefinedType(cs *ComponentState, dt ComponentDefinedType) (DefinedTypeDesc, error) {
	switch t := dt.(type) {
	case PrimitiveValType:
		return DefinedTypeDesc{Kind: DefinedKindPrimitive, Primitive: t}, nil
	case TypeIndexValType:
		// Type alias - resolve to the underlying defined type
		idx := uint32(t)
		if idx >= uint32(len(cs.types)) {
			return DefinedTypeDesc{}, fmt.Errorf("type index out of bounds")
		}
		ty := cs.types[idx]
		if ty.Kind != AnyTypeDefined {
			return DefinedTypeDesc{}, fmt.Errorf("type index %d is not a defined type", idx)
		}
		return v.arena.DefinedTypes[ty.Index], nil
	case *RecordType:
		if len(t.Fields) == 0 {
			return DefinedTypeDesc{}, fmt.Errorf("record type must have at least one field")
		}
		fields := make([]FieldDesc, len(t.Fields))
		names := make(map[string]string)
		for i, f := range t.Fields {
			if f.Name == "" {
				return DefinedTypeDesc{}, fmt.Errorf("name cannot be empty")
			}
			if !IsKebabCase(f.Name) {
				return DefinedTypeDesc{}, fmt.Errorf("record field name `%s` is not in kebab case", f.Name)
			}
			lower := strings.ToLower(f.Name)
			if prev, ok := names[lower]; ok {
				return DefinedTypeDesc{}, fmt.Errorf("record field name `%s` conflicts with previous field name `%s`", f.Name, prev)
			}
			names[lower] = f.Name
			ivt, err := internalValTypeFromParsed(f.Type, cs)
			if err != nil {
				return DefinedTypeDesc{}, err
			}
			fields[i] = FieldDesc{Name: f.Name, Type: ivt}
		}
		return DefinedTypeDesc{
			Kind:   DefinedKindRecord,
			Record: &RecordTypeDesc{Fields: fields},
		}, nil
	case *VariantType:
		if len(t.Cases) == 0 {
			return DefinedTypeDesc{}, fmt.Errorf("variant type must have at least one case")
		}
		cases := make([]VariantCaseDesc, len(t.Cases))
		names := make(map[string]string)
		for i, c := range t.Cases {
			if c.Name == "" {
				return DefinedTypeDesc{}, fmt.Errorf("name cannot be empty")
			}
			if !IsKebabCase(c.Name) {
				return DefinedTypeDesc{}, fmt.Errorf("variant case name `%s` is not in kebab case", c.Name)
			}
			lower := strings.ToLower(c.Name)
			if prev, ok := names[lower]; ok {
				return DefinedTypeDesc{}, fmt.Errorf("variant case name `%s` conflicts with previous case name `%s`", c.Name, prev)
			}
			names[lower] = c.Name
			var caseType Optional[ValTypeDesc]
			if c.Type.Valid {
				ivt, err := internalValTypeFromParsed(c.Type.Value, cs)
				if err != nil {
					return DefinedTypeDesc{}, err
				}
				caseType = Some(ivt)
			}
			if c.Refines.Valid {
				if c.Refines.Value >= uint32(i) {
					return DefinedTypeDesc{}, fmt.Errorf("variant case refines index out of bounds")
				}
			}
			cases[i] = VariantCaseDesc{Name: c.Name, Type: caseType, Refines: c.Refines}
		}
		return DefinedTypeDesc{
			Kind:    DefinedKindVariant,
			Variant: &VariantTypeDesc{Cases: cases},
		}, nil
	case *ListType:
		ivt, err := internalValTypeFromParsed(t.Element, cs)
		if err != nil {
			return DefinedTypeDesc{}, err
		}
		return DefinedTypeDesc{Kind: DefinedKindList, List: ivt}, nil
	case *TupleType:
		if len(t.Types) == 0 {
			return DefinedTypeDesc{}, fmt.Errorf("tuple type must have at least one type")
		}
		types := make([]ValTypeDesc, len(t.Types))
		for i, ty := range t.Types {
			ivt, err := internalValTypeFromParsed(ty, cs)
			if err != nil {
				return DefinedTypeDesc{}, err
			}
			types[i] = ivt
		}
		return DefinedTypeDesc{Kind: DefinedKindTuple, Tuple: types}, nil
	case *FlagsType:
		if len(t.Labels) == 0 {
			return DefinedTypeDesc{}, fmt.Errorf("flags must have at least one entry")
		}
		if len(t.Labels) > 32 {
			return DefinedTypeDesc{}, fmt.Errorf("cannot have more than 32 flags")
		}
		flagNames := make(map[string]string)
		for _, l := range t.Labels {
			if l == "" {
				return DefinedTypeDesc{}, fmt.Errorf("name cannot be empty")
			}
			if !IsKebabCase(l) {
				return DefinedTypeDesc{}, fmt.Errorf("flag name `%s` is not in kebab case", l)
			}
			lower := strings.ToLower(l)
			if prev, ok := flagNames[lower]; ok {
				return DefinedTypeDesc{}, fmt.Errorf("flag name `%s` conflicts with previous flag name `%s`", l, prev)
			}
			flagNames[lower] = l
		}
		return DefinedTypeDesc{Kind: DefinedKindFlags, Flags: t.Labels}, nil
	case *EnumType:
		if len(t.Labels) == 0 {
			return DefinedTypeDesc{}, fmt.Errorf("enum type must have at least one variant")
		}
		enumNames := make(map[string]string)
		for _, l := range t.Labels {
			if l == "" {
				return DefinedTypeDesc{}, fmt.Errorf("name cannot be empty")
			}
			if !IsKebabCase(l) {
				return DefinedTypeDesc{}, fmt.Errorf("enum tag name `%s` is not in kebab case", l)
			}
			lower := strings.ToLower(l)
			if prev, ok := enumNames[lower]; ok {
				return DefinedTypeDesc{}, fmt.Errorf("enum tag name `%s` conflicts with previous tag name `%s`", l, prev)
			}
			enumNames[lower] = l
		}
		return DefinedTypeDesc{Kind: DefinedKindEnum, Enum: t.Labels}, nil
	case *OptionType:
		ivt, err := internalValTypeFromParsed(t.Inner, cs)
		if err != nil {
			return DefinedTypeDesc{}, err
		}
		return DefinedTypeDesc{Kind: DefinedKindOption, Option: ivt}, nil
	case *ResultType:
		var ok Optional[ValTypeDesc]
		if t.Ok.Valid {
			ivt, err := internalValTypeFromParsed(t.Ok.Value, cs)
			if err != nil {
				return DefinedTypeDesc{}, err
			}
			ok = Some(ivt)
		}
		var errT Optional[ValTypeDesc]
		if t.Err.Valid {
			ivt, err := internalValTypeFromParsed(t.Err.Value, cs)
			if err != nil {
				return DefinedTypeDesc{}, err
			}
			errT = Some(ivt)
		}
		return DefinedTypeDesc{Kind: DefinedKindResult, ResultOk: ok, ResultErr: errT}, nil
	case *OwnType:
		ty, err := cs.getType(t.ResourceIndex)
		if err != nil {
			return DefinedTypeDesc{}, err
		}
		if ty.Kind != AnyTypeResource {
			return DefinedTypeDesc{}, fmt.Errorf("type index %d is not a resource type", t.ResourceIndex)
		}
		return DefinedTypeDesc{Kind: DefinedKindOwn, Own: ty.ResID}, nil
	case *BorrowType:
		ty, err := cs.getType(t.ResourceIndex)
		if err != nil {
			return DefinedTypeDesc{}, err
		}
		if ty.Kind != AnyTypeResource {
			return DefinedTypeDesc{}, fmt.Errorf("type index %d is not a resource type", t.ResourceIndex)
		}
		return DefinedTypeDesc{Kind: DefinedKindBorrow, Borrow: ty.ResID}, nil
	default:
		return DefinedTypeDesc{}, fmt.Errorf("unknown defined type %T", dt)
	}
}

func (v *Validator) addFuncType(cs *ComponentState, ft *ComponentFuncType) error {
	params := make([]FuncParamDesc, len(ft.Params))
	paramNames := make(map[string]string)
	for i, p := range ft.Params {
		if p.Name == "" {
			return fmt.Errorf("function parameter name cannot be empty")
		}
		if !IsKebabCase(p.Name) {
			return fmt.Errorf("function parameter name `%s` is not in kebab case", p.Name)
		}
		lower := strings.ToLower(p.Name)
		if prev, ok := paramNames[lower]; ok {
			return fmt.Errorf("function parameter name `%s` conflicts with previous parameter name `%s`", p.Name, prev)
		}
		paramNames[lower] = p.Name
		ivt, err := internalValTypeFromParsed(p.Type, cs)
		if err != nil {
			return err
		}
		params[i] = FuncParamDesc{Name: p.Name, Type: ivt}
	}
	results := make([]FuncParamDesc, len(ft.Results))
	resultNames := make(map[string]bool)
	for i, r := range ft.Results {
		// Results can have empty names (single unnamed result)
		if r.Name != "" {
			if !IsKebabCase(r.Name) {
				return fmt.Errorf("`%s` is not in kebab case", r.Name)
			}
			lower := strings.ToLower(r.Name)
			if resultNames[lower] {
				return fmt.Errorf("duplicate function result name `%s`", r.Name)
			}
			resultNames[lower] = true
		}
		ivt, err := internalValTypeFromParsed(r.Type, cs)
		if err != nil {
			return err
		}
		results[i] = FuncParamDesc{Name: r.Name, Type: ivt}
		// Check that results don't contain borrow types
		if v.valTypeContainsBorrow(ivt) {
			return fmt.Errorf("function result cannot contain a `borrow` type")
		}
	}
	id := v.arena.pushFuncType(FuncTypeDesc{Params: params, Results: results})
	if v.arena.FuncTypeSizes[id] > MaxEffectiveTypeSize {
		return fmt.Errorf("effective type size exceeds the limit")
	}
	cs.types = append(cs.types, ComponentAnyTypeID{Kind: AnyTypeFunc, Index: uint32(id)})
	return nil
}

// valTypeContainsBorrow checks if a value type contains any borrow types.
func (v *Validator) valTypeContainsBorrow(ivt ValTypeDesc) bool {
	return v.walkValType(ivt, func(dt *DefinedTypeDesc) bool {
		return dt.Kind == DefinedKindBorrow
	})
}

func (v *Validator) addResourceType(cs *ComponentState, rt *ResourceType) error {
	if cs.kind != ComponentKindComponent {
		return fmt.Errorf("resources can only be defined within a concrete component")
	}
	// Validate destructor if present
	if rt.Dtor.Valid {
		funcID, err := cs.getCoreFunc(rt.Dtor.Value)
		if err != nil {
			return fmt.Errorf("resource destructor %s", err)
		}
		// Destructor must take [i32] -> []
		dtorType := v.arena.CoreFuncTypes[funcID]
		if len(dtorType.Params) != 1 || dtorType.Params[0] != CoreValTypeI32 || len(dtorType.Results) != 0 {
			return fmt.Errorf("wrong signature for a destructor")
		}
	}
	// Validate representation type
	if rt.Rep != ValTypeI32 {
		return fmt.Errorf("resources can only be represented by `i32`")
	}
	rid := v.arena.allocResourceID()
	cs.localResources[rid] = true
	cs.types = append(cs.types, ComponentAnyTypeID{Kind: AnyTypeResource, ResID: rid})
	return nil
}

func (v *Validator) addComponentTypeDecl(cs *ComponentState, ctd *ComponentTypeDecl) error {
	// Create a sub-state for the component type declarations
	subCS := newComponentState(v.features, v.arena, ComponentKindComponentType)

	// Push subCS onto the components stack so outer aliases can resolve
	v.components = append(v.components, subCS)
	defer func() {
		v.components = v.components[:len(v.components)-1]
	}()

	for _, decl := range ctd.Declarations {
		if err := v.processComponentTypeDecl(subCS, decl); err != nil {
			return err
		}
	}

	// Build the component type from subCS
	compType := ComponentTypeDesc{
		Imports:     subCS.imports,
		ImportOrder: subCS.importOrder,
		Exports:     subCS.exports,
		ExportOrder: subCS.exportOrder,
	}
	id := v.arena.pushComponentType(compType)
	if v.arena.ComponentTypeSizes[id] > MaxEffectiveTypeSize {
		return fmt.Errorf("effective type size exceeds the limit")
	}
	cs.types = append(cs.types, ComponentAnyTypeID{Kind: AnyTypeComponent, Index: uint32(id)})
	return nil
}

func (v *Validator) processComponentTypeDecl(subCS *ComponentState, decl ComponentTypeDeclaration) error {
	switch d := decl.(type) {
	case InstanceDeclCoreType:
		return v.addCoreType(subCS, d.Type)
	case InstanceDeclType:
		return v.addType(subCS, d.Type)
	case InstanceDeclAlias:
		if err := v.checkAliasInTypeDecl(d.Alias); err != nil {
			return err
		}
		return v.addAlias(subCS, d.Alias)
	case InstanceDeclExport:
		return v.addExportDecl(subCS, d.Export)
	case ComponentDeclImport:
		return v.addImportDecl(subCS, d.Import)
	default:
		return fmt.Errorf("unknown component type declaration %T", decl)
	}
}

func (v *Validator) addInstanceTypeDecl(cs *ComponentState, itd *InstanceTypeDecl) error {
	// Create a sub-state for the instance type declarations
	subCS := newComponentState(v.features, v.arena, ComponentKindInstanceType)

	// Push subCS onto the components stack so outer aliases can resolve
	v.components = append(v.components, subCS)
	defer func() {
		v.components = v.components[:len(v.components)-1]
	}()

	for _, decl := range itd.Declarations {
		if err := v.processInstanceTypeDecl(subCS, decl); err != nil {
			return err
		}
	}

	// Build the instance type from subCS
	// Collect defined resources: resources exported as types with sub-resource
	// bounds that were allocated (not aliased in) by this instance type. Outer
	// aliases pull in existing resource IDs; those are NOT defined here and
	// must not be freshened on import.
	var definedResources []ResourceID
	for _, et := range subCS.exports {
		if et.Kind == EntityType && et.TypeRef.Kind == AnyTypeResource &&
			subCS.locallyDefinedSubResources[et.TypeRef.ResID] {
			definedResources = append(definedResources, et.TypeRef.ResID)
		}
	}
	instType := InstanceTypeDesc{
		Exports:          subCS.exports,
		DefinedResources: definedResources,
	}
	id := v.arena.pushInstanceType(instType)
	if v.arena.InstanceTypeSizes[id] > MaxEffectiveTypeSize {
		return fmt.Errorf("effective type size exceeds the limit")
	}
	cs.types = append(cs.types, ComponentAnyTypeID{Kind: AnyTypeInstance, Index: uint32(id)})
	return nil
}

func (v *Validator) processInstanceTypeDecl(subCS *ComponentState, decl InstanceTypeDeclaration) error {
	switch d := decl.(type) {
	case InstanceDeclCoreType:
		return v.addCoreType(subCS, d.Type)
	case InstanceDeclType:
		return v.addType(subCS, d.Type)
	case InstanceDeclAlias:
		if err := v.checkAliasInTypeDecl(d.Alias); err != nil {
			return err
		}
		return v.addAlias(subCS, d.Alias)
	case InstanceDeclExport:
		return v.addExportDecl(subCS, d.Export)
	default:
		return fmt.Errorf("unknown instance type declaration %T", decl)
	}
}

// ---------- Import Section ----------

func (v *Validator) validateImportSection(p *ComponentImportSectionPayload) error {
	cs := v.current()
	if cs == nil {
		return fmt.Errorf("import section outside component")
	}
	v.lastImportEntries = v.lastImportEntries[:0]
	for imp, err := range p.Items() {
		if err != nil {
			return err
		}
		et, err := v.addImportCapturing(cs, imp)
		if err != nil {
			return err
		}
		v.lastImportEntries = append(v.lastImportEntries, et)
	}
	return nil
}

func (v *Validator) addImport(cs *ComponentState, imp *ComponentImport) error {
	_, err := v.addImportCapturing(cs, imp)
	return err
}

func (v *Validator) addImportCapturing(cs *ComponentState, imp *ComponentImport) (ComponentEntityType, error) {
	// Check name uniqueness first (before format validation)
	if err := cs.checkImportName(imp.Name.Name); err != nil {
		return ComponentEntityType{}, err
	}

	// Validate the name format
	if err := ValidateImportName(imp.Name); err != nil {
		return ComponentEntityType{}, fmt.Errorf("`%s` is not a valid extern name: %s", imp.Name.Name, err)
	}

	// Resolve type ref to entity type
	et, err := cs.typeRefToEntityType(imp.Type)
	if err != nil {
		return ComponentEntityType{}, err
	}

	// Freshen resources for imported instances that define resources
	et = v.freshenImportResources(et)

	// Mint a fresh "created" identity for type entities so subsequent
	// references to the import name resolve to a different ID than the
	// original declaration. Skip when typeRefToEntityType already minted
	// a fresh resource ID via a (sub resource) bound.
	et = v.freshenTypeEntityCreated(cs, et, importTypeRefIsSubResource(imp.Type))

	// Validate that the entity type is valid for use as an import
	if err := v.validateAndRegisterNamedTypes(cs, et, "import"); err != nil {
		return ComponentEntityType{}, err
	}

	// Validate bracket-prefixed names against the entity type
	if err := v.validateBracketName(cs, imp.Name.Name, et, "import"); err != nil {
		return ComponentEntityType{}, err
	}

	// If this is a type import with SubResource bounds, register the resource name
	if et.Kind == EntityType && et.TypeRef.Kind == AnyTypeResource {
		addResourceName(v.arena, cs.resourceNames, et.TypeRef.ResID, imp.Name.Name)
	}

	// Add entity to index space
	if err := cs.addEntity(et); err != nil {
		return ComponentEntityType{}, err
	}

	// Record in imports map
	cs.imports[imp.Name.Name] = et
	cs.importOrder = append(cs.importOrder, imp.Name.Name)
	return et, nil
}

// freshenTypeEntityCreated mints a new "created" identity for a type-entity
// import or export, mirroring wasm-tools' validate_and_register_named_types
// where the entity carries a created ID distinct from the referenced source.
//
// Resource entities get a freshly allocated ResourceID and the mapping
// fresh→defining is recorded in cs.resourceAliases. Defined-type entities get
// a copy of the source DefinedTypeDesc pushed at a new arena slot.
//
// The fresh identity ensures subsequent references via the import/export name
// (e.g. through a (export $alias "x" (type $orig)) binding) resolve to a
// different ID than references to the original declaration — what lets the
// named-type tracking distinguish unexported sources from their exported
// aliases, and what wasm-tools encodes via per-export "created" IDs.
//
// fromSubResourceBound indicates the entity was constructed from a (sub
// resource) bound, in which case typeRefToEntityType has already allocated a
// fresh ResourceID and tagged it in locallyDefinedSubResources; minting again
// would leak that ID and lose the tracking.
func (v *Validator) freshenTypeEntityCreated(cs *ComponentState, et ComponentEntityType, fromSubResourceBound bool) ComponentEntityType {
	if et.Kind != EntityType {
		return et
	}
	switch et.TypeRef.Kind {
	case AnyTypeResource:
		if fromSubResourceBound {
			return et
		}
		src := et.TypeRef.ResID
		fresh := v.arena.allocResourceID()
		// Record the alias on the arena, resolving any pre-existing alias
		// one hop so the recorded defining ID is the original declaration,
		// never another export alias.
		v.arena.resourceAliases[fresh] = v.arena.resolveResourceAlias(src)
		// The alias represents the same underlying resource as its source.
		// Carry forward local-resource flagging so (canon resource.new/rep)
		// still treats the alias as locally defined when the source was.
		if cs.localResources[src] {
			cs.localResources[fresh] = true
		}
		et.TypeRef.ResID = fresh
	case AnyTypeDefined:
		srcID := ComponentDefinedTypeID(et.TypeRef.Index)
		srcDT := v.arena.DefinedTypes[srcID]
		et.TypeRef.Index = uint32(v.arena.pushDefinedType(srcDT))
	}
	return et
}

// importTypeRefIsSubResource reports whether the supplied type ref carries a
// (sub resource) bound — used to skip a redundant freshen in
// freshenTypeEntityCreated.
func importTypeRefIsSubResource(tr ComponentTypeRef) bool {
	rt, ok := tr.(TypeRefType)
	if !ok {
		return false
	}
	_, ok = rt.Bounds.(TypeBoundsSubResource)
	return ok
}

// freshenExportResources freshens resource IDs for exported instances with defined
// resources in type declarations. Each export of an instance type gets fresh resource
// IDs so that two exports of the same type produce distinct resources.
func (v *Validator) freshenExportResources(et ComponentEntityType) ComponentEntityType {
	if et.Kind != EntityInstance {
		return et
	}
	instType := v.arena.InstanceTypes[et.InstID]
	if len(instType.DefinedResources) == 0 {
		return et
	}

	// Build remapping from old defined resource IDs to fresh ones
	sc := v.newSubtypeCheckerWithRemapping()
	for _, oldRes := range instType.DefinedResources {
		freshRes := v.arena.allocResourceID()
		sc.resourceMapping[oldRes] = scopedResID{scope: nil, arena: v.arena, id: freshRes}
	}

	// Remap the instance type's exports and clear defined resources
	// (the concrete exported instance has no defined resources — they've been freshened)
	newInstID := sc.remapInstanceType(et.InstID)
	return ComponentEntityType{Kind: EntityInstance, InstID: newInstID}
}

// freshenImportResources freshens resource IDs for imported instances that define
// resources. Each import of an instance type with defined resources gets fresh
// resource IDs so that two imports of the same instance type have distinct resources.
func (v *Validator) freshenImportResources(et ComponentEntityType) ComponentEntityType {
	if et.Kind != EntityInstance {
		return et
	}
	instType := v.arena.InstanceTypes[et.InstID]
	if len(instType.DefinedResources) == 0 {
		return et
	}

	// Build remapping from old defined resource IDs to fresh ones
	sc := v.newSubtypeCheckerWithRemapping()
	for _, oldRes := range instType.DefinedResources {
		freshRes := v.arena.allocResourceID()
		sc.resourceMapping[oldRes] = scopedResID{scope: nil, arena: v.arena, id: freshRes}
	}

	// Remap the instance type's exports
	newInstID := sc.remapInstanceType(et.InstID)
	return ComponentEntityType{Kind: EntityInstance, InstID: newInstID}
}

// addImportDecl handles an import within a type declaration.
func (v *Validator) addImportDecl(cs *ComponentState, imp ComponentImport) error {
	// Validate the name
	if err := ValidateImportName(imp.Name); err != nil {
		return fmt.Errorf("`%s` is not a valid extern name: %s", imp.Name.Name, err)
	}

	// Check name uniqueness
	if err := cs.checkImportName(imp.Name.Name); err != nil {
		return err
	}

	// Resolve type ref to entity type
	et, err := cs.typeRefToEntityType(imp.Type)
	if err != nil {
		return err
	}

	// Mint a fresh "created" identity for type entities (see addImportCapturing).
	et = v.freshenTypeEntityCreated(cs, et, importTypeRefIsSubResource(imp.Type))

	// Validate that the entity type is valid for use as an import
	if err := v.validateAndRegisterNamedTypes(cs, et, "import"); err != nil {
		return err
	}

	// Validate bracket-prefixed names
	if err := v.validateBracketName(cs, imp.Name.Name, et, "import"); err != nil {
		return err
	}

	// If this is a type import with SubResource bounds, register the resource name
	if et.Kind == EntityType && et.TypeRef.Kind == AnyTypeResource {
		addResourceName(v.arena, cs.resourceNames, et.TypeRef.ResID, imp.Name.Name)
	}

	// Add entity to index space
	if err := cs.addEntity(et); err != nil {
		return err
	}

	// Record in imports map
	cs.imports[imp.Name.Name] = et
	cs.importOrder = append(cs.importOrder, imp.Name.Name)
	return nil
}

// ---------- Export Section ----------

func (v *Validator) validateExportSection(p *ComponentExportSectionPayload) error {
	cs := v.current()
	if cs == nil {
		return fmt.Errorf("export section outside component")
	}
	v.lastExportEntries = v.lastExportEntries[:0]
	for exp, err := range p.Items() {
		if err != nil {
			return err
		}
		et, err := v.addExportCapturing(cs, exp)
		if err != nil {
			return err
		}
		v.lastExportEntries = append(v.lastExportEntries, et)
	}
	return nil
}

func (v *Validator) addExport(cs *ComponentState, exp *ComponentExport) error {
	_, err := v.addExportCapturing(cs, exp)
	return err
}

func (v *Validator) addExportCapturing(cs *ComponentState, exp *ComponentExport) (ComponentEntityType, error) {
	// Get entity type from the exported index first (check bounds before name validation)
	et, err := v.resolveExportedEntity(cs, exp.Kind, exp.Index)
	if err != nil {
		return ComponentEntityType{}, err
	}

	// If there's an ascribed type, check that the actual entity type is a
	// subtype of the ascribed type, then narrow to the ascription.
	if exp.AscribedType.Valid {
		ascribed, err := cs.typeRefToEntityType(exp.AscribedType.Value)
		if err != nil {
			return ComponentEntityType{}, err
		}
		sc := v.newSubtypeCheckerWithRemapping()
		if err := sc.isSubtype(et, ascribed); err != nil {
			return ComponentEntityType{}, fmt.Errorf("ascribed type of export is not compatible: %w", err)
		}
		et = ascribed
	}

	// Check name uniqueness
	if err := cs.checkExportName(exp.Name.Name); err != nil {
		return ComponentEntityType{}, err
	}

	// Validate the name format
	if err := ValidateExportNameStr(exp.Name); err != nil {
		if _, ok := err.(*ErrNotValidExternName); ok {
			return ComponentEntityType{}, fmt.Errorf("`%s` is not a valid extern name: %s", exp.Name.Name, err)
		}
		return ComponentEntityType{}, fmt.Errorf("`%s` is not a valid export name: %s", exp.Name.Name, err)
	}

	// Mint a fresh "created" identity for type-entity exports (see
	// freshenTypeEntityCreated). At component level the source is always an
	// existing index reference, never a sub-resource bound.
	et = v.freshenTypeEntityCreated(cs, et, false)

	// Validate that the entity type is valid for use as an export
	if err := v.validateAndRegisterNamedTypes(cs, et, "export"); err != nil {
		return ComponentEntityType{}, err
	}

	// Validate bracket-prefixed names against the entity type
	if err := v.validateBracketName(cs, exp.Name.Name, et, "export"); err != nil {
		return ComponentEntityType{}, err
	}

	// If this is a type export with a resource, register the resource name.
	// A resource may be exported under multiple names; all are valid.
	if et.Kind == EntityType && et.TypeRef.Kind == AnyTypeResource {
		addResourceName(v.arena, cs.resourceNames, et.TypeRef.ResID, exp.Name.Name)
		addResourceName(v.arena, cs.exportResourceNamesMap, et.TypeRef.ResID, exp.Name.Name)
	}

	// Add the exported entity to the appropriate index space
	if err := cs.addEntity(et); err != nil {
		return ComponentEntityType{}, err
	}

	// Record in exports map
	cs.exports[exp.Name.Name] = et
	cs.exportOrder = append(cs.exportOrder, exp.Name.Name)
	return et, nil
}

// addExportDecl handles an export within a type declaration.
// In type declarations, exports encode a type reference directly (no sort+index).
func (v *Validator) addExportDecl(cs *ComponentState, exp ComponentExport) error {
	// In type declarations, resolve the type ref first (validates bounds)
	var et ComponentEntityType
	if exp.AscribedType.Valid {
		var err error
		et, err = cs.typeRefToEntityType(exp.AscribedType.Value)
		if err != nil {
			return err
		}
	} else {
		// Fallback: resolve from kind+index (shouldn't happen in type decls)
		var err error
		et, err = v.resolveExportedEntity(cs, exp.Kind, exp.Index)
		if err != nil {
			return err
		}
	}

	// Freshen resources for exported instances with defined resources
	// (in type declarations, each export of the same instance type gets fresh resource IDs)
	et = v.freshenExportResources(et)

	// Note: type-entity exports inside a type declaration intentionally do
	// NOT get a freshly minted identity. Their AscribedType always carries an
	// (eq ...) or (sub resource) bound, and our representation encodes the eq
	// constraint by reusing the referenced type's ID — minting a fresh ID here
	// would break that linkage and cause downstream subtype checks to lose the
	// eq relationship between, e.g., two instance imports that reference the
	// same alias.

	// Validate the name
	if err := ValidateExportNameStr(exp.Name); err != nil {
		if _, ok := err.(*ErrNotValidExternName); ok {
			return fmt.Errorf("`%s` is not a valid extern name: %s", exp.Name.Name, err)
		}
		return fmt.Errorf("`%s` is not a valid export name: %s", exp.Name.Name, err)
	}

	// Check name uniqueness
	if err := cs.checkExportName(exp.Name.Name); err != nil {
		return err
	}

	// Validate that the entity type is valid for use as an export
	if err := v.validateAndRegisterNamedTypes(cs, et, "export"); err != nil {
		return err
	}

	// Validate bracket-prefixed names
	if err := v.validateBracketName(cs, exp.Name.Name, et, "export"); err != nil {
		return err
	}

	// If this is a type export with a resource, register the resource name.
	// A resource may be exported under multiple names; all are valid.
	if et.Kind == EntityType && et.TypeRef.Kind == AnyTypeResource {
		addResourceName(v.arena, cs.resourceNames, et.TypeRef.ResID, exp.Name.Name)
		addResourceName(v.arena, cs.exportResourceNamesMap, et.TypeRef.ResID, exp.Name.Name)
	}

	// Add entities to their respective index spaces
	if err := cs.addEntity(et); err != nil {
		return err
	}
	cs.exports[exp.Name.Name] = et
	cs.exportOrder = append(cs.exportOrder, exp.Name.Name)
	return nil
}

// validateBracketName validates semantic constraints for bracket-prefixed names
// ([constructor]X, [method]X.Y, [static]X.Y) against the entity type.
func (v *Validator) validateBracketName(cs *ComponentState, name string, et ComponentEntityType, kind string) error {
	if strings.HasPrefix(name, "[constructor]") {
		resourceName := name[len("[constructor]"):]
		if et.Kind != EntityFunc {
			return fmt.Errorf("%s name `%s` is not valid: %s is not a func", kind, name, et.Desc())
		}
		ft := v.arena.FuncTypes[et.FuncID]
		// Constructor must return exactly one value
		if len(ft.Results) != 1 {
			return fmt.Errorf("function should return one value")
		}
		// The result must be (own $T) or (result (own $T))
		result := ft.Results[0].Type
		if !v.isOwnType(result) && !v.isResultOfOwn(result) {
			if kind == "import" {
				return fmt.Errorf("function should return `(own $T)` or `(result (own $T))`")
			}
			return fmt.Errorf("function should return `(own $T)`")
		}
		// Check function's resource matches the expected resource name
		if err := v.checkFuncResourceName(cs, resourceName, et.FuncID, kind); err != nil {
			return fmt.Errorf("%s name `%s` is not valid: %w", kind, name, err)
		}
		return nil
	}

	if strings.HasPrefix(name, "[method]") {
		rest := name[len("[method]"):]
		resourceName, _, ok := strings.Cut(rest, ".")
		if !ok {
			return fmt.Errorf("failed to find `.` character in method name")
		}
		if et.Kind != EntityFunc {
			return fmt.Errorf("%s name `%s` is not valid: %s is not a func", kind, name, et.Desc())
		}
		ft := v.arena.FuncTypes[et.FuncID]
		// Method must have at least one parameter
		if len(ft.Params) == 0 {
			return fmt.Errorf("function should have at least one argument")
		}
		// First parameter must be named "self"
		if ft.Params[0].Name != "self" {
			return fmt.Errorf("function should have a first argument called `self`")
		}
		// First parameter must be (borrow $T)
		if !v.isBorrowType(ft.Params[0].Type) {
			return fmt.Errorf("function should take a first argument of `(borrow $T)`")
		}
		// Check resource name matches
		if err := v.checkMethodResourceName(cs, resourceName, ft.Params[0].Type, kind); err != nil {
			return err
		}
		return nil
	}

	if strings.HasPrefix(name, "[static]") {
		rest := name[len("[static]"):]
		resourceName, _, ok := strings.Cut(rest, ".")
		if !ok {
			return fmt.Errorf("failed to find `.` character in static name")
		}
		if et.Kind != EntityFunc {
			return fmt.Errorf("%s name `%s` is not valid: %s is not a func", kind, name, et.Desc())
		}
		// Check resource name is known
		if !v.isKnownResourceName(cs, resourceName) {
			return fmt.Errorf("static resource name is not known in this context")
		}
		// Validate function type resources have names in this context
		if err := v.checkFuncResourcesHaveNames(cs, et.FuncID); err != nil {
			return err
		}
		return nil
	}

	// TODO: For non-bracket func imports/exports, check that the function doesn't
	// reference resources without names. This requires full resource name propagation
	// through aliases and instance exports.

	return nil
}

