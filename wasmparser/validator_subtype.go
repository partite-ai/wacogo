package wasmparser

import (
	"fmt"
	"maps"
	"slices"
)

// scopedResID identifies a resource together with enough context that
// numerically-equal IDs from different namespaces are not conflated.
// scope is a caller-supplied discriminator — typically a *InstanceType
// pointer for host-level checks, or nil for validator-internal checks
// where identity is defined by (arena, id) alone.
type scopedResID struct {
	scope any
	arena *TypeArena
	id    ResourceID
}

// SubtypeChecker performs subtype checking across two arenas.
//
// The checker compares an "a-side" (actual/argument) type tree rooted in
// aArena against a "b-side" (expected/import) type tree rooted in bArena.
// When both sides live in the same arena (the common case during
// validator-internal checks), aArena == bArena.
//
// A separate writeArena is used by remap*: helpers that construct new
// type entries (for example when instantiating a component produces a
// new instance type with fresh resource identities). remap* reads from
// aArena and writes into writeArena. For callers that only perform
// read-only comparisons, writeArena can be left nil.
type SubtypeChecker struct {
	aArena     *TypeArena
	bArena     *TypeArena
	writeArena *TypeArena

	// aScope is an optional discriminator that further scopes a-side
	// resource identities beyond aArena — for host-level checks where
	// multiple distinct instances share an arena, aScope is set to the
	// *InstanceType pointer so that fresh-per-instance resource semantics
	// are honored without any per-instance arena allocation.
	aScope any

	// bScope is aScope for the b-side. It is set only while comparing a
	// component's imports, where the two sides swap.
	bScope any

	// resourceMapping maps b-side ResourceIDs to the (scope, arena,
	// ResourceID) they have been matched against.
	resourceMapping map[ResourceID]scopedResID

	// aBound and bBound hold what the enclosing component types' own resources
	// were bound to, by their defining IDs. See isComponentSubtype.
	aBound map[ResourceID]scopedResID
	bBound map[ResourceID]scopedResID

	// definedTypeRemap memoizes remapDefinedType so multiple references to the
	// same source defined type within one remap session resolve to the same
	// destination ID. Without this two field references to the same record
	// produce two structurally-identical-but-distinct arena entries, which
	// breaks identity checks in the validator's named-type tracking.
	definedTypeRemap  map[ComponentDefinedTypeID]ComponentDefinedTypeID
	funcTypeRemap     map[ComponentFuncTypeID]ComponentFuncTypeID
	instanceTypeRemap map[ComponentInstanceTypeID]ComponentInstanceTypeID

	// definedTypeMapping records subtype matches: when isDefinedTypeSubtype
	// successfully matches a-side `a` against b-side `b`, mapping[b] = a.
	// remapDefinedType consults this when rewriting an instance's exports
	// after instantiation so references to the inner's import-bound types
	// flow through to the outer's arg types — wasm-tools' substitution.
	definedTypeMapping map[ComponentDefinedTypeID]ComponentDefinedTypeID


	// hostBinding relaxes function subtyping to match how host imports
	// are linked: parameter names are not compared, only count and
	// types. Validator-internal checks leave this false and keep the
	// strict structural equality the spec tests assert. CheckInstantiation
	// sets it true because host-provided funcs are registered by Rust-
	// style closures with no intrinsic param names — the consumer's
	// declared names are purely the binding contract.
	hostBinding bool
}

func (v *Validator) newSubtypeCheckerWithRemapping() *SubtypeChecker {
	return &SubtypeChecker{
		aArena:             v.arena,
		bArena:             v.arena,
		writeArena:         v.arena,
		resourceMapping:    make(map[ResourceID]scopedResID),
		definedTypeRemap:   make(map[ComponentDefinedTypeID]ComponentDefinedTypeID),
		funcTypeRemap:      make(map[ComponentFuncTypeID]ComponentFuncTypeID),
		instanceTypeRemap:  make(map[ComponentInstanceTypeID]ComponentInstanceTypeID),
		definedTypeMapping: make(map[ComponentDefinedTypeID]ComponentDefinedTypeID),
	}
}

// newSubtypeCheckerOnArena constructs a single-arena SubtypeChecker without a
// live Validator. Used by validator-internal code paths that still need
// remap* (for staging the result of an instantiation).
func newSubtypeCheckerOnArena(arena *TypeArena) *SubtypeChecker {
	return &SubtypeChecker{
		aArena:             arena,
		bArena:             arena,
		writeArena:         arena,
		resourceMapping:    make(map[ResourceID]scopedResID),
		definedTypeRemap:   make(map[ComponentDefinedTypeID]ComponentDefinedTypeID),
		funcTypeRemap:      make(map[ComponentFuncTypeID]ComponentFuncTypeID),
		instanceTypeRemap:  make(map[ComponentInstanceTypeID]ComponentInstanceTypeID),
		definedTypeMapping: make(map[ComponentDefinedTypeID]ComponentDefinedTypeID),
	}
}

