package core

import (
	"context"

	"github.com/partite-ai/wacogo/wasmparser"
	"github.com/tetratelabs/wazero/api"
)

// InstanceKind is the subset of Sort enumerants that NewInstance
// accepts when populating an instance's exports.
type InstanceKind uint8

const (
	InstanceKindFunc InstanceKind = iota
	InstanceKindType
	InstanceKindInstance
	InstanceKindCoreModule
)

// InstanceSpec is the parameter struct for NewInstance. It carries
// only wasm-level pieces — host-specific concerns (user state,
// cleanups, type-resource slot tables) live in the host package's
// wrapper, not here.
type InstanceSpec struct {
	// Modules are closed in declared order during ComponentInstance.Close.
	// Callers control the order; core does not interpret which is which.
	Modules []api.Module

	// ParserInstanceType is the wasmparser instance-type handle minted
	// at creation time. Optional; nil means this instance cannot be
	// supplied as an instance import to another component's
	// Instantiate call (typical for sub-component instances).
	ParserInstanceType *wasmparser.InstanceType

	// ExternTable, when non-nil, is the read-only lookup table queried
	// by (*ComponentInstance).LookupExtern. Host wrappers pass their
	// per-instance extern table here; sub-component instances leave it nil.
	ExternTable ExternTable

	// BuildExports, when non-nil, is invoked once during NewInstance
	// after the *ComponentInstance is allocated but before it is
	// returned. The callback may capture inst into the values it
	// returns (e.g. canon.Callees that reference inst for
	// resource-table lookup). Any error it returns is propagated from
	// NewInstance.
	BuildExports func(inst *ComponentInstance) (types []InstanceTypeSlot, exports []InstanceExport, err error)
}

// InstanceTypeSlot is one entry in an instance's type index space.
// Name may be "" for anonymous structural types.
type InstanceTypeSlot struct {
	Name string
	Type Type
}

// InstanceExport is one entry in an instance's exports map.
type InstanceExport struct {
	Name string
	Kind InstanceKind

	// FuncVal is non-nil when Kind == InstanceKindFunc.
	FuncVal *ExportedFunc

	// Instance is non-nil when Kind == InstanceKindInstance.
	Instance *ComponentInstance

	// CompiledModule is non-nil when Kind == InstanceKindCoreModule.
	CompiledModule *CompiledModule

	// TypeIdx is valid when Kind == InstanceKindType; it indexes into
	// the types slice returned alongside this export.
	TypeIdx uint32
}

// NewInstance constructs a *ComponentInstance from an InstanceSpec.
// If spec.BuildExports is non-nil, it is invoked during construction
// to populate the instance's type-index space and exports map; any
// error it returns is propagated.
func NewInstance(e *Engine, spec *InstanceSpec) (*ComponentInstance, error) {
	inst := &ComponentInstance{
		engine:  e,
		exports: map[string]exportEntry{},
	}

	inst.canLeave = true
	inst.resources = NewResourceTable(inst)

	if len(spec.Modules) > 0 {
		inst.coreInstances = append(inst.coreInstances, spec.Modules...)
	}

	if spec.ParserInstanceType != nil {
		inst.wpInstance = spec.ParserInstanceType
	}

	if spec.ExternTable != nil {
		inst.externTable = spec.ExternTable
	}

	if spec.BuildExports != nil {
		types, exports, err := spec.BuildExports(inst)
		if err != nil {
			// Close already-attached modules in reverse declaration order so
			// the failed instance does not leak wazero state. Errors from
			// the close are dropped — the BuildExports error is the cause.
			closeCtx := context.Background()
			for i := len(inst.coreInstances) - 1; i >= 0; i-- {
				_ = inst.coreInstances[i].Close(closeCtx)
			}
			return nil, err
		}
		inst.initExports(types, exports)
	}
	return inst, nil
}

func (inst *ComponentInstance) initExports(types []InstanceTypeSlot, exports []InstanceExport) {
	if len(types) > 0 {
		inst.types = make([]Type, len(types))
		for i, slot := range types {
			inst.types[i] = slot.Type
		}
	}

	if inst.exports == nil {
		inst.exports = make(map[string]exportEntry, len(exports))
	}
	for _, es := range exports {
		entry := exportEntry{kind: instanceKindToSort(es.Kind)}
		switch es.Kind {
		case InstanceKindFunc:
			entry.funcVal = es.FuncVal
			if es.FuncVal != nil && es.FuncVal.instance == nil {
				es.FuncVal.instance = inst
			}
		case InstanceKindType:
			entry.typeIdx = es.TypeIdx
		case InstanceKindInstance:
			entry.instance = es.Instance
		case InstanceKindCoreModule:
			entry.compiledModule = es.CompiledModule
		}
		inst.exports[es.Name] = entry
	}

	for _, slot := range types {
		if tr, ok := slot.Type.(*TypeResource); ok {
			if tr.instance == nil {
				tr.instance = inst
			}
		}
	}
}

func instanceKindToSort(k InstanceKind) Sort {
	switch k {
	case InstanceKindFunc:
		return SortFunc
	case InstanceKindType:
		return SortType
	case InstanceKindInstance:
		return SortInstance
	case InstanceKindCoreModule:
		return SortCoreModule
	default:
		panic("wacogo/core: unknown InstanceKind")
	}
}
