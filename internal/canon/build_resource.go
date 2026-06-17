package canon

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

var resourceCounter atomic.Uint64

// BuildResourceNew constructs a wasm stub module whose exported "f" implements
// canonical-ABI resource.new for inst's table and the given resource type.
// Signature: (rep: i32) -> (handle: i32).
func (h *Host) BuildResourceNew(
	ctx context.Context,
	baseName string,
	inst Instance,
	rt ResourceType,
) (*CallAdapter, error) {
	tbl := inst.ResourceTable()
	fn := api.GoModuleFunc(func(_ context.Context, _ api.Module, stack []uint64) {
		if !inst.CanLeave() {
			trapf("resource.new: may_leave violation")
		}
		rep := uint32(stack[0])
		dst := tbl.IssueOwn(rt, rep)
		stack[0] = uint64(dst.HandleID())
	})
	return h.buildResourceStub(ctx, baseName, "resource.new", fn,
		[]api.ValueType{api.ValueTypeI32},
		[]api.ValueType{api.ValueTypeI32})
}

// BuildResourceDrop constructs a wasm stub implementing resource.drop.
// The destructor is reached via rt.Destructor() and orchestrated by
// ResourceTable.Drop — same-component shortcut, defining-instance
// Enter/Exit, and reentrance gating all live in canon. The wasm
// caller is the resource's defining instance only when inst itself
// declared rt; for cross-component drops inst is the importer's
// instance and the same-component shortcut does not fire.
// Signature: (handle: i32) -> ().
func (h *Host) BuildResourceDrop(
	ctx context.Context,
	baseName string,
	inst Instance,
	rt ResourceType,
) (*CallAdapter, error) {
	tbl := inst.ResourceTable()
	fn := api.GoModuleFunc(func(callCtx context.Context, _ api.Module, stack []uint64) {
		if !inst.CanLeave() {
			trapf("resource.drop: may_leave violation")
		}
		handle := uint32(stack[0])
		h, err := tbl.LookupBorrowable(rt, handle)
		if err != nil {
			trapf("resource.drop: %v", err)
		}
		if err := h.Drop(callCtx); err != nil {
			trapf("resource.drop: %v", err)
		}
	})
	return h.buildResourceStub(ctx, baseName, "resource.drop", fn,
		[]api.ValueType{api.ValueTypeI32}, nil)
}

// BuildResourceRep constructs a wasm stub implementing resource.rep.
// Signature: (handle: i32) -> (rep: i32).
//
// Per the canonical-ABI spec (canon_resource_rep in definitions.py),
// resource.rep does NOT trap on may_leave — unlike resource.new and
// resource.drop. It is purely a handle lookup.
func (h *Host) BuildResourceRep(
	ctx context.Context,
	baseName string,
	inst Instance,
	rt ResourceType,
) (*CallAdapter, error) {
	tbl := inst.ResourceTable()
	fn := api.GoModuleFunc(func(_ context.Context, _ api.Module, stack []uint64) {
		handle := uint32(stack[0])
		h, err := tbl.LookupOwn(rt, handle)
		if err != nil {
			trapf("resource.rep: %v", err)
		}
		stack[0] = uint64(h.Rep())
	})
	return h.buildResourceStub(ctx, baseName, "resource.rep", fn,
		[]api.ValueType{api.ValueTypeI32},
		[]api.ValueType{api.ValueTypeI32})
}

func (h *Host) buildResourceStub(
	ctx context.Context,
	baseName, opName string,
	fn api.GoModuleFunction,
	paramTypes, resultTypes []api.ValueType,
) (*CallAdapter, error) {
	n := resourceCounter.Add(1)
	hostModName := fmt.Sprintf("%s_host_%d", baseName, n)
	// Compile + instantiate in two steps so the host instance is a raw
	// *ModuleInstance usable directly as the core function via wazero's
	// import resolver.
	hostCompiled, err := h.runtime.NewHostModuleBuilder(hostModName).
		NewFunctionBuilder().
		WithGoModuleFunction(fn, paramTypes, resultTypes).
		Export("f").
		Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("canon: %s: compile host: %w", opName, err)
	}
	hostMod, err := h.runtime.InstantiateModule(ctx, hostCompiled,
		wazero.NewModuleConfig().WithName(""))
	if err != nil {
		_ = hostCompiled.Close(ctx)
		return nil, fmt.Errorf("canon: %s: instantiate host: %w", opName, err)
	}
	return &CallAdapter{
		Module:   hostMod,
		Name:     "f",
		compiled: []wazero.CompiledModule{hostCompiled},
	}, nil
}