// isSubtype checks whether entity type `a` (in aArena) is a subtype of
// entity type `b` (in bArena). Returns nil if a is a subtype of b.
func (sc *SubtypeChecker) isSubtype(a, b ComponentEntityType) error {
	if a.Kind != b.Kind {
		return fmt.Errorf("expected %s found %s (expected %s, found %s)",
			entityExpectedName(b.Kind), entityFoundName(a.Kind),
			b.Desc(), a.Desc())
	}
	switch a.Kind {
	case EntityModule:
		return sc.isModuleSubtype(a.ModuleID, b.ModuleID)
	case EntityFunc:
		return sc.isFuncTypeSubtype(a.FuncID, b.FuncID)
	case EntityValue:
		return sc.isValTypeSubtype(a.ValType, b.ValType)
	case EntityType:
		return sc.isTypeSubtype(a.TypeRef, b.TypeRef)
	case EntityInstance:
		return sc.isInstanceSubtype(a.InstID, b.InstID)
	case EntityComponent:
		return sc.isComponentSubtype(a.CompID, b.CompID)
	}
	return nil
}

// isModuleSubtype checks that module type `a` is a subtype of module type `b`.
func (sc *SubtypeChecker) isModuleSubtype(a, b CoreModuleTypeID) error {
	at := sc.aArena.CoreModuleTypes[a]
	bt := sc.bArena.CoreModuleTypes[b]

	for key := range at.Imports {
		bImport, ok := bt.Imports[key]
		if !ok {
			return fmt.Errorf("missing expected import `::%[2]s`: module import `%[1]s::%[2]s` not defined", key.Module, key.Name)
		}
		aImport := at.Imports[key]
		if err := checkCoreEntityTypeMatch(sc.aArena, sc.bArena, aImport, bImport); err != nil {
			return fmt.Errorf("type mismatch in import `::%[2]s`: module import `%[1]s::%[2]s` has the wrong type: %w", key.Module, key.Name, err)
		}
	}

	for name, bExport := range bt.Exports {
		aExport, ok := at.Exports[name]
		if !ok {
			return fmt.Errorf("missing expected export `%[1]s`: module export `%[1]s` not defined", name)
		}
		if err := checkCoreEntityTypeMatch(sc.bArena, sc.aArena, bExport, aExport); err != nil {
			return fmt.Errorf("type mismatch in export `%[1]s`: export `%[1]s` has the wrong type: %w", name, err)
		}
	}

	return nil
}

// isFuncTypeSubtype checks that func type `a` is a subtype of func type `b`.
func (sc *SubtypeChecker) isFuncTypeSubtype(a, b ComponentFuncTypeID) error {
	at := sc.aArena.FuncTypes[a]
	bt := sc.bArena.FuncTypes[b]

	if len(at.Params) != len(bt.Params) {
		return fmt.Errorf("expected %d parameters, found %d", len(bt.Params), len(at.Params))
	}
	for i := range bt.Params {
		if !sc.hostBinding && at.Params[i].Name != bt.Params[i].Name {
			return fmt.Errorf("expected parameter named `%s`, found `%s`", bt.Params[i].Name, at.Params[i].Name)
		}
		if err := sc.isValTypeSubtype(at.Params[i].Type, bt.Params[i].Type); err != nil {
			return fmt.Errorf("type mismatch in function parameter `%s`: %w", bt.Params[i].Name, err)
		}
	}

	if len(at.Results) != len(bt.Results) {
		if len(at.Results) > 0 && len(bt.Results) == 0 {
			return fmt.Errorf("expected a result, found none")
		}
		if len(at.Results) == 0 && len(bt.Results) > 0 {
			return fmt.Errorf("expected no results, found %d", len(bt.Results))
		}
		return fmt.Errorf("expected %d results, found %d", len(bt.Results), len(at.Results))
	}
	for i := range bt.Results {
		if err := sc.isValTypeSubtype(at.Results[i].Type, bt.Results[i].Type); err != nil {
			return fmt.Errorf("type mismatch with result type: %w", err)
		}
	}

	return nil
}

// isValTypeSubtype checks that val type `a` is a subtype of val type `b`.
//
// A primitive may be encoded directly (IsPrimitive=true) or via a
// DefinedKindPrimitive alias in the arena. The two encodings are
// equivalent for subtype purposes: this function normalizes by
// unwrapping any DefinedKindPrimitive entries before comparison.
func (sc *SubtypeChecker) isValTypeSubtype(a, b ValTypeDesc) error {
	aIsPrim, aPrim := unwrapPrimitive(sc.aArena, a)
	bIsPrim, bPrim := unwrapPrimitive(sc.bArena, b)
	if aIsPrim && bIsPrim {
		if aPrim != bPrim {
			return fmt.Errorf("expected primitive `%s` found primitive `%s`", primitiveName(bPrim), primitiveName(aPrim))
		}
		return nil
	}
	if aIsPrim && !bIsPrim {
		bdt := sc.bArena.DefinedTypes[b.TypeID]
		return fmt.Errorf("expected %s, found %s", definedTypeKindName(bdt.Kind), primitiveName(aPrim))
	}
	if !aIsPrim && bIsPrim {
		adt := sc.aArena.DefinedTypes[a.TypeID]
		return fmt.Errorf("expected %s, found %s", primitiveName(bPrim), definedTypeKindName(adt.Kind))
	}
	return sc.isDefinedTypeSubtype(a.TypeID, b.TypeID)
}

