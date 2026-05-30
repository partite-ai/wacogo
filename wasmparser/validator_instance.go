package wasmparser

import (
	"fmt"
	"maps"
	"strings"
)

func (v *Validator) validateInstanceSection(p *ComponentInstanceSectionPayload) error {
	cs := v.current()
	if cs == nil {
		return fmt.Errorf("instance section outside component")
	}
	for inst, err := range p.Items() {
		if err != nil {
			return err
		}
		if err := v.addInstance(cs, inst); err != nil {
			return err
		}
	}
	return nil
}

func (v *Validator) addInstance(cs *ComponentState, inst ComponentInstance) error {
	if cs.kind != ComponentKindComponent {
		return fmt.Errorf("instances cannot be defined in type declarations")
	}
	switch i := inst.(type) {
	case Instantiate:
		return v.instantiateComponent(cs, i)
	case InstantiateFromExports:
		return v.instantiateFromExports(cs, i)
	default:
		return fmt.Errorf("unknown instance type %T", inst)
	}
}

func (v *Validator) instantiateComponent(cs *ComponentState, inst Instantiate) error {
	compID, err := cs.getComponent(inst.ComponentIndex)
	if err != nil {
		return err
	}
	compType := v.arena.ComponentTypes[compID]

	// Build map of supplied args and validate each references a valid entity
	suppliedArgs := make(map[string]ComponentEntityType)
	for _, arg := range inst.Args {
		// Check for duplicate arg names
		if _, exists := suppliedArgs[arg.Name]; exists {
			return fmt.Errorf("instantiation argument `%s` conflicts with previous argument `%s`", arg.Name, arg.Name)
		}
		// Validate the arg references a valid entity
		argType, err := v.resolveExportedEntity(cs, arg.Kind, arg.Index)
		if err != nil {
			return err
		}
		suppliedArgs[arg.Name] = argType
	}

	// Check all imports are satisfied, building resource mapping.
	// Process in insertion order for deterministic error messages and correct resource mapping.
	sc := v.newSubtypeCheckerWithRemapping()
	for _, importName := range compType.ImportOrder {
		importType := compType.Imports[importName]
		argType, ok := suppliedArgs[importName]
		if !ok {
			return fmt.Errorf("missing import named `%s`", importName)
		}
		// Subtype check with resource remapping
		if err := sc.isSubtype(argType, importType); err != nil {
			return fmt.Errorf("type mismatch for import `%s`: %w", importName, err)
		}
	}

	// Build instance exports from the component's exports, remapping resource IDs
	instType := InstanceTypeDesc{
		Exports: make(map[string]ComponentEntityType),
	}
	for name, et := range compType.Exports {
		instType.Exports[name] = sc.remapEntityType(et)
	}

	id := v.arena.pushInstanceType(instType)
	if v.arena.InstanceTypeSizes[id] > MaxEffectiveTypeSize {
		return fmt.Errorf("effective type size exceeds the limit")
	}
	cs.instances = append(cs.instances, id)
	return nil
}

func (v *Validator) instantiateFromExports(cs *ComponentState, inst InstantiateFromExports) error {
	instType := InstanceTypeDesc{
		Exports: make(map[string]ComponentEntityType),
	}
	exportNames := make(map[string]string) // lowercase -> original for uniqueness

	for _, exp := range inst.Exports {
		// Validate the entity index first (to match Rust validator error ordering)
		et, err := v.resolveExportedEntity(cs, exp.Kind, exp.Index)
		if err != nil {
			return err
		}

		// Validate name
		if err := ValidateExportNameStr(exp.Name); err != nil {
			if _, ok := err.(*ErrNotValidExternName); ok {
				return fmt.Errorf("`%s` is not a valid extern name: %s", exp.Name.Name, err)
			}
			return fmt.Errorf("`%s` is not a valid export name: %s", exp.Name.Name, err)
		}
		lower := strings.ToLower(exp.Name.Name)
		if prev, ok := exportNames[lower]; ok {
			if prev == exp.Name.Name {
				return fmt.Errorf("export name `%s` conflicts with previous name `%s`", exp.Name.Name, exp.Name.Name)
			}
			return fmt.Errorf("export name `%s` conflicts with previous name `%s`", exp.Name.Name, prev)
		}
		exportNames[lower] = exp.Name.Name

		// Validate bracket-prefixed names using the outer component's export context.
		// Resources must have names in the component's context, not just the instance.
		if err := v.validateBracketName(cs, exp.Name.Name, et, "export"); err != nil {
			return err
		}

		instType.Exports[exp.Name.Name] = et
	}

	id := v.arena.pushInstanceType(instType)
	if v.arena.InstanceTypeSizes[id] > MaxEffectiveTypeSize {
		return fmt.Errorf("effective type size exceeds the limit")
	}
	cs.instances = append(cs.instances, id)
	return nil
}

