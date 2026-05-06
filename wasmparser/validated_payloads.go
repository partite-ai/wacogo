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
// validator resolved for it. FuncType(), ModuleType(), and
// ComponentType() each return a non-nil handle iff the export's Kind
// matches; otherwise they return nil. InstanceType() is present for
// API symmetry but always returns nil — see its comment.
type ValidatedComponentExport struct {
	Export *ComponentExport
	et     ComponentEntityType
	arena  *TypeArena
}

// FuncType returns the handle if this export is a func, else nil.
func (v ValidatedComponentExport) FuncType() *FuncType {
	if v.et.Kind != EntityFunc {
		return nil
	}
	return &FuncType{arena: v.arena, id: v.et.FuncID}
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

// InstanceType returns nil. Instance-typed exports do not surface as
// *InstanceType through this API — *InstanceType represents a fresh
// runtime instance minted via (*ComponentType).NewInstance, not the
// static instance-type of a component export. This method exists so
// the four-accessor shape across handle kinds is complete; callers
// needing instance-type info should consult the raw Export directly.
func (v ValidatedComponentExport) InstanceType() *InstanceType {
	return nil
}

// ValidatedComponentImport is an import paired with the handle the
// validator resolved for it. FuncType(), ModuleType(), and
// ComponentType() each return a non-nil handle iff the import's Kind
// matches; otherwise they return nil. InstanceType() is present for
// API symmetry but always returns nil — see its comment.
type ValidatedComponentImport struct {
	Import *ComponentImport
	et     ComponentEntityType
	arena  *TypeArena
}

// FuncType returns the handle if this import is a func, else nil.
func (v ValidatedComponentImport) FuncType() *FuncType {
	if v.et.Kind != EntityFunc {
		return nil
	}
	return &FuncType{arena: v.arena, id: v.et.FuncID}
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

// InstanceType returns nil. Instance-typed imports do not surface as
// *InstanceType through this API — *InstanceType represents a fresh
// runtime instance minted via (*ComponentType).NewInstance, not the
// static instance-type of a component import. This method exists so
// the four-accessor shape across handle kinds is complete; callers
// needing instance-type info should consult the raw Import directly.
func (v ValidatedComponentImport) InstanceType() *InstanceType {
	return nil
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