// unwrapPrimitive flattens a ValTypeDesc to (true, prim) when it is
// either a direct primitive or a DefinedKindPrimitive alias in arena.
// Returns (false, _) for any other defined-type kind.
func unwrapPrimitive(arena *TypeArena, vt ValTypeDesc) (bool, PrimitiveValType) {
	if vt.IsPrimitive {
		return true, vt.Primitive
	}
	dt := arena.DefinedTypes[vt.TypeID]
	if dt.Kind == DefinedKindPrimitive {
		return true, dt.Primitive
	}
	return false, 0
}

// isDefinedTypeSubtype checks that defined type `a` is a subtype of defined type `b`.
// On success it records the b→a binding in definedTypeMapping so a subsequent
// remap session can substitute the import-side ID for the arg-side ID — this
// is what makes instance-type exports referencing inner imports flow through
// to the caller's named-type set.
func (sc *SubtypeChecker) isDefinedTypeSubtype(a, b ComponentDefinedTypeID) error {
	if err := sc.isDefinedTypeSubtypeImpl(a, b); err != nil {
		return err
	}
	if sc.definedTypeMapping != nil && sc.aArena == sc.bArena {
		sc.definedTypeMapping[b] = a
	}
	return nil
}

func (sc *SubtypeChecker) isDefinedTypeSubtypeImpl(a, b ComponentDefinedTypeID) error {
	at := sc.aArena.DefinedTypes[a]
	bt := sc.bArena.DefinedTypes[b]

	if at.Kind != bt.Kind {
		aName := definedTypeKindName(at.Kind)
		bName := definedTypeKindName(bt.Kind)
		if at.Kind == DefinedKindPrimitive {
			aName = primitiveName(at.Primitive)
		}
		return fmt.Errorf("expected %s, found %s", bName, aName)
	}

	switch at.Kind {
	case DefinedKindRecord:
		return sc.isRecordSubtype(at.Record, bt.Record)
	case DefinedKindVariant:
		return sc.isVariantSubtype(at.Variant, bt.Variant)
	case DefinedKindList:
		return sc.isValTypeSubtype(at.List, bt.List)
	case DefinedKindTuple:
		return sc.isTupleSubtype(at.Tuple, bt.Tuple)
	case DefinedKindFlags:
		return isFlagsSubtype(at.Flags, bt.Flags)
	case DefinedKindEnum:
		return isEnumSubtype(at.Enum, bt.Enum)
	case DefinedKindOption:
		return sc.isValTypeSubtype(at.Option, bt.Option)
	case DefinedKindResult:
		return sc.isResultSubtype(at.ResultOk, at.ResultErr, bt.ResultOk, bt.ResultErr)
	case DefinedKindOwn:
		return sc.checkResourceMatch(at.Own, bt.Own)
	case DefinedKindBorrow:
		return sc.checkResourceMatch(at.Borrow, bt.Borrow)
	}
	return nil
}

// checkResourceMatch checks that the a-side resource (ResourceID aRes in
// aArena) is compatible with the b-side resource (ResourceID bRes in
// bArena). When resource remapping is enabled, it records matches so
// repeated references to the same b-side resource require consistent
// a-side matches — the mechanism behind (eq $r) constraints.
//
// If the a-side scope records that aRes is defined elsewhere (see
// InstanceType.resourceRoots), the comparison uses that root. This lets
// resources shared across distinct supplied instances pass identity checks
// without weakening per-instance freshness for locally defined resources.
func (sc *SubtypeChecker) checkResourceMatch(aRes, bRes ResourceID) error {
	a := resolveResource(sc.aScope, sc.aArena, aRes, sc.aBound)

	if sc.resourceMapping != nil {
		bRes = sc.bArena.ResolveResourceAlias(bRes)
		if existing, ok := sc.resourceMapping[bRes]; ok {
			if existing != a {
				return sc.resourceMismatchErr()
			}
			return nil
		}
		sc.resourceMapping[bRes] = a
		return nil
	}
	if a != resolveResource(sc.bScope, sc.bArena, bRes, sc.bBound) {
		return sc.resourceMismatchErr()
	}
	return nil
}

// resolveResource returns the identity of a resource: what it was bound to if
// bound holds it, and otherwise its defining resource, or the root recorded for
// that on an *InstanceType scope if it is defined elsewhere.
func resolveResource(scope any, arena *TypeArena, res ResourceID, bound map[ResourceID]scopedResID) scopedResID {
	// A resource may be known by several IDs, each an alias of its defining ID.
	// Aliases live globally on the arena.
	res = arena.ResolveResourceAlias(res)
	if b, ok := bound[res]; ok {
		return b
	}
	if it, ok := scope.(*InstanceType); ok {
		if root, ok := it.resourceRoots[res]; ok {
			return root
		}
	}
	return scopedResID{scope: scope, arena: arena, id: res}
}

// resourceMismatchErr selects the spec-aligned wording for a rejected
// resource match: host-binding paths (CheckInstantiation) report
// "mismatched resource types" per the wasmtime spec suite, while
// validator-internal paths report "resource types are not the same"
// per the wasm-tools spec suite.
func (sc *SubtypeChecker) resourceMismatchErr() error {
	if sc.hostBinding {
		return fmt.Errorf("mismatched resource types")
	}
	return fmt.Errorf("resource types are not the same")
}

