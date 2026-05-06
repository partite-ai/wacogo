package wasmparser

import (
	"fmt"
)

// Types is a sentinel returned by (*Validator).Types to indicate that
// parsing has finished successfully. The underlying arena is not exposed
// — consumers should use the ComponentType handle returned by
// (*ValidatingParser).TopLevelComponentType instead.
type Types struct {
	arena *TypeArena
}

// validatorState tracks the overall state of the Validator.
type validatorState uint8

const (
	validatorStateUnparsed  validatorState = iota
	validatorStateComponent                // actively parsing a component
	validatorStateEnd                      // top-level component finished
)

// Validator validates a stream of Payload values from ParseAll.
type Validator struct {
	state      validatorState
	features   FeatureSet
	components []*ComponentState // stack, one per nesting level
	arena      *TypeArena

	// pendingCompTypeIDs tracks the ComponentTypeID placeholders for nested
	// components. When a nested component section starts, we push the
	// placeholder ID here; when the nested component's End is processed, we
	// pop it and fill in the actual imports/exports.
	pendingCompTypeIDs []ComponentTypeID

	// topLevelComponentTypeID holds the ComponentTypeID pushed onto the arena
	// when the outermost component finishes parsing. Valid only after parsing
	// completes successfully (topLevelValid == true).
	topLevelComponentTypeID ComponentTypeID
	topLevelValid           bool

	// lastValidatedModuleType holds the CoreModuleTypeID assigned during
	// the most recent validateModuleSection call. Read once by Next() and
	// cleared. Sentinel lastValidatedModuleValid distinguishes "unset"
	// from "ID 0".
	lastValidatedModuleType  CoreModuleTypeID
	lastValidatedModuleValid bool

	// lastImportEntries / lastExportEntries are populated in order during
	// validateImportSection / validateExportSection and consumed by Next()
	// when building a Validated*SectionPayload wrapper.
	lastImportEntries []ComponentEntityType
	lastExportEntries []ComponentEntityType
}

// NewValidator creates a new Validator with the given feature set.
func NewValidator(features FeatureSet) *Validator {
	return &Validator{
		state:    validatorStateUnparsed,
		features: features,
		arena:    newTypeArena(),
	}
}

// ValidatePayload dispatches to per-section validation.
func (v *Validator) ValidatePayload(p Payload) error {
	switch payload := p.(type) {
	case *VersionPayload:
		return v.validateVersion(payload)
	case *EndPayload:
		return v.validateEnd(payload)
	case *CustomSectionPayload:
		return nil
	// Core module sections — these are only emitted when parsing standalone modules.
	// The component validator handles nested modules via parseInnerCoreModule.
	case *ModuleTypeSectionPayload:
		return nil
	case *ModuleImportSectionPayload:
		return nil
	case *ModuleFunctionSectionPayload:
		return nil
	case *ModuleTableSectionPayload:
		return nil
	case *ModuleMemorySectionPayload:
		return nil
	case *ModuleGlobalSectionPayload:
		return nil
	case *ModuleExportSectionPayload:
		return nil
	case *ModuleStartSectionPayload:
		return nil
	case *ModuleElementSectionPayload:
		return nil
	case *ModuleCodeSectionPayload:
		return nil
	case *ModuleDataSectionPayload:
		return nil
	case *ModuleDataCountSectionPayload:
		return nil
	case *ModuleTagSectionPayload:
		return nil
	case *ModuleSectionPayload:
		return v.validateModuleSection(payload)
	case *ComponentSectionPayload:
		return v.validateComponentSection(payload)
	case *ComponentTypeSectionPayload:
		return v.validateTypeSection(payload)
	case *ComponentImportSectionPayload:
		return v.validateImportSection(payload)
	case *ComponentExportSectionPayload:
		return v.validateExportSection(payload)
	case *ComponentInstanceSectionPayload:
		return v.validateInstanceSection(payload)
	case *ComponentAliasSectionPayload:
		return v.validateAliasSection(payload)
	case *ComponentCanonicalSectionPayload:
		return v.validateCanonicalSection(payload)
	case *ComponentStartSectionPayload:
		return v.validateStartSection(payload)
	case *CoreTypeSectionPayload:
		return v.validateCoreTypeSection(payload)
	case *CoreInstanceSectionPayload:
		return v.validateCoreInstanceSection(payload)
	default:
		return fmt.Errorf("validator: unknown payload type %T", p)
	}
}

