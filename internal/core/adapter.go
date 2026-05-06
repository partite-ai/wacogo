package core

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo/internal/canon"
	"github.com/tetratelabs/wazero/api"
)

// execLowerWithGoAdapter creates a cross-component adapter via canon.Host.BuildAdapter.
// The adapter bridges a call from the caller's core ABI into the callee's core ABI
// using a pre-compiled transfer plan. Caller options come from the lower step;
// the callee side comes from fn.Callee().
func (s *instantiationState) execLowerWithGoAdapter(ctx context.Context, step planLower, fn *ExportedFunc) error {
	var callerMem api.Memory
	if step.options.hasMemory && int(step.options.memory) < len(s.coreMemories) {
		cm := s.coreMemories[step.options.memory]
		callerMem = cm.instance.ExportedMemory(cm.name)
	}

	var callerRealloc api.Function
	if step.options.hasRealloc && int(step.options.realloc) < len(s.coreFuncs) {
		rf := s.coreFuncs[step.options.realloc]
		callerRealloc = rf.instance.ExportedFunction(rf.name)
	}

	caller := canon.CallSide{
		Instance:       canonInstanceView{i: s.inst},
		Memory:         callerMem,
		Realloc:        callerRealloc,
		StringEncoding: step.options.stringEncoding,
	}

	callee := fn.binding.Callee()

	var baseName string
	if fn.Name != "" {
		baseName = fmt.Sprintf("wacogo_lower_%s", sanitizeName(fn.Name))
	} else {
		baseName = fmt.Sprintf("wacogo_lower_%d", instanceCounter.Add(1))
	}
	adapter, err := s.inst.engine.canonHost.BuildAdapter(ctx, baseName,
		caller, callee,
		FuncTypeParamsAsCanon(fn.funcType),
		FuncTypeResultsAsCanon(fn.funcType))
	if err != nil {
		return fmt.Errorf("wacogo: lower adapter: %w", err)
	}

	s.auxiliaryAdapters = append(s.auxiliaryAdapters, adapter)
	s.coreFuncs = append(s.coreFuncs, coreFunc{
		instance: adapter.Module,
		name:     adapter.Name,
	})
	return nil
}

// sanitizeName replaces characters unsuitable for wasm module names with '_'.
func sanitizeName(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			out = append(out, c)
			continue
		}
		out = append(out, '_')
	}
	return string(out)
}

// execResourceNew creates a core function for canon resource.new.
// Signature: (rep: i32) -> (handle: i32)
func (step *planResourceNew) execute(ctx context.Context, s *instantiationState) error {
	rtype := lookupResourceType(s, step.typeID)
	baseName := fmt.Sprintf("wacogo_resnew_%d", instanceCounter.Add(1))
	adapter, err := s.inst.engine.canonHost.BuildResourceNew(ctx, baseName,
		canonInstanceView{i: s.inst}, resourceType{rt: rtype})
	if err != nil {
		return fmt.Errorf("wacogo: resource.new: %w", err)
	}
	s.auxiliaryAdapters = append(s.auxiliaryAdapters, adapter)
	s.coreFuncs = append(s.coreFuncs, coreFunc{
		instance: adapter.Module,
		name:     adapter.Name,
	})
	return nil
}

// execResourceDrop creates a core function for canon resource.drop.
// Signature: (handle: i32) -> ()
//
// The destructor is no longer threaded through the BuildResourceDrop
// signature — canon's ResourceTable.Drop reaches it via
// ResourceType.Destructor() which forwards to *TypeResource.dtor.
// step.dtorFuncIndex still influences load-time core-func wiring (the
// dtor api.Function is registered as a regular core function for
// alias resolution etc.), but canon no longer consumes it directly
// from this code path.
func (step *planResourceDrop) execute(ctx context.Context, s *instantiationState) error {
	rtype := lookupResourceType(s, step.typeID)

	baseName := fmt.Sprintf("wacogo_resdrop_%d", instanceCounter.Add(1))
	adapter, err := s.inst.engine.canonHost.BuildResourceDrop(ctx, baseName,
		canonInstanceView{i: s.inst}, resourceType{rt: rtype})
	if err != nil {
		return fmt.Errorf("wacogo: resource.drop: %w", err)
	}
	s.auxiliaryAdapters = append(s.auxiliaryAdapters, adapter)
	s.coreFuncs = append(s.coreFuncs, coreFunc{
		instance: adapter.Module,
		name:     adapter.Name,
	})
	return nil
}

// execResourceRep creates a core function for canon resource.rep.
// Signature: (handle: i32) -> (rep: i32)
func (step *planResourceRep) execute(ctx context.Context, s *instantiationState) error {
	rtype := lookupResourceType(s, step.typeID)
	baseName := fmt.Sprintf("wacogo_resrep_%d", instanceCounter.Add(1))
	adapter, err := s.inst.engine.canonHost.BuildResourceRep(ctx, baseName,
		canonInstanceView{i: s.inst}, resourceType{rt: rtype})
	if err != nil {
		return fmt.Errorf("wacogo: resource.rep: %w", err)
	}
	s.auxiliaryAdapters = append(s.auxiliaryAdapters, adapter)
	s.coreFuncs = append(s.coreFuncs, coreFunc{
		instance: adapter.Module,
		name:     adapter.Name,
	})
	return nil
}
