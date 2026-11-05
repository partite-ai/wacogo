package wasmparser

import "iter"

// ValidatedModuleSectionPayload is yielded by (*ValidatingParser).Next
// for each locally-defined core module. It embeds the raw payload fields
// via the Raw() accessor and exposes the core-module type handle the
// validator assigned.
type ValidatedModuleSectionPayload struct {
	raw        *ModuleSectionPayload
	moduleType CoreModuleTypeID
	arena      *TypeArena
}

func (*ValidatedModuleSectionPayload) payload() {}

// Raw returns the underlying raw payload for callers that need the
// module binary bytes, range, or nested parser.
func (p *ValidatedModuleSectionPayload) Raw() *ModuleSectionPayload { return p.raw }

// ModuleType returns a handle to this core module's type. Never nil on
// a valid payload.
func (p *ValidatedModuleSectionPayload) ModuleType() *ModuleType {
	return &ModuleType{arena: p.arena, id: p.moduleType}
}

// ValidatedComponentExportSectionPayload is yielded by
// (*ValidatingParser).Next for component export sections. Items()
// yields handle-bearing ValidatedComponentExport values.
type ValidatedComponentExportSectionPayload struct {
	raw     *ComponentExportSectionPayload
	entries []ComponentEntityType // parallel to raw.Items() order
	arena   *TypeArena
}

func (*ValidatedComponentExportSectionPayload) payload() {}

// Raw returns the underlying raw payload.
func (p *ValidatedComponentExportSectionPayload) Raw() *ComponentExportSectionPayload {
	return p.raw
}

// Items returns an iterator yielding each export paired with the handle
// the validator resolved for it. Errors from the underlying raw Items()
// are propagated unchanged.
func (p *ValidatedComponentExportSectionPayload) Items() iter.Seq2[ValidatedComponentExport, error] {
	return func(yield func(ValidatedComponentExport, error) bool) {
		i := 0
		for exp, err := range p.raw.Items() {
			if err != nil {
				yield(ValidatedComponentExport{}, err)
				return
			}
			var et ComponentEntityType
			if i < len(p.entries) {
				et = p.entries[i]
			}
			if !yield(ValidatedComponentExport{Export: exp, et: et, arena: p.arena}, nil) {
				return
			}
			i++
		}
	}
}

// ValidatedComponentImportSectionPayload is the import-side mirror.
type ValidatedComponentImportSectionPayload struct {
	raw     *ComponentImportSectionPayload
	entries []ComponentEntityType
	arena   *TypeArena
}

func (*ValidatedComponentImportSectionPayload) payload() {}

// Raw returns the underlying raw payload.
func (p *ValidatedComponentImportSectionPayload) Raw() *ComponentImportSectionPayload {
	return p.raw
}

// Items returns an iterator yielding each import paired with the handle
// the validator resolved for it. Errors from the underlying raw Items()
// are propagated unchanged.
func (p *ValidatedComponentImportSectionPayload) Items() iter.Seq2[ValidatedComponentImport, error] {
	return func(yield func(ValidatedComponentImport, error) bool) {
		i := 0
		for imp, err := range p.raw.Items() {
			if err != nil {
				yield(ValidatedComponentImport{}, err)
				return
			}
			var et ComponentEntityType
			if i < len(p.entries) {
				et = p.entries[i]
			}
			if !yield(ValidatedComponentImport{Import: imp, et: et, arena: p.arena}, nil) {
				return
			}
			i++
		}
	}
}

// ValidatedComponentExport is an export paired with the handle the
// validator resolved for it. ModuleType() and ComponentType() each return a
// non-nil handle iff the export's Kind matches; otherwise they return nil.
type ValidatedComponentExport struct {
	Export *ComponentExport
	et     ComponentEntityType
	arena  *TypeArena
}

// ModuleType returns the handle if this export is a core module, else nil.
func (v ValidatedComponentExport) ModuleType() *ModuleType {
	if v.et.Kind != EntityModule {
		return nil
	}
	return &ModuleType{arena: v.arena, id: v.et.ModuleID}
}

// ComponentType returns the handle if this export is a component, else nil.
func (v ValidatedComponentExport) ComponentType() *ComponentType {
	if v.et.Kind != EntityComponent {
		return nil
	}
	return &ComponentType{arena: v.arena, id: v.et.CompID}
}

// ValidatedComponentImport is an import paired with the handle the
// validator resolved for it. ModuleType() and ComponentType() each return a
// non-nil handle iff the import's Kind matches; otherwise they return nil.
type ValidatedComponentImport struct {
	Import *ComponentImport
	et     ComponentEntityType
	arena  *TypeArena
}

// ModuleType returns the handle if this import is a core module, else nil.
func (v ValidatedComponentImport) ModuleType() *ModuleType {
	if v.et.Kind != EntityModule {
		return nil
	}
	return &ModuleType{arena: v.arena, id: v.et.ModuleID}
}

