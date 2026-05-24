package core

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo/internal/canon"
	"github.com/tetratelabs/wazero/api"
)

// ReallocFunc is the Go-level signature of the canonical-ABI realloc
// option. Construction sites that have a wazero api.Function wrap it
// once into this shape.
type ReallocFunc func(ctx context.Context, origPtr, origSize, align, newSize uint32) (uint32, error)

// Task is the per-call accounting record shared by the caller and
// callee CallContexts of one component-model call. Borrow tracking
// and post-call release closures live on it; ResourceHandle.LendTo
// uses it to register an unlend that fires when the call returns.
type Task = canon.Task

// CallContext is the canon-level per-call accessor. Holds an instance
// pointer, a *canon.Task for borrow accounting, and the memory + realloc
// in scope for arg lift/lower at this point in the call.
//
// Constructed via NewCallContext from inside the host package; never
// constructed by user code.
type CallContext struct {
	inst    *ComponentInstance
	task    *canon.Task
	memory  api.Memory
	realloc ReallocFunc
}

// NewCallContext returns a CallContext for the given instance, task,
// memory, and realloc. memory and realloc may be nil for canon-only
// contexts that touch neither (e.g., temporary CCs built solely to
// manage resource handles).
func NewCallContext(inst *ComponentInstance, task *canon.Task, memory api.Memory, realloc ReallocFunc) *CallContext {
	return &CallContext{inst: inst, task: task, memory: memory, realloc: realloc}
}

// Memory returns the memory in scope for arg lift/lower at this point
// in the call, or nil for canon-only contexts.
func (cc *CallContext) Memory() api.Memory { return cc.memory }

// Realloc invokes the realloc function in scope for this call. Errors
// when no realloc is bound (canon-only contexts or sides that never
// allocate).
func (cc *CallContext) Realloc(ctx context.Context, origPtr, origSize, align, newSize uint32) (uint32, error) {
	if cc.realloc == nil {
		return 0, fmt.Errorf("wacogo/core: Realloc unavailable: no realloc in scope")
	}
	return cc.realloc(ctx, origPtr, origSize, align, newSize)
}

// Instance returns the *ComponentInstance this context is bound to.
func (cc *CallContext) Instance() *ComponentInstance { return cc.inst }

// Task returns the *canon.Task this context shares with its peer side
// of the call. May be nil. Used by host-side closures that need to
// share lent state.
func (cc *CallContext) Task() *canon.Task { return cc.task }

// IssueOwnHandle allocates an own entry on this context's instance canon
// table and returns the live ResourceHandle. Use HandleID() on the result
// to obtain the uint32 wire value.
func (cc *CallContext) IssueOwnHandle(tr *TypeResource, rep uint32) ResourceHandle {
	return cc.inst.resources.IssueOwn(tr, rep)
}

// LookupOwn returns the ResourceHandle for an own entry in this
// context's resource table.
func (cc *CallContext) LookupOwn(tr *TypeResource, handle uint32) (ResourceHandle, error) {
	return cc.inst.resources.LookupOwn(tr, handle)
}

// LookupBorrowable returns the ResourceHandle for an own or borrow
// entry in this context's resource table.
func (cc *CallContext) LookupBorrowable(tr *TypeResource, handle uint32) (ResourceHandle, error) {
	return cc.inst.resources.LookupBorrowable(tr, handle)
}

// TransferTarget returns the call's resource table as an opaque
// TransferTarget. Pass it to ResourceHandle.LendTo or TransferOwn as
// the destination side; external callers cannot otherwise manipulate
// the underlying table.
func (cc *CallContext) TransferTarget() TransferTarget {
	return cc.inst.resources
}