// ---------- Core Instance Section ----------

func (v *Validator) validateCoreInstanceSection(p *CoreInstanceSectionPayload) error {
	cs := v.current()
	if cs == nil {
		return fmt.Errorf("core instance section outside component")
	}
	for inst, err := range p.Items() {
		if err != nil {
			return err
		}
		if err := v.addCoreInstance(cs, inst); err != nil {
			return err
		}
	}
	return nil
}

func (v *Validator) addCoreInstance(cs *ComponentState, inst Instance) error {
	if cs.kind != ComponentKindComponent {
		return fmt.Errorf("core instances cannot be defined in type declarations")
	}
	switch i := inst.(type) {
	case CoreInstantiate:
		return v.coreInstantiate(cs, i)
	case CoreInstantiateFromExports:
		return v.coreInstantiateFromExports(cs, i)
	default:
		return fmt.Errorf("unknown core instance type %T", inst)
	}
}

func (v *Validator) coreInstantiate(cs *ComponentState, inst CoreInstantiate) error {
	modID, err := cs.getCoreModule(inst.ModuleIndex)
	if err != nil {
		return err
	}
	modType := v.arena.CoreModuleTypes[modID]

	// Build instance type from module exports
	ciType := CoreInstanceTypeDesc{
		Exports: make(map[string]CoreEntityType, len(modType.Exports)),
	}
	maps.Copy(ciType.Exports, modType.Exports)

	// Collect unique import module names from the module's imports
	importModules := make(map[string]bool)
	for key := range modType.Imports {
		importModules[key.Module] = true
	}

	// Validate args are instances and build a map from name to instance type
	argInstances := make(map[string]CoreInstanceTypeID)
	for _, arg := range inst.Args {
		if _, exists := argInstances[arg.Name]; exists {
			return fmt.Errorf("duplicate module instantiation argument named `%s`", arg.Name)
		}
		if arg.Kind != CoreSortInstance {
			return fmt.Errorf("core instantiation arg must be an instance")
		}
		ciID, err := cs.getCoreInstance(arg.Index)
		if err != nil {
			return err
		}
		argInstances[arg.Name] = ciID
	}

	// Check that all import module names are satisfied by args
	for modName := range importModules {
		argCiID, ok := argInstances[modName]
		if !ok {
			return fmt.Errorf("missing module instantiation argument named `%s`", modName)
		}
		argCiType := v.arena.CoreInstanceTypes[argCiID]

		// Check that the arg instance provides all required imports.
		// Use the ordered import keys to check in source order.
		for _, key := range modType.ImportOrder {
			if key.Module != modName {
				continue
			}
			expectedImport := modType.Imports[key]
			actualExport, ok := argCiType.Exports[key.Name]
			if !ok {
				return fmt.Errorf("module instantiation argument `%s` does not export an item named `%s`", modName, key.Name)
			}
			if err := checkCoreEntityTypeMatch(v.arena, v.arena, expectedImport, actualExport); err != nil {
				return fmt.Errorf("type mismatch for module import `%s::%s`: %w", modName, key.Name, err)
			}
		}
	}

	id := v.arena.pushCoreInstanceType(ciType)
	cs.coreInstances = append(cs.coreInstances, id)
	return nil
}

// checkCoreEntityTypeMatch checks that an actual core entity type is compatible
// with an expected import type. The expected/actual sides may live in different
// arenas (e.g. cross-component subtype checks); expArena holds expected's
// referenced types, actArena holds actual's. Pass the same arena for both
// when the two sides come from one arena.
//
// Kind-mismatch messages embed both `expected K, found K2` (wasm-tools style)
// and `expected K found K2` (wasmtime style) so substring assertions in either
// upstream spec suite are satisfied by the same error.
func checkCoreEntityTypeMatch(expArena, actArena *TypeArena, expected, actual CoreEntityType) error {
	if expected.Kind != actual.Kind {
		exp := coreEntityKindName(expected.Kind)
		act := coreEntityKindName(actual.Kind)
		return fmt.Errorf("expected %[1]s, found %[2]s; expected %[1]s found %[2]s", exp, act)
	}
	switch expected.Kind {
	case CoreEntityFunc:
		return checkCoreFuncTypeMatch(expArena, actArena, expected.Func, actual.Func)
	case CoreEntityTable:
		return checkCoreTableTypeMatch(expected.Table, actual.Table)
	case CoreEntityMemory:
		return checkCoreMemoryTypeMatch(expected.Memory, actual.Memory)
	case CoreEntityGlobal:
		return checkCoreGlobalTypeMatch(expected.Global, actual.Global)
	}
	return nil
}