// ComponentType returns the handle if this import is a component, else nil.
func (v ValidatedComponentImport) ComponentType() *ComponentType {
	if v.et.Kind != EntityComponent {
		return nil
	}
	return &ComponentType{arena: v.arena, id: v.et.CompID}
}

// IsInstanceRuntimeEmpty reports whether this import is an instance type
// that carries no runtime data: every export is a type export or a
// recursively runtime-empty instance, and the instance introduces no
// fresh resource identities. Such an import is fully determined by the
// consumer's static knowledge — any alias/equality it declares resolves
// against types the consumer already has — so hosts may instantiate
// without supplying an argument for it. Returns false for non-instance
// imports and for instance imports whose type includes any runtime-valued
// export (func/module/component/value, or non-empty nested instance) or
// any freshly-defined resource.
func (v ValidatedComponentImport) IsInstanceRuntimeEmpty() bool {
	if v.et.Kind != EntityInstance {
		return false
	}
	return isInstanceTypeRuntimeEmpty(v.arena, v.et.InstID)
}

// EntityType returns the type the validator resolved for this import, along
// with the arena its IDs index into.
func (v ValidatedComponentImport) EntityType() (ComponentEntityType, *TypeArena) {
	return v.et, v.arena
}

func isInstanceTypeRuntimeEmpty(arena *TypeArena, instID ComponentInstanceTypeID) bool {
	it := arena.InstanceTypes[instID]
	if len(it.DefinedResources) > 0 {
		return false
	}
	for _, e := range it.Exports {
		switch e.Kind {
		case EntityType:
			continue
		case EntityInstance:
			if !isInstanceTypeRuntimeEmpty(arena, e.InstID) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// ValidatedComponentAliasSectionPayload is the alias-side mirror of
// ValidatedComponentImportSectionPayload.
type ValidatedComponentAliasSectionPayload struct {
	raw     *ComponentAliasSectionPayload
	entries []ComponentEntityType
	arena   *TypeArena
}

func (*ValidatedComponentAliasSectionPayload) payload() {}

// Raw returns the underlying raw payload.
func (p *ValidatedComponentAliasSectionPayload) Raw() *ComponentAliasSectionPayload {
	return p.raw
}

// Items returns an iterator yielding each alias paired with the type the
// validator resolved for it.
func (p *ValidatedComponentAliasSectionPayload) Items() iter.Seq2[ValidatedComponentAlias, error] {
	return func(yield func(ValidatedComponentAlias, error) bool) {
		i := 0
		for alias, err := range p.raw.Items() {
			if err != nil {
				yield(ValidatedComponentAlias{}, err)
				return
			}
			var et ComponentEntityType
			if i < len(p.entries) {
				et = p.entries[i]
			}
			if !yield(ValidatedComponentAlias{Alias: alias, et: et, arena: p.arena}, nil) {
				return
			}
			i++
		}
	}
}

// ValidatedComponentAlias is an alias paired with the type the validator
// resolved for it.
type ValidatedComponentAlias struct {
	Alias ComponentAlias
	et    ComponentEntityType
	arena *TypeArena
}

// EntityType returns the type the validator resolved for an instance-export
// alias, along with the arena its IDs index into. ok is false for any other
// kind of alias.
func (v ValidatedComponentAlias) EntityType() (et ComponentEntityType, arena *TypeArena, ok bool) {
	if _, ok := v.Alias.(AliasInstanceExport); !ok {
		return ComponentEntityType{}, nil, false
	}
	return v.et, v.arena, true
}

// IsInstanceRuntimeEmpty reports whether this alias names an instance that needs
// nothing at runtime, so that the instance it comes from may omit it. See
// [ValidatedComponentImport.IsInstanceRuntimeEmpty].
func (v ValidatedComponentAlias) IsInstanceRuntimeEmpty() bool {
	if _, ok := v.Alias.(AliasInstanceExport); !ok || v.et.Kind != EntityInstance {
		return false
	}
	return isInstanceTypeRuntimeEmpty(v.arena, v.et.InstID)
}

// ValidatedComponentCanonicalSectionPayload is a canonical function section with
// what the validator worked out about each function.
type ValidatedComponentCanonicalSectionPayload struct {
	raw     *ComponentCanonicalSectionPayload
	entries []ValidatedCanonicalFunction
}

func (*ValidatedComponentCanonicalSectionPayload) payload() {}

// Raw returns the underlying raw payload.
func (p *ValidatedComponentCanonicalSectionPayload) Raw() *ComponentCanonicalSectionPayload {
	return p.raw
}

// Items returns the section's functions in order.
func (p *ValidatedComponentCanonicalSectionPayload) Items() []ValidatedCanonicalFunction {
	return p.entries
}

// ValidatedCanonicalFunction is a canonical function paired with the signature of
// the core function it creates.
type ValidatedCanonicalFunction struct {
	Function        CanonicalFunction
	created         bool
	params, results []CoreValType
}

// CoreFunc returns the signature of the core function this creates. A canon lift
// creates a component function, so it has none.
func (f ValidatedCanonicalFunction) CoreFunc() (params, results []CoreValType, ok bool) {
	return f.params, f.results, f.created
}
