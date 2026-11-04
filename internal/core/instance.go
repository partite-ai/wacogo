package core

import (
	"context"
	"errors"

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
	// Modules are closed in reverse declared order during ComponentInstance.Close.
	// Callers control the order; core does not interpret which is which.
	Modules []api.Module

	// ParserInstanceType is the wasmparser handle to this instance's actual
	// type. Optional; if nil, this instance's type is not checked when it is
	// supplied as an instance import to another component's Instantiate call.
	ParserInstanceType *wasmparser.InstanceType

	// ExternTable, when non-nil, is the read-only lookup table queried
	// by (*ComponentInstance).LookupExtern. Host wrappers pass their
	// per-instance extern table here; sub-component instances leave it nil.
	ExternTable ExternTable

	// BuildExports assembles the instance's exports. It runs once during
	// NewInstance, after the *ComponentInstance is allocated but before
	// it is returned, so that exports which refer back to the instance
	// can be constructed: the resource types it defines, minted with
	// NewTypeResource(inst, …), and the canon callees its exported
	// funcs carry. Any error it returns is propagated from NewInstance,
	// which closes the attached modules rather than returning a
	// half-built instance.
	BuildExports func(inst *ComponentInstance) ([]InstanceExport, error)
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

	// Type is the exported type when Kind == InstanceKindType. Naming a
	// type here makes it resolvable on this instance; it says nothing
	// about which instance defines it, which is fixed when the type is
	// created. An instance assembled purely from exports names types
	// its enclosing instance defines.
	Type Type
}

// NewInstance constructs a *ComponentInstance from an InstanceSpec.
// The returned instance is fully built: if spec.BuildExports is
// non-nil it is invoked during construction to assemble the instance's
// exports, and any error it returns is propagated instead of a
// partially populated instance.
func NewInstance(e *Engine, spec *InstanceSpec) (*ComponentInstance, error) {
	inst := &ComponentInstance{
		engine:  e,
		exports: map[string]exportEntry{},
	}

	inst.canLeave = true
	inst.resources = NewResourceTable(inst)

	for _, module := range spec.Modules {
		inst.owned = append(inst.owned, module)
	}

	if spec.ParserInstanceType != nil {
		inst.wpInstance = spec.ParserInstanceType
	}

	if spec.ExternTable != nil {
		inst.externTable = spec.ExternTable
	}

	if spec.BuildExports != nil {
		exports, err := spec.BuildExports(inst)
		if err != nil {
			if closeErr := inst.Close(context.Background()); closeErr != nil {
				return nil, errors.Join(err, closeErr)
			}
			return nil, err
		}
		inst.initExports(exports)
	}
	return inst, nil
}

func (inst *ComponentInstance) initExports(exports []InstanceExport) {
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
			entry.typ = es.Type
		case InstanceKindInstance:
			entry.instance = es.Instance
		case InstanceKindCoreModule:
			entry.compiledModule = es.CompiledModule
		}
		inst.exports[es.Name] = entry
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