func (sc *SubtypeChecker) isRecordSubtype(a, b *RecordTypeDesc) error {
	if a == nil && b == nil {
		return nil
	}
	if a == nil || b == nil {
		return fmt.Errorf("record type mismatch")
	}
	if len(a.Fields) != len(b.Fields) {
		return fmt.Errorf("expected %d fields, found %d", len(b.Fields), len(a.Fields))
	}
	for i := range b.Fields {
		if a.Fields[i].Name != b.Fields[i].Name {
			return fmt.Errorf("expected field name `%s`, found `%s`", b.Fields[i].Name, a.Fields[i].Name)
		}
		if err := sc.isValTypeSubtype(a.Fields[i].Type, b.Fields[i].Type); err != nil {
			return fmt.Errorf("type mismatch in record field `%s`: %w", b.Fields[i].Name, err)
		}
	}
	return nil
}

func (sc *SubtypeChecker) isVariantSubtype(a, b *VariantTypeDesc) error {
	if a == nil && b == nil {
		return nil
	}
	if a == nil || b == nil {
		return fmt.Errorf("variant type mismatch")
	}
	if len(a.Cases) != len(b.Cases) {
		return fmt.Errorf("expected %d cases, found %d", len(b.Cases), len(a.Cases))
	}
	for i := range b.Cases {
		if a.Cases[i].Name != b.Cases[i].Name {
			return fmt.Errorf("expected case named `%s`, found `%s`", b.Cases[i].Name, a.Cases[i].Name)
		}
		if b.Cases[i].Type.Valid && !a.Cases[i].Type.Valid {
			return fmt.Errorf("expected case `%s` to have a type, found none", b.Cases[i].Name)
		}
		if !b.Cases[i].Type.Valid && a.Cases[i].Type.Valid {
			return fmt.Errorf("expected case `%s` to have no type", b.Cases[i].Name)
		}
		if a.Cases[i].Type.Valid && b.Cases[i].Type.Valid {
			if err := sc.isValTypeSubtype(a.Cases[i].Type.Value, b.Cases[i].Type.Value); err != nil {
				return fmt.Errorf("type mismatch in variant case `%s`: %w", b.Cases[i].Name, err)
			}
		}
	}
	return nil
}

func (sc *SubtypeChecker) isTupleSubtype(a, b []ValTypeDesc) error {
	if len(a) != len(b) {
		return fmt.Errorf("expected %d types, found %d", len(b), len(a))
	}
	for i := range b {
		if err := sc.isValTypeSubtype(a[i], b[i]); err != nil {
			return fmt.Errorf("type mismatch in tuple field %d: %w", i, err)
		}
	}
	return nil
}

func isFlagsSubtype(a, b []string) error {
	if len(a) != len(b) {
		return fmt.Errorf("mismatch in flags elements")
	}
	for i := range b {
		if a[i] != b[i] {
			return fmt.Errorf("mismatch in flags elements")
		}
	}
	return nil
}

func isEnumSubtype(a, b []string) error {
	if len(a) != len(b) {
		return fmt.Errorf("mismatch in enum elements")
	}
	for i := range b {
		if a[i] != b[i] {
			return fmt.Errorf("mismatch in enum elements")
		}
	}
	return nil
}

func (sc *SubtypeChecker) isResultSubtype(aOk, aErr, bOk, bErr Optional[ValTypeDesc]) error {
	if bOk.Valid && !aOk.Valid {
		return fmt.Errorf("expected ok type, but found none")
	}
	if !bOk.Valid && aOk.Valid {
		return fmt.Errorf("expected ok type to not be present")
	}
	if aOk.Valid && bOk.Valid {
		if err := sc.isValTypeSubtype(aOk.Value, bOk.Value); err != nil {
			return fmt.Errorf("type mismatch in ok variant: %w", err)
		}
	}

	if bErr.Valid && !aErr.Valid {
		return fmt.Errorf("expected err type, but found none")
	}
	if !bErr.Valid && aErr.Valid {
		return fmt.Errorf("expected err type to not be present")
	}
	if aErr.Valid && bErr.Valid {
		if err := sc.isValTypeSubtype(aErr.Value, bErr.Value); err != nil {
			return fmt.Errorf("type mismatch in err variant: %w", err)
		}
	}

	return nil
}

// isInstanceSubtype checks that instance type `a` is a subtype of instance type `b`.
func (sc *SubtypeChecker) isInstanceSubtype(a, b ComponentInstanceTypeID) error {
	at := sc.aArena.InstanceTypes[a]
	bt := sc.bArena.InstanceTypes[b]

	for name, bExport := range bt.Exports {
		aExport, ok := at.Exports[name]
		if !ok {
			return fmt.Errorf("missing expected export `%s`", name)
		}
		if err := sc.isSubtype(aExport, bExport); err != nil {
			return fmt.Errorf("type mismatch in instance export `%s`: %w", name, err)
		}
	}
	return nil
}