func coreEntityKindName(k CoreEntityKind) string {
	switch k {
	case CoreEntityFunc:
		return "func"
	case CoreEntityTable:
		return "table"
	case CoreEntityMemory:
		return "memory"
	case CoreEntityGlobal:
		return "global"
	}
	return "unknown"
}

func checkCoreFuncTypeMatch(expArena, actArena *TypeArena, expectedID, actualID CoreFuncTypeID) error {
	expected := expArena.CoreFuncTypes[expectedID]
	actual := actArena.CoreFuncTypes[actualID]
	if coreFuncTypesEqual(expected, actual) {
		return nil
	}
	detail := coreFuncTypeMismatchDetail(expected, actual)
	return fmt.Errorf("expected: (func): %s; expected type `%s`, found type `%s`",
		detail, formatCoreFuncType(expected), formatCoreFuncType(actual))
}

// coreFuncTypeMismatchDetail returns a wasm-tools-style description of the
// first concrete difference between two core func types. Callers compose it
// into a larger error message that also carries the wasmtime-style
// "expected type `(func ...)`, found type `(func ...)`" rendering.
func coreFuncTypeMismatchDetail(expected, actual CoreFuncTypeDesc) string {
	if len(expected.Params) != len(actual.Params) {
		return fmt.Sprintf("expected %d parameters, found %d", len(expected.Params), len(actual.Params))
	}
	for i := range expected.Params {
		if expected.Params[i] != actual.Params[i] {
			return fmt.Sprintf("parameter type mismatch at index %d", i)
		}
	}
	if len(expected.Results) != len(actual.Results) {
		if len(expected.Results) > 0 && len(actual.Results) == 0 {
			return "expected a result, found none"
		}
		return fmt.Sprintf("expected %d results, found %d", len(expected.Results), len(actual.Results))
	}
	for i := range expected.Results {
		if expected.Results[i] != actual.Results[i] {
			return fmt.Sprintf("result type mismatch at index %d", i)
		}
	}
	return "type mismatch"
}

func coreFuncTypesEqual(a, b CoreFuncTypeDesc) bool {
	if len(a.Params) != len(b.Params) || len(a.Results) != len(b.Results) {
		return false
	}
	for i := range a.Params {
		if a.Params[i] != b.Params[i] {
			return false
		}
	}
	for i := range a.Results {
		if a.Results[i] != b.Results[i] {
			return false
		}
	}
	return true
}

// formatCoreFuncType renders a core func type in wasmtime's WAT-style notation,
// e.g. `(func)`, `(func (param i32))`, `(func (param i32) (result i64))`.
func formatCoreFuncType(t CoreFuncTypeDesc) string {
	var b strings.Builder
	b.WriteString("(func")
	if len(t.Params) > 0 {
		b.WriteString(" (param")
		for _, p := range t.Params {
			b.WriteByte(' ')
			b.WriteString(coreValTypeName(p))
		}
		b.WriteByte(')')
	}
	if len(t.Results) > 0 {
		b.WriteString(" (result")
		for _, r := range t.Results {
			b.WriteByte(' ')
			b.WriteString(coreValTypeName(r))
		}
		b.WriteByte(')')
	}
	b.WriteByte(')')
	return b.String()
}

func checkCoreTableTypeMatch(expected, actual CoreTableType) error {
	if expected.ElemType != actual.ElemType {
		return fmt.Errorf("expected table element type %s, found %s",
			coreValTypeName(expected.ElemType), coreValTypeName(actual.ElemType))
	}
	if err := checkCoreLimitsMatch("table", uint64(expected.Min), optU32ToU64(expected.Max),
		uint64(actual.Min), optU32ToU64(actual.Max)); err != nil {
		return err
	}
	return nil
}

func checkCoreMemoryTypeMatch(expected, actual CoreMemoryType) error {
	if expected.Shared != actual.Shared {
		return fmt.Errorf("mismatch in the shared flag for memories")
	}
	if expected.Mem64 != actual.Mem64 {
		return fmt.Errorf("mismatch in the memory64 flag for memories")
	}
	if err := checkCoreLimitsMatch("memory", expected.Min, expected.Max,
		actual.Min, actual.Max); err != nil {
		return err
	}
	return nil
}

func checkCoreGlobalTypeMatch(expected, actual CoreGlobalType) error {
	if expected.ValType != actual.ValType {
		return fmt.Errorf("expected global type %s, found %s",
			coreValTypeName(expected.ValType), coreValTypeName(actual.ValType))
	}
	if expected.Mutable != actual.Mutable {
		return fmt.Errorf("global mutability mismatch")
	}
	return nil
}

