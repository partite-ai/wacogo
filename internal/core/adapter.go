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
	if step.options.hasMemory {
		if int(step.options.memory) >= len(s.coreMemories) {
			return fmt.Errorf("wacogo: lower: memory option index %d out of range", step.options.memory)
		}
		cm := s.coreMemories[step.options.memory]
		callerMem = cm.instance.ExportedMemory(cm.name)
		if callerMem == nil {
			return fmt.Errorf("wacogo: lower: memory %q not exported by module", cm.name)
		}
	}

	var callerRealloc api.Function
	var callerReallocModName, callerReallocFnExport string
	if step.options.hasRealloc {
		if int(step.options.realloc) >= len(s.coreFuncs) {
			return fmt.Errorf("wacogo: lower: realloc option index %d out of range", step.options.realloc)
		}
		rf := s.coreFuncs[step.options.realloc]
		callerRealloc = rf.instance.ExportedFunction(rf.name)
		if callerRealloc == nil {
			return fmt.Errorf("wacogo: lower: realloc %q not exported by module", rf.name)
		}
		callerReallocModName = rf.instance.Name()
		callerReallocFnExport = rf.name
	}

	caller := canon.CallSide{
		Instance:        canonInstanceView{i: s.inst},
		Memory:          callerMem,
		Realloc:         callerRealloc,
		StringEncoding:  step.options.stringEncoding,
		ReallocModName:  callerReallocModName,
		ReallocFnExport: callerReallocFnExport,
	}

	callee := fn.binding.Callee()

	adapter, err := s.inst.engine.canonHost.BuildAdapter(ctx,
		caller, callee,
		FuncTypeParamsAsCanon(fn.funcType),
		FuncTypeResultsAsCanon(fn.funcType))
	if err != nil {
		return fmt.Errorf("wacogo: lower adapter: %w", err)
	}
	return s.bindHostFunc(step.hostFuncIndex, adapter)
}

// execResourceNew creates a core function for canon resource.new.
// Signature: (rep: i32) -> (handle: i32)
func (step *planResourceNew) execute(_ context.Context, s *instantiationState) error {
	rtype := lookupResourceType(s, step.typeID)
	return s.bindHostFunc(step.hostFuncIndex, s.inst.engine.canonHost.BuildResourceNew(canonInstanceView{i: s.inst}, resourceType{rt: rtype}))
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
func (step *planResourceDrop) execute(_ context.Context, s *instantiationState) error {
	rtype := lookupResourceType(s, step.typeID)
	return s.bindHostFunc(step.hostFuncIndex, s.inst.engine.canonHost.BuildResourceDrop(canonInstanceView{i: s.inst}, resourceType{rt: rtype}))
}

// execResourceRep creates a core function for canon resource.rep.
// Signature: (handle: i32) -> (rep: i32)
func (step *planResourceRep) execute(_ context.Context, s *instantiationState) error {
	rtype := lookupResourceType(s, step.typeID)
	return s.bindHostFunc(step.hostFuncIndex, s.inst.engine.canonHost.BuildResourceRep(canonInstanceView{i: s.inst}, resourceType{rt: rtype}))
}