// isComponentSubtype checks that component type `a` is a subtype of component type `b`:
// that an `a` can be used wherever a `b` is expected. It may import less and export
// more, and each import of `b` must satisfy the import of `a` it is given to.
//
// Each type's own resources stand for whatever its user supplies. So the resources
// `a` imports are bound to the matching resources `b` imports, and those `b` exports
// to the matching resources `a` exports. All other resources compare by identity.
func (sc *SubtypeChecker) isComponentSubtype(a, b ComponentTypeID) error {
	at := sc.aArena.ComponentTypes[a]
	bt := sc.bArena.ComponentTypes[b]

	inner := &SubtypeChecker{
		aArena:      sc.aArena,
		bArena:      sc.bArena,
		aScope:      sc.aScope,
		bScope:      sc.bScope,
		hostBinding: sc.hostBinding,
		aBound:      maps.Clone(sc.aBound),
		bBound:      maps.Clone(sc.bBound),
	}
	if inner.aBound == nil {
		inner.aBound = make(map[ResourceID]scopedResID)
	}
	if inner.bBound == nil {
		inner.bBound = make(map[ResourceID]scopedResID)
	}
	maps.Copy(inner.bBound, sc.resourceMapping)
	swapped := &SubtypeChecker{
		aArena:      inner.bArena,
		bArena:      inner.aArena,
		aScope:      inner.bScope,
		bScope:      inner.aScope,
		hostBinding: inner.hostBinding,
		aBound:      inner.bBound,
		bBound:      inner.aBound,
	}

	// A resource is named again by each (eq) referring to it, under the same ID, so
	// each is bound once, where the type introduces it.
	open := make(map[ResourceID]bool)
	for _, res := range at.ImportedResources {
		open[res] = true
	}
	for _, name := range at.ImportOrder {
		bImport, ok := bt.Imports[name]
		if !ok {
			return fmt.Errorf("missing import named `%s`", name)
		}
		swapped.bindOpenResources(bImport, at.Imports[name], open)
	}
	for _, name := range at.ImportOrder {
		if err := swapped.isSubtype(bt.Imports[name], at.Imports[name]); err != nil {
			return fmt.Errorf("type mismatch in import `%s`: %w", name, err)
		}
	}

	clear(open)
	for _, res := range bt.DefinedResources {
		open[res] = true
	}
	for _, name := range bt.ExportOrder {
		aExport, ok := at.Exports[name]
		if !ok {
			return fmt.Errorf("export `%s` was not found", name)
		}
		inner.bindOpenResources(aExport, bt.Exports[name], open)
	}
	for _, name := range bt.ExportOrder {
		if err := inner.isSubtype(at.Exports[name], bt.Exports[name]); err != nil {
			return fmt.Errorf("type mismatch in export `%s`: %w", name, err)
		}
	}

	return nil
}

// bindOpenResources binds the resources the b-side type introduces to the a-side
// resources at the same place: a directly named one if it is still in open, which it
// is then removed from, and those of an instance. Mismatched shapes bind nothing, and
// are left for the subtype check to report.
func (sc *SubtypeChecker) bindOpenResources(a, b ComponentEntityType, open map[ResourceID]bool) {
	switch {
	case a.Kind == EntityType && b.Kind == EntityType:
		if a.TypeRef.Kind == AnyTypeResource && b.TypeRef.Kind == AnyTypeResource && open[b.TypeRef.ResID] {
			delete(open, b.TypeRef.ResID)
			sc.bBound[b.TypeRef.ResID] = resolveResource(sc.aScope, sc.aArena, a.TypeRef.ResID, sc.aBound)
		}
	case a.Kind == EntityInstance && b.Kind == EntityInstance:
		aExports := sc.aArena.InstanceTypes[a.InstID].Exports
		bt := sc.bArena.InstanceTypes[b.InstID]
		for _, res := range bt.DefinedResources {
			if aExport, ok := aExports[res.Export]; ok && aExport.Kind == EntityType && aExport.TypeRef.Kind == AnyTypeResource {
				sc.bBound[res.ID] = resolveResource(sc.aScope, sc.aArena, aExport.TypeRef.ResID, sc.aBound)
			}
		}
		for name, bExport := range bt.Exports {
			if aExport, ok := aExports[name]; ok && aExport.Kind == EntityInstance && bExport.Kind == EntityInstance {
				sc.bindOpenResources(aExport, bExport, nil)
			}
		}
	}
}

// isTypeSubtype checks that type ref `a` is a subtype of type ref `b`.
func (sc *SubtypeChecker) isTypeSubtype(a, b ComponentAnyTypeID) error {
	if a.Kind != b.Kind {
		switch {
		case b.Kind == AnyTypeResource:
			return fmt.Errorf("expected resource, found %s", a.Desc())
		case a.Kind == AnyTypeResource:
			return fmt.Errorf("expected %s, found resource", b.Desc())
		default:
			return fmt.Errorf("expected %s, found %s", b.Desc(), a.Desc())
		}
	}
	switch a.Kind {
	case AnyTypeDefined:
		return sc.isDefinedTypeSubtype(ComponentDefinedTypeID(a.Index), ComponentDefinedTypeID(b.Index))
	case AnyTypeFunc:
		return sc.isFuncTypeSubtype(ComponentFuncTypeID(a.Index), ComponentFuncTypeID(b.Index))
	case AnyTypeInstance:
		return sc.isInstanceSubtype(ComponentInstanceTypeID(a.Index), ComponentInstanceTypeID(b.Index))
	case AnyTypeComponent:
		return sc.isComponentSubtype(ComponentTypeID(a.Index), ComponentTypeID(b.Index))
	case AnyTypeResource:
		return sc.checkResourceMatch(a.ResID, b.ResID)
	}
	return nil
}