func coreValTypeName(t CoreValType) string {
	switch t {
	case CoreValTypeI32:
		return "i32"
	case CoreValTypeI64:
		return "i64"
	case CoreValTypeF32:
		return "f32"
	case CoreValTypeF64:
		return "f64"
	case CoreValTypeV128:
		return "v128"
	case CoreValTypeFuncRef:
		return "funcref"
	case CoreValTypeExternRef:
		return "externref"
	}
	return "unknown"
}

func optU32ToU64(o Optional[uint32]) Optional[uint64] {
	if o.Valid {
		return Some(uint64(o.Value))
	}
	return Optional[uint64]{}
}

// checkCoreLimitsMatch checks import/export limits compatibility.
// For imports: the actual (provided) limits must be at least as restrictive.
// actual.min >= expected.min, and if expected has max, actual must have max <= expected.max
func checkCoreLimitsMatch(entity string, expectedMin uint64, expectedMax Optional[uint64],
	actualMin uint64, actualMax Optional[uint64]) error {
	if actualMin < expectedMin {
		return fmt.Errorf("mismatch in %s limits: expected min %d, got %d", entity, expectedMin, actualMin)
	}
	if expectedMax.Valid {
		if !actualMax.Valid {
			return fmt.Errorf("mismatch in %s limits: expected max %d, got no max", entity, expectedMax.Value)
		}
		if actualMax.Value > expectedMax.Value {
			return fmt.Errorf("mismatch in %s limits: expected max %d, got %d", entity, expectedMax.Value, actualMax.Value)
		}
	}
	return nil
}

func (v *Validator) coreInstantiateFromExports(cs *ComponentState, inst CoreInstantiateFromExports) error {
	ciType := CoreInstanceTypeDesc{
		Exports: make(map[string]CoreEntityType),
	}

	for _, exp := range inst.Exports {
		// Check for duplicate export names
		if _, exists := ciType.Exports[exp.Name]; exists {
			return fmt.Errorf("export name `%s` already defined", exp.Name)
		}
		switch exp.Kind {
		case CoreSortFunc:
			funcID, err := cs.getCoreFunc(exp.Index)
			if err != nil {
				return err
			}
			ciType.Exports[exp.Name] = CoreEntityType{Kind: CoreEntityFunc, Func: funcID}
		case CoreSortTable:
			tbl, err := cs.getCoreTable(exp.Index)
			if err != nil {
				return err
			}
			ciType.Exports[exp.Name] = CoreEntityType{Kind: CoreEntityTable, Table: tbl}
		case CoreSortMemory:
			mem, err := cs.getCoreMemory(exp.Index)
			if err != nil {
				return err
			}
			ciType.Exports[exp.Name] = CoreEntityType{Kind: CoreEntityMemory, Memory: mem}
		case CoreSortGlobal:
			gbl, err := cs.getCoreGlobal(exp.Index)
			if err != nil {
				return err
			}
			ciType.Exports[exp.Name] = CoreEntityType{Kind: CoreEntityGlobal, Global: gbl}
		default:
			return fmt.Errorf("unsupported core sort in instance from exports")
		}
	}

	id := v.arena.pushCoreInstanceType(ciType)
	cs.coreInstances = append(cs.coreInstances, id)
	return nil
}

// ---------- Start Section ----------

func (v *Validator) validateStartSection(p *ComponentStartSectionPayload) error {
	cs := v.current()
	if cs == nil {
		return fmt.Errorf("start section outside component")
	}
	if cs.kind != ComponentKindComponent {
		return fmt.Errorf("start not allowed in type declarations")
	}
	if cs.hasStart {
		return fmt.Errorf("multiple start sections")
	}
	cs.hasStart = true

	// Validate function index
	funcID, err := cs.getFunc(p.Start.FuncIndex)
	if err != nil {
		return err
	}
	funcType := v.arena.FuncTypes[funcID]

	// Validate argument count
	if uint32(len(funcType.Params)) != uint32(len(p.Start.Args)) {
		return fmt.Errorf("start function requires %d args, got %d", len(funcType.Params), len(p.Start.Args))
	}

	// Validate each arg is a valid value index
	for _, argIdx := range p.Start.Args {
		_, err := cs.getValue(argIdx)
		if err != nil {
			return err
		}
	}

	// Add results as values
	if p.Start.Results != uint32(len(funcType.Results)) {
		return fmt.Errorf("start function declares %d results, but function type has %d", p.Start.Results, len(funcType.Results))
	}
	for _, r := range funcType.Results {
		cs.values = append(cs.values, valueEntry{ty: r.Type})
	}
	return nil
}

// ---------- Helpers ----------