// Types returns completed type information after validation.
func (v *Validator) Types() (Types, error) {
	if v.state != validatorStateEnd {
		return Types{}, fmt.Errorf("validator: Types() called before end of component")
	}
	return Types{arena: v.arena}, nil
}

func (v *Validator) current() *ComponentState {
	if len(v.components) == 0 {
		return nil
	}
	return v.components[len(v.components)-1]
}

// ---------- Version / End ----------

func (v *Validator) validateVersion(_ *VersionPayload) error {
	cs := newComponentState(v.features, v.arena, ComponentKindComponent)
	v.components = append(v.components, cs)
	v.state = validatorStateComponent
	return nil
}

func (v *Validator) validateEnd(_ *EndPayload) error {
	if len(v.components) == 0 {
		return fmt.Errorf("validator: unexpected EndPayload with empty component stack")
	}
	child := v.components[len(v.components)-1]
	v.components = v.components[:len(v.components)-1]
	if len(v.components) == 0 {
		// Top-level component end: materialize its ComponentType.
		id := v.arena.pushComponentType(ComponentTypeDesc{
			Imports:     child.imports,
			ImportOrder: child.importOrder,
			Exports:     child.exports,
			ExportOrder: child.exportOrder,
		})
		if v.arena.ComponentTypeSizes[id] > MaxEffectiveTypeSize {
			return fmt.Errorf("effective type size exceeds the limit")
		}
		v.topLevelComponentTypeID = id
		v.topLevelValid = true
		v.state = validatorStateEnd
	} else if len(v.pendingCompTypeIDs) > 0 {
		// A nested component just finished — update the placeholder.
		compTypeID := v.pendingCompTypeIDs[len(v.pendingCompTypeIDs)-1]
		v.pendingCompTypeIDs = v.pendingCompTypeIDs[:len(v.pendingCompTypeIDs)-1]
		ct := &v.arena.ComponentTypes[compTypeID]
		ct.Imports = child.imports
		ct.ImportOrder = child.importOrder
		ct.Exports = child.exports
		ct.ExportOrder = child.exportOrder
		size := v.arena.computeComponentTypeSize(ct)
		v.arena.ComponentTypeSizes[compTypeID] = size
		if size > MaxEffectiveTypeSize {
			return fmt.Errorf("effective type size exceeds the limit")
		}
	}
	return nil
}

// ---------- Module Section ----------

func (v *Validator) validateModuleSection(p *ModuleSectionPayload) error {
	cs := v.current()
	if cs == nil {
		return fmt.Errorf("module section outside component")
	}
	if cs.kind != ComponentKindComponent {
		return fmt.Errorf("modules cannot be defined in type declarations")
	}

	// Parse the inner core module to extract its type information.
	modType, err := v.parseInnerCoreModule(p.Parser)
	if err != nil {
		return fmt.Errorf("in inline core module: %w", err)
	}
	modTypeID := v.arena.pushCoreModuleType(modType)
	cs.coreModules = append(cs.coreModules, modTypeID)

	v.lastValidatedModuleType = modTypeID
	v.lastValidatedModuleValid = true
	return nil
}

// ---------- Component Section ----------

func (v *Validator) validateComponentSection(_ *ComponentSectionPayload) error {
	cs := v.current()
	if cs == nil {
		return fmt.Errorf("component section outside component")
	}
	if cs.kind != ComponentKindComponent {
		return fmt.Errorf("components cannot be defined in type declarations")
	}
	// Push a placeholder component type. The actual type info is filled in
	// when the sub-component's EndPayload is validated (either by the caller
	// traversing the sub-ValidatingParser, or by lazy draining).
	compTypeID := v.arena.pushComponentType(ComponentTypeDesc{
		Imports: make(map[string]ComponentEntityType),
		Exports: make(map[string]ComponentEntityType),
	})
	cs.components = append(cs.components, compTypeID)
	v.pendingCompTypeIDs = append(v.pendingCompTypeIDs, compTypeID)
	return nil
}

// ---------- Core Type Section ----------