// remapEntityType applies resourceMapping and definedTypeMapping to an entity
// type: whatever they name is replaced, everything else keeps its identity, and a
// type is copied into writeArena only if something in it was replaced. The caller
// decides what is replaced; nothing here decides that a resource is new.
func (sc *SubtypeChecker) remapEntityType(et ComponentEntityType) ComponentEntityType {
	if sc.resourceMapping == nil {
		return et
	}
	switch et.Kind {
	case EntityType:
		return ComponentEntityType{
			Kind:    EntityType,
			TypeRef: sc.remapAnyTypeID(et.TypeRef),
		}
	case EntityFunc:
		return ComponentEntityType{
			Kind:   EntityFunc,
			FuncID: sc.remapFuncType(et.FuncID),
		}
	case EntityInstance:
		return ComponentEntityType{
			Kind:   EntityInstance,
			InstID: sc.remapInstanceType(et.InstID),
		}
	case EntityValue:
		return ComponentEntityType{
			Kind:    EntityValue,
			ValType: sc.remapValType(et.ValType),
		}
	case EntityComponent:
		return ComponentEntityType{
			Kind:   EntityComponent,
			CompID: sc.remapComponentType(et.CompID),
		}
	default:
		return et
	}
}

// remapResource returns what a resource reference becomes. An export alias of a
// replaced resource becomes a new alias of its replacement, so that re-exports
// still resolve to it.
func (sc *SubtypeChecker) remapResource(id ResourceID) ResourceID {
	if mapped, ok := sc.resourceMapping[id]; ok {
		return mapped.id
	}
	mapped, ok := sc.resourceMapping[sc.aArena.ResolveResourceAlias(id)]
	if !ok {
		return id
	}
	alias := sc.writeArena.allocResourceID()
	if sc.writeArena.resourceAliases == nil {
		sc.writeArena.resourceAliases = make(map[ResourceID]ResourceID)
	}
	sc.writeArena.resourceAliases[alias] = sc.writeArena.ResolveResourceAlias(mapped.id)
	sc.resourceMapping[id] = scopedResID{arena: sc.writeArena, id: alias}
	return alias
}

func (sc *SubtypeChecker) remapAnyTypeID(id ComponentAnyTypeID) ComponentAnyTypeID {
	switch id.Kind {
	case AnyTypeResource:
		return ComponentAnyTypeID{Kind: AnyTypeResource, ResID: sc.remapResource(id.ResID)}
	case AnyTypeDefined:
		newID := sc.remapDefinedType(ComponentDefinedTypeID(id.Index))
		return ComponentAnyTypeID{Kind: AnyTypeDefined, Index: uint32(newID)}
	case AnyTypeFunc:
		newID := sc.remapFuncType(ComponentFuncTypeID(id.Index))
		return ComponentAnyTypeID{Kind: AnyTypeFunc, Index: uint32(newID)}
	case AnyTypeInstance:
		newID := sc.remapInstanceType(ComponentInstanceTypeID(id.Index))
		return ComponentAnyTypeID{Kind: AnyTypeInstance, Index: uint32(newID)}
	case AnyTypeComponent:
		newID := sc.remapComponentType(ComponentTypeID(id.Index))
		return ComponentAnyTypeID{Kind: AnyTypeComponent, Index: uint32(newID)}
	}
	return id
}

func (sc *SubtypeChecker) remapValType(vt ValTypeDesc) ValTypeDesc {
	if vt.IsPrimitive {
		return vt
	}
	newID := sc.remapDefinedType(vt.TypeID)
	return ValTypeDesc{TypeID: newID}
}

func (sc *SubtypeChecker) remapDefinedType(id ComponentDefinedTypeID) ComponentDefinedTypeID {
	if sc.definedTypeRemap != nil {
		if cached, ok := sc.definedTypeRemap[id]; ok {
			return cached
		}
	}
	// A direct binding from isDefinedTypeSubtype takes precedence: this
	// happens when the inner component declared a type (typically an
	// import-bound) whose identity should be replaced wholesale by the
	// outer's arg.
	if sc.definedTypeMapping != nil {
		if mapped, ok := sc.definedTypeMapping[id]; ok {
			if sc.definedTypeRemap != nil {
				sc.definedTypeRemap[id] = mapped
			}
			return mapped
		}
	}
	dt := sc.aArena.DefinedTypes[id]
	if !sc.definedTypeChanges(&dt) && sc.aArena == sc.writeArena {
		if sc.definedTypeRemap != nil {
			sc.definedTypeRemap[id] = id
		}
		return id
	}

	newDT := dt // copy
	switch dt.Kind {
	case DefinedKindOwn:
		newDT.Own = sc.remapResource(dt.Own)
	case DefinedKindBorrow:
		newDT.Borrow = sc.remapResource(dt.Borrow)
	case DefinedKindRecord:
		if dt.Record != nil {
			fields := make([]FieldDesc, len(dt.Record.Fields))
			for i, f := range dt.Record.Fields {
				fields[i] = FieldDesc{Name: f.Name, Type: sc.remapValType(f.Type)}
			}
			newDT.Record = &RecordTypeDesc{Fields: fields}
		}
	case DefinedKindVariant:
		if dt.Variant != nil {
			cases := make([]VariantCaseDesc, len(dt.Variant.Cases))
			for i, c := range dt.Variant.Cases {
				cases[i] = c
				if c.Type.Valid {
					cases[i].Type = Some(sc.remapValType(c.Type.Value))
				}
			}
			newDT.Variant = &VariantTypeDesc{Cases: cases}
		}
	case DefinedKindList:
		newDT.List = sc.remapValType(dt.List)
	case DefinedKindTuple:
		tuple := make([]ValTypeDesc, len(dt.Tuple))
		for i, t := range dt.Tuple {
			tuple[i] = sc.remapValType(t)
		}
		newDT.Tuple = tuple
	case DefinedKindOption:
		newDT.Option = sc.remapValType(dt.Option)
	case DefinedKindResult:
		if dt.ResultOk.Valid {
			newDT.ResultOk = Some(sc.remapValType(dt.ResultOk.Value))
		}
		if dt.ResultErr.Valid {
			newDT.ResultErr = Some(sc.remapValType(dt.ResultErr.Value))
		}
	}
	newID := sc.writeArena.pushDefinedType(newDT)
	if sc.definedTypeRemap != nil {
		sc.definedTypeRemap[id] = newID
	}
	return newID
}

