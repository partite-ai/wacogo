package core

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/partite-ai/wacogo/internal/canon"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// maxHostFuncsPerModule is the most functions wazero allows in a host module.
const maxHostFuncsPerModule = 1 << 16

// hostFuncSig describes a function a component's canon section creates: a lowered
// function or a resource builtin. name is what stack traces call it.
type hostFuncSig struct {
	name            string
	params, results []api.ValueType
}

// hostFuncSlot is one function of an instantiation's host modules. The modules are
// built before the plan runs, while what a function calls can depend on what
// earlier steps create, so its plan step fills this in. Nothing can call a slot
// before then, as nothing has been given it to import.
type hostFuncSlot struct {
	fn api.GoModuleFunction
}

func (s *hostFuncSlot) Call(ctx context.Context, mod api.Module, stack []uint64) {
	s.fn.Call(ctx, mod, stack)
}

// hostFuncs is the Go-implemented core functions of one instantiation. Compiling
// a host module is expensive however little is in it, so they share as few modules
// as wazero allows.
type hostFuncs struct {
	sigs    []hostFuncSig
	slots   []hostFuncSlot
	modules []api.Module
}

// hostFuncExport is the name a host function is linked by. Names shown in traces
// needn't be unique, so they can't be used for this.
func hostFuncExport(index int) string { return "f" + strconv.Itoa(index) }

// newHostFuncs builds the modules for sigs, owned by inst.
func newHostFuncs(ctx context.Context, rt wazero.Runtime, inst *ComponentInstance, sigs []hostFuncSig) (*hostFuncs, error) {
	h := &hostFuncs{sigs: sigs, slots: make([]hostFuncSlot, len(sigs))}
	for start := 0; start < len(sigs); start += maxHostFuncsPerModule {
		end := min(start+maxHostFuncsPerModule, len(sigs))
		b := rt.NewHostModuleBuilder("wacogo_host_funcs")
		for i := start; i < end; i++ {
			b.NewFunctionBuilder().
				WithGoModuleFunction(&h.slots[i], sigs[i].params, sigs[i].results).
				WithName(sigs[i].name).
				Export(hostFuncExport(i))
		}
		compiled, err := b.Compile(ctx)
		if err != nil {
			return nil, fmt.Errorf("wacogo: compile host functions: %w", err)
		}
		// Owned before the instance, as owned modules close in reverse order.
		inst.owned = append(inst.owned, compiled)
		// Compile and instantiate in two steps (rather than the builder's
		// Instantiate) to get a raw module instance: the wrapper Instantiate returns
		// fails the *ModuleInstance type assertion in wazero's import resolver.
		mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(""))
		if err != nil {
			return nil, fmt.Errorf("wacogo: instantiate host functions: %w", err)
		}
		inst.owned = append(inst.owned, mod)
		h.modules = append(h.modules, mod)
	}
	return h, nil
}

// bindHostFunc makes adapter the host function at index and adds it to the core
// func index space.
func (s *instantiationState) bindHostFunc(index int, adapter *canon.CallAdapter) error {
	h := s.hostFuncs
	// The validator worked out the signature the module was built with, from the
	// function type this component declares. Calling through a different one would
	// misread the stack.
	sig := h.sigs[index]
	if !slices.Equal(sig.params, adapter.Params) || !slices.Equal(sig.results, adapter.Results) {
		return fmt.Errorf("wacogo: core function signature (%v) -> (%v) does not match the (%v) -> (%v) validated",
			adapter.Params, adapter.Results, sig.params, sig.results)
	}
	h.slots[index].fn = adapter.Func
	s.inst.owned = append(s.inst.owned, adapter)
	s.coreFuncs = append(s.coreFuncs, coreFunc{
		instance: h.modules[index/maxHostFuncsPerModule],
		name:     hostFuncExport(index),
	})
	return nil
}
