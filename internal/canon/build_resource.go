package canon

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// BuildResourceNew constructs the Go function implementing
// canonical-ABI resource.new for inst's table and the given resource type.
// Signature: (rep: i32) -> (handle: i32).
func (h *Host) BuildResourceNew(inst Instance, rt ResourceType) *CallAdapter {
	tbl := inst.ResourceTable()
	fn := api.GoModuleFunc(func(_ context.Context, _ api.Module, stack []uint64) {
		if !inst.CanLeave() {
			trapf("resource.new: may_leave violation")
		}
		rep := uint32(stack[0])
		dst := tbl.IssueOwn(rt, rep)
		stack[0] = uint64(dst.HandleID())
	})
	return &CallAdapter{Func: fn, Params: []api.ValueType{api.ValueTypeI32}, Results: []api.ValueType{api.ValueTypeI32}}
}

// BuildResourceDrop constructs the Go function implementing resource.drop.
// The destructor is reached via rt.Destructor() and orchestrated by
// ResourceTable.Drop — same-component shortcut, defining-instance
// Enter/Exit, and reentrance gating all live in canon. The wasm
// caller is the resource's defining instance only when inst itself
// declared rt; for cross-component drops inst is the importer's
// instance and the same-component shortcut does not fire.
// Signature: (handle: i32) -> ().
func (h *Host) BuildResourceDrop(inst Instance, rt ResourceType) *CallAdapter {
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
	return &CallAdapter{Func: fn, Params: []api.ValueType{api.ValueTypeI32}}
}

// BuildResourceRep constructs the Go function implementing resource.rep.
// Signature: (handle: i32) -> (rep: i32).
//
// Per the canonical-ABI spec (canon_resource_rep in definitions.py),
// resource.rep does NOT trap on may_leave — unlike resource.new and
// resource.drop. It is purely a handle lookup.
func (h *Host) BuildResourceRep(inst Instance, rt ResourceType) *CallAdapter {
	tbl := inst.ResourceTable()
	fn := api.GoModuleFunc(func(_ context.Context, _ api.Module, stack []uint64) {
		handle := uint32(stack[0])
		h, err := tbl.LookupOwn(rt, handle)
		if err != nil {
			trapf("resource.rep: %v", err)
		}
		stack[0] = uint64(h.Rep())
	})
	return &CallAdapter{Func: fn, Params: []api.ValueType{api.ValueTypeI32}, Results: []api.ValueType{api.ValueTypeI32}}
}