func (sc *SubtypeChecker) resourceChanges(id ResourceID) bool {
	if _, ok := sc.resourceMapping[id]; ok {
		return true
	}
	_, ok := sc.resourceMapping[sc.aArena.ResolveResourceAlias(id)]
	return ok
}

func (sc *SubtypeChecker) valTypeChanges(vt ValTypeDesc) bool {
	if vt.IsPrimitive {
		return false
	}
	if _, ok := sc.definedTypeMapping[vt.TypeID]; ok {
		return true
	}
	return sc.definedTypeChanges(&sc.aArena.DefinedTypes[vt.TypeID])
}

// definedTypeChanges reports whether remapping would replace anything in dt.
func (sc *SubtypeChecker) definedTypeChanges(dt *DefinedTypeDesc) bool {
	switch dt.Kind {
	case DefinedKindOwn:
		return sc.resourceChanges(dt.Own)
	case DefinedKindBorrow:
		return sc.resourceChanges(dt.Borrow)
	case DefinedKindRecord:
		if dt.Record != nil {
			for _, f := range dt.Record.Fields {
				if sc.valTypeChanges(f.Type) {
					return true
				}
			}
		}
	case DefinedKindVariant:
		if dt.Variant != nil {
			for _, c := range dt.Variant.Cases {
				if c.Type.Valid && sc.valTypeChanges(c.Type.Value) {
					return true
				}
			}
		}
	case DefinedKindList:
		return sc.valTypeChanges(dt.List)
	case DefinedKindTuple:
		return slices.ContainsFunc(dt.Tuple, sc.valTypeChanges)
	case DefinedKindOption:
		return sc.valTypeChanges(dt.Option)
	case DefinedKindResult:
		return (dt.ResultOk.Valid && sc.valTypeChanges(dt.ResultOk.Value)) ||
			(dt.ResultErr.Valid && sc.valTypeChanges(dt.ResultErr.Value))
	}
	return false
}

func (sc *SubtypeChecker) funcTypeChanges(id ComponentFuncTypeID) bool {
	ft := sc.aArena.FuncTypes[id]
	changes := func(p FuncParamDesc) bool { return sc.valTypeChanges(p.Type) }
	return slices.ContainsFunc(ft.Params, changes) || slices.ContainsFunc(ft.Results, changes)
}

func (sc *SubtypeChecker) entitiesChange(entities map[string]ComponentEntityType) bool {
	for _, et := range entities {
		if sc.entityTypeChanges(et) {
			return true
		}
	}
	return false
}

func (sc *SubtypeChecker) instanceTypeChanges(id ComponentInstanceTypeID) bool {
	return sc.entitiesChange(sc.aArena.InstanceTypes[id].Exports)
}

func (sc *SubtypeChecker) componentTypeChanges(id ComponentTypeID) bool {
	ct := sc.aArena.ComponentTypes[id]
	return sc.entitiesChange(ct.Imports) || sc.entitiesChange(ct.Exports)
}

func (sc *SubtypeChecker) entityTypeChanges(et ComponentEntityType) bool {
	switch et.Kind {
	case EntityType:
		switch et.TypeRef.Kind {
		case AnyTypeResource:
			return sc.resourceChanges(et.TypeRef.ResID)
		case AnyTypeDefined:
			id := ComponentDefinedTypeID(et.TypeRef.Index)
			if _, ok := sc.definedTypeMapping[id]; ok {
				return true
			}
			return sc.definedTypeChanges(&sc.aArena.DefinedTypes[id])
		case AnyTypeFunc:
			return sc.funcTypeChanges(ComponentFuncTypeID(et.TypeRef.Index))
		case AnyTypeInstance:
			return sc.instanceTypeChanges(ComponentInstanceTypeID(et.TypeRef.Index))
		case AnyTypeComponent:
			return sc.componentTypeChanges(ComponentTypeID(et.TypeRef.Index))
		}
	case EntityFunc:
		return sc.funcTypeChanges(et.FuncID)
	case EntityInstance:
		return sc.instanceTypeChanges(et.InstID)
	case EntityComponent:
		return sc.componentTypeChanges(et.CompID)
	case EntityValue:
		return sc.valTypeChanges(et.ValType)
	}
	return false
}

