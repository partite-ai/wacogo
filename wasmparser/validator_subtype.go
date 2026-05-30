package wasmparser

import (
	"fmt"
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

	// resourceMapping maps b-side ResourceIDs to the (scope, arena,
	// ResourceID) they have been matched against.
	resourceMapping map[ResourceID]scopedResID

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
// If the supplied a-side scope is an *InstanceType that re-exports
// aRes from another instance via aliasing (recorded on the
// InstanceType's resourceOrigin map), the chain is walked to the
// defining instance before scope comparison. This lets `use`-style
// resource sharing across distinct supplied instances pass identity
// checks without weakening per-instance freshness for locally defined
// resources.
func (sc *SubtypeChecker) checkResourceMatch(aRes, bRes ResourceID) error {
	aScope := sc.aScope
	aArena := sc.aArena
	for hops := 0; hops < 8; hops++ {
		it, ok := aScope.(*InstanceType)
		if !ok || it.resourceOrigin == nil {
			break
		}
		spec, ok := it.resourceOrigin[aRes]
		if !ok || spec.Lender == nil || spec.Lender == it {
			break
		}
		aScope = spec.Lender
		aRes = spec.LenderResID
		aArena = spec.Lender.source.arena
	}

	// One-hop alias resolution: if either side is an export-alias of a
	// defining resource, fall back to the defining ID. Two imports that both
	// reference (eq $r) where $r is itself an alias must still compare
	// equal under this check. Aliases live globally on the arena.
	aRes = aArena.resolveResourceAlias(aRes)
	bRes = sc.bArena.resolveResourceAlias(bRes)

	if sc.resourceMapping != nil {
		if existing, ok := sc.resourceMapping[bRes]; ok {
			if existing.scope != aScope || existing.arena != aArena || existing.id != aRes {
				return sc.resourceMismatchErr()
			}
			return nil
		}
		sc.resourceMapping[bRes] = scopedResID{scope: aScope, arena: aArena, id: aRes}
		return nil
	}
	if aRes != bRes || aArena != sc.bArena {
		return sc.resourceMismatchErr()
	}
	return nil
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

// isComponentSubtype checks that component type `a` is a subtype of component type `b`.
func (sc *SubtypeChecker) isComponentSubtype(a, b ComponentTypeID) error {
	at := sc.aArena.ComponentTypes[a]
	bt := sc.bArena.ComponentTypes[b]

	for name, bImport := range bt.Imports {
		aImport, ok := at.Imports[name]
		if !ok {
			return fmt.Errorf("missing expected import `%s`", name)
		}
		if err := sc.isSubtype(bImport, aImport); err != nil {
			return fmt.Errorf("type mismatch in import `%s`: %w", name, err)
		}
	}

	for name, bExport := range bt.Exports {
		aExport, ok := at.Exports[name]
		if !ok {
			return fmt.Errorf("export `%s` was not found", name)
		}
		if err := sc.isSubtype(aExport, bExport); err != nil {
			return fmt.Errorf("type mismatch in export `%s`: %w", name, err)
		}
	}

	return nil
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

// remapEntityType copies an entity type from aArena, rewriting resource
// references through resourceMapping and writing any derived types into
// writeArena. Used by validator-internal paths that stage the result of
// an instantiation; not used by the read-only CheckInstantiation path.
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
	default:
		return et
	}
}

// mintMapped returns a resource ID in writeArena corresponding to the
// source-arena resource srcID. Reuses any previously-minted ID for the
// same source so `(eq $r)` constraints across a single remap session
// stay consistent. resourceMapping keys are source-arena IDs (in aArena
// convention for remap use) and values are writeArena-scoped IDs.
//
// When srcID is the export-alias half of a fresh→defining pair recorded on
// the arena, mintMapped also mints the defining ID and records the alias
// pair in writeArena so the alias relationship survives into the new arena.
// That way a re-exported resource (e.g. borrower exports the lender's "r")
// continues to resolve to the source after instantiation freshens it.
func (sc *SubtypeChecker) mintMapped(srcID ResourceID) ResourceID {
	if mapped, ok := sc.resourceMapping[srcID]; ok && mapped.arena == sc.writeArena {
		return mapped.id
	}
	fresh := sc.writeArena.allocResourceID()
	sc.resourceMapping[srcID] = scopedResID{arena: sc.writeArena, id: fresh}
	if defining := sc.aArena.resolveResourceAlias(srcID); defining != srcID {
		definingFresh := sc.mintMapped(defining)
		if sc.writeArena.resourceAliases == nil {
			sc.writeArena.resourceAliases = make(map[ResourceID]ResourceID)
		}
		sc.writeArena.resourceAliases[fresh] = definingFresh
	}
	return fresh
}

func (sc *SubtypeChecker) remapAnyTypeID(id ComponentAnyTypeID) ComponentAnyTypeID {
	switch id.Kind {
	case AnyTypeResource:
		return ComponentAnyTypeID{Kind: AnyTypeResource, ResID: sc.mintMapped(id.ResID)}
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
	needsRebuild := sc.definedTypeHasResources(&dt)
	if !needsRebuild && len(sc.definedTypeMapping) > 0 {
		needsRebuild = sc.definedTypeReferencesMapped(&dt)
	}
	if !needsRebuild && sc.aArena == sc.writeArena {
		if sc.definedTypeRemap != nil {
			sc.definedTypeRemap[id] = id
		}
		return id
	}

	newDT := dt // copy
	switch dt.Kind {
	case DefinedKindOwn:
		newDT.Own = sc.mintMapped(dt.Own)
	case DefinedKindBorrow:
		newDT.Borrow = sc.mintMapped(dt.Borrow)
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

// definedTypeReferencesMapped returns true if any subtree of dt references a
// defined type that has a binding in definedTypeMapping. Used to decide whether
// remapDefinedType must rebuild a type even when it doesn't carry resources.
func (sc *SubtypeChecker) definedTypeReferencesMapped(dt *DefinedTypeDesc) bool {
	switch dt.Kind {
	case DefinedKindRecord:
		if dt.Record != nil {
			for _, f := range dt.Record.Fields {
				if sc.valTypeReferencesMapped(f.Type) {
					return true
				}
			}
		}
	case DefinedKindVariant:
		if dt.Variant != nil {
			for _, c := range dt.Variant.Cases {
				if c.Type.Valid && sc.valTypeReferencesMapped(c.Type.Value) {
					return true
				}
			}
		}
	case DefinedKindList:
		return sc.valTypeReferencesMapped(dt.List)
	case DefinedKindTuple:
		for _, t := range dt.Tuple {
			if sc.valTypeReferencesMapped(t) {
				return true
			}
		}
	case DefinedKindOption:
		return sc.valTypeReferencesMapped(dt.Option)
	case DefinedKindResult:
		if dt.ResultOk.Valid && sc.valTypeReferencesMapped(dt.ResultOk.Value) {
			return true
		}
		if dt.ResultErr.Valid && sc.valTypeReferencesMapped(dt.ResultErr.Value) {
			return true
		}
	}
	return false
}

func (sc *SubtypeChecker) valTypeReferencesMapped(vt ValTypeDesc) bool {
	if vt.IsPrimitive {
		return false
	}
	if _, ok := sc.definedTypeMapping[vt.TypeID]; ok {
		return true
	}
	dt := sc.aArena.DefinedTypes[vt.TypeID]
	return sc.definedTypeReferencesMapped(&dt)
}

// entityTypeReferencesMapped is the entity-level analogue: returns true when
// any defined-type ID anywhere inside et appears in definedTypeMapping. Used
// by remapInstanceType / remapComponentType to decide whether to rebuild even
// when no resources are present.
func (sc *SubtypeChecker) entityTypeReferencesMapped(et ComponentEntityType) bool {
	switch et.Kind {
	case EntityType:
		switch et.TypeRef.Kind {
		case AnyTypeDefined:
			id := ComponentDefinedTypeID(et.TypeRef.Index)
			if _, ok := sc.definedTypeMapping[id]; ok {
				return true
			}
			dt := sc.aArena.DefinedTypes[id]
			return sc.definedTypeReferencesMapped(&dt)
		case AnyTypeFunc:
			ft := sc.aArena.FuncTypes[et.TypeRef.Index]
			for _, p := range ft.Params {
				if sc.valTypeReferencesMapped(p.Type) {
					return true
				}
			}
			for _, r := range ft.Results {
				if sc.valTypeReferencesMapped(r.Type) {
					return true
				}
			}
		case AnyTypeInstance:
			it := sc.aArena.InstanceTypes[et.TypeRef.Index]
			for _, sub := range it.Exports {
				if sc.entityTypeReferencesMapped(sub) {
					return true
				}
			}
		}
	case EntityFunc:
		ft := sc.aArena.FuncTypes[et.FuncID]
		for _, p := range ft.Params {
			if sc.valTypeReferencesMapped(p.Type) {
				return true
			}
		}
		for _, r := range ft.Results {
			if sc.valTypeReferencesMapped(r.Type) {
				return true
			}
		}
	case EntityValue:
		return sc.valTypeReferencesMapped(et.ValType)
	case EntityInstance:
		it := sc.aArena.InstanceTypes[et.InstID]
		for _, sub := range it.Exports {
			if sc.entityTypeReferencesMapped(sub) {
				return true
			}
		}
	}
	return false
}

func (sc *SubtypeChecker) definedTypeHasResources(dt *DefinedTypeDesc) bool {
	switch dt.Kind {
	case DefinedKindOwn, DefinedKindBorrow:
		return true
	case DefinedKindRecord:
		if dt.Record != nil {
			for _, f := range dt.Record.Fields {
				if sc.valTypeHasResources(f.Type) {
					return true
				}
			}
		}
	case DefinedKindVariant:
		if dt.Variant != nil {
			for _, c := range dt.Variant.Cases {
				if c.Type.Valid && sc.valTypeHasResources(c.Type.Value) {
					return true
				}
			}
		}
	case DefinedKindList:
		return sc.valTypeHasResources(dt.List)
	case DefinedKindTuple:
		return slices.ContainsFunc(dt.Tuple, sc.valTypeHasResources)
	case DefinedKindOption:
		return sc.valTypeHasResources(dt.Option)
	case DefinedKindResult:
		if dt.ResultOk.Valid && sc.valTypeHasResources(dt.ResultOk.Value) {
			return true
		}
		if dt.ResultErr.Valid && sc.valTypeHasResources(dt.ResultErr.Value) {
			return true
		}
	}
	return false
}

func (sc *SubtypeChecker) valTypeHasResources(vt ValTypeDesc) bool {
	if vt.IsPrimitive {
		return false
	}
	dt := sc.aArena.DefinedTypes[vt.TypeID]
	return sc.definedTypeHasResources(&dt)
}

func (sc *SubtypeChecker) remapFuncType(id ComponentFuncTypeID) ComponentFuncTypeID {
	if sc.funcTypeRemap != nil {
		if cached, ok := sc.funcTypeRemap[id]; ok {
			return cached
		}
	}
	ft := sc.aArena.FuncTypes[id]
	hasRes := false
	for _, p := range ft.Params {
		if sc.valTypeHasResources(p.Type) {
			hasRes = true
			break
		}
	}
	if !hasRes {
		for _, r := range ft.Results {
			if sc.valTypeHasResources(r.Type) {
				hasRes = true
				break
			}
		}
	}
	if !hasRes && len(sc.definedTypeMapping) > 0 {
		for _, p := range ft.Params {
			if sc.valTypeReferencesMapped(p.Type) {
				hasRes = true
				break
			}
		}
		if !hasRes {
			for _, r := range ft.Results {
				if sc.valTypeReferencesMapped(r.Type) {
					hasRes = true
					break
				}
			}
		}
	}
	if !hasRes && sc.aArena == sc.writeArena {
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
	hasRes := false
	for _, et := range it.Exports {
		if sc.entityTypeHasResources(et) {
			hasRes = true
			break
		}
	}
	if !hasRes && len(sc.definedTypeMapping) > 0 {
		for _, et := range it.Exports {
			if sc.entityTypeReferencesMapped(et) {
				hasRes = true // reuse the rebuild path
				break
			}
		}
	}
	if !hasRes && sc.aArena == sc.writeArena {
		if sc.instanceTypeRemap != nil {
			sc.instanceTypeRemap[id] = id
		}
		return id
	}

	exports := make(map[string]ComponentEntityType)
	for name, et := range it.Exports {
		exports[name] = sc.remapEntityType(et)
	}
	var definedResources []ResourceID
	if len(it.DefinedResources) > 0 {
		definedResources = make([]ResourceID, len(it.DefinedResources))
		for i, oldRes := range it.DefinedResources {
			if mapped, ok := sc.resourceMapping[oldRes]; ok {
				definedResources[i] = mapped.id
			} else {
				definedResources[i] = oldRes
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
	hasRes := false
	for _, et := range ct.Imports {
		if sc.entityTypeHasResources(et) {
			hasRes = true
			break
		}
	}
	if !hasRes {
		for _, et := range ct.Exports {
			if sc.entityTypeHasResources(et) {
				hasRes = true
				break
			}
		}
	}
	if !hasRes && sc.aArena == sc.writeArena {
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
	return sc.writeArena.pushComponentType(ComponentTypeDesc{Imports: imports, ImportOrder: ct.ImportOrder, Exports: exports, ExportOrder: ct.ExportOrder})
}

func (sc *SubtypeChecker) entityTypeHasResources(et ComponentEntityType) bool {
	switch et.Kind {
	case EntityType:
		return et.TypeRef.Kind == AnyTypeResource || (et.TypeRef.Kind == AnyTypeDefined && sc.definedTypeHasResources(&sc.aArena.DefinedTypes[et.TypeRef.Index]))
	case EntityFunc:
		ft := sc.aArena.FuncTypes[et.FuncID]
		for _, p := range ft.Params {
			if sc.valTypeHasResources(p.Type) {
				return true
			}
		}
		for _, r := range ft.Results {
			if sc.valTypeHasResources(r.Type) {
				return true
			}
		}
	case EntityInstance:
		it := sc.aArena.InstanceTypes[et.InstID]
		for _, exp := range it.Exports {
			if sc.entityTypeHasResources(exp) {
				return true
			}
		}
	case EntityValue:
		return sc.valTypeHasResources(et.ValType)
	}
	return false
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