func (sc *SubtypeChecker) remapFuncType(id ComponentFuncTypeID) ComponentFuncTypeID {
	if sc.funcTypeRemap != nil {
		if cached, ok := sc.funcTypeRemap[id]; ok {
			return cached
		}
	}
	ft := sc.aArena.FuncTypes[id]
	if !sc.funcTypeChanges(id) && sc.aArena == sc.writeArena {
		if sc.funcTypeRemap != nil {
			sc.funcTypeRemap[id] = id
		}
		return id
	}

	params := make([]FuncParamDesc, len(ft.Params))
	for i, p := range ft.Params {
		params[i] = FuncParamDesc{Name: p.Name, Type: sc.remapValType(p.Type)}
	}
	results := make([]FuncParamDesc, len(ft.Results))
	for i, r := range ft.Results {
		results[i] = FuncParamDesc{Name: r.Name, Type: sc.remapValType(r.Type)}
	}
	newID := sc.writeArena.pushFuncType(FuncTypeDesc{Params: params, Results: results})
	if sc.funcTypeRemap != nil {
		sc.funcTypeRemap[id] = newID
	}
	return newID
}

func (sc *SubtypeChecker) remapInstanceType(id ComponentInstanceTypeID) ComponentInstanceTypeID {
	if sc.instanceTypeRemap != nil {
		if cached, ok := sc.instanceTypeRemap[id]; ok {
			return cached
		}
	}
	it := sc.aArena.InstanceTypes[id]
	if !sc.instanceTypeChanges(id) && sc.aArena == sc.writeArena {
		if sc.instanceTypeRemap != nil {
			sc.instanceTypeRemap[id] = id
		}
		return id
	}

	exports := make(map[string]ComponentEntityType)
	for name, et := range it.Exports {
		exports[name] = sc.remapEntityType(et)
	}
	var definedResources []DefinedResource
	if len(it.DefinedResources) > 0 {
		definedResources = make([]DefinedResource, len(it.DefinedResources))
		for i, res := range it.DefinedResources {
			definedResources[i] = res
			if mapped, ok := sc.resourceMapping[res.ID]; ok {
				definedResources[i].ID = mapped.id
			}
		}
	}
	newID := sc.writeArena.pushInstanceType(InstanceTypeDesc{
		Exports:          exports,
		DefinedResources: definedResources,
	})
	if sc.instanceTypeRemap != nil {
		sc.instanceTypeRemap[id] = newID
	}
	return newID
}

func (sc *SubtypeChecker) remapComponentType(id ComponentTypeID) ComponentTypeID {
	ct := sc.aArena.ComponentTypes[id]
	if !sc.componentTypeChanges(id) && sc.aArena == sc.writeArena {
		return id
	}

	imports := make(map[string]ComponentEntityType)
	for name, et := range ct.Imports {
		imports[name] = sc.remapEntityType(et)
	}
	exports := make(map[string]ComponentEntityType)
	for name, et := range ct.Exports {
		exports[name] = sc.remapEntityType(et)
	}
	remapped := ct
	remapped.Imports = imports
	remapped.Exports = exports
	remapped.ImportedResources = sc.remapResourceList(ct.ImportedResources)
	remapped.DefinedResources = sc.remapResourceList(ct.DefinedResources)
	return sc.writeArena.pushComponentType(remapped)
}

func (sc *SubtypeChecker) remapResourceList(ids []ResourceID) []ResourceID {
	out := slices.Clone(ids)
	for i, id := range out {
		if mapped, ok := sc.resourceMapping[id]; ok {
			out[i] = mapped.id
		}
	}
	return out
}

// primitiveName returns the string name for a primitive value type.
func primitiveName(p PrimitiveValType) string {
	switch p {
	case PrimBool:
		return "bool"
	case PrimS8:
		return "s8"
	case PrimU8:
		return "u8"
	case PrimS16:
		return "s16"
	case PrimU16:
		return "u16"
	case PrimS32:
		return "s32"
	case PrimU32:
		return "u32"
	case PrimS64:
		return "s64"
	case PrimU64:
		return "u64"
	case PrimF32:
		return "f32"
	case PrimF64:
		return "f64"
	case PrimChar:
		return "char"
	case PrimString:
		return "string"
	}
	return "unknown"
}

// definedTypeKindName returns a human-readable name for a defined type kind.
func definedTypeKindName(k ComponentDefinedTypeKind) string {
	switch k {
	case DefinedKindPrimitive:
		return "primitive"
	case DefinedKindRecord:
		return "record"
	case DefinedKindVariant:
		return "variant"
	case DefinedKindList:
		return "list"
	case DefinedKindTuple:
		return "tuple"
	case DefinedKindFlags:
		return "flags"
	case DefinedKindEnum:
		return "enum"
	case DefinedKindOption:
		return "option"
	case DefinedKindResult:
		return "result"
	case DefinedKindOwn:
		return "own"
	case DefinedKindBorrow:
		return "borrow"
	}
	return "unknown"
}
