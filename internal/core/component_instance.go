package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/partite-ai/wacogo/wasmparser"
)

// ComponentInstance is a live instance of a Component.
// ComponentInstance is NOT safe for concurrent use from multiple goroutines.
// The component model defines components as single-threaded with no reentrance.
type ComponentInstance struct {
	engine    *Engine
	component *Component
	// owned records only newly instantiated modules, adapters and components,
	// in creation order. Imported instances and aliases remain borrowed.
	owned   []instanceCloser
	closed  bool
	exports map[string]exportEntry
	// resources is the per-instance canonical-ABI resource table.
	// Populated during instantiation.
	resources *ResourceTable
	entered   bool
	// activeOperations and refundPending are guarded by engine.lifecycleMu.
	activeOperations uint32
	refundPending    bool
	canLeave         bool
	// poisoned is set by Poison when a trap leaves the instance in an
	// unreliable state. Non-nil makes Enter fail; nil means usable.
	poisoned error

	// wpInstance is the opaque wasmparser instance-type handle minted at
	// creation time (fresh ResourceIDs for any resources this instance
	// defines). Nil for sub-component instances and for instances whose
	// originating Component had no type handle. Used when this instance is
	// supplied as an instance import to another component's Instantiate call.
	wpInstance *wasmparser.InstanceType

	// externTable, when non-nil, supplies LookupExtern. Set from
	// InstanceSpec.ExternTable at construction time.
	externTable ExternTable
}

type instanceCloser interface {
	Close(context.Context) error
}

// LookupExtern returns the Go object registered at rep in this
// instance's extern table, or (nil, false) if no extern table was
// supplied at construction time or rep is not registered.
func (i *ComponentInstance) LookupExtern(rep uint32) (any, bool) {
	if i.externTable == nil {
		return nil, false
	}
	return i.externTable.Lookup(rep)
}

type exportEntry struct {
	kind           Sort
	funcVal        *ExportedFunc
	instance       *ComponentInstance
	compiledModule *CompiledModule // for SortCoreModule exports
	component      *Component      // for SortComponent exports
	typ            Type            // populated only for SortType entries
}

// ParserInstanceType returns the wasmparser instance-type handle for
// this instance, or nil if none was minted. Use this to query extended
// type information about the instance (e.g. exported resource type IDs)
// beyond what the runtime API exposes directly.
func (i *ComponentInstance) ParserInstanceType() *wasmparser.InstanceType {
	return i.wpInstance
}

// ExportedFunc returns the exported function with the given name, or nil if not found.
func (i *ComponentInstance) ExportedFunc(name string) *ExportedFunc {
	e, ok := i.exports[name]
	if !ok || e.kind != SortFunc {
		return nil
	}
	return e.funcVal
}

// ExportedInstance returns the exported component instance with the given name,
// or nil if not found. The result is borrowed: retaining it or importing it into
// another component does not extend its lifetime. Closing its owning parent
// closes it. Re-exporting a borrowed instance does not transfer ownership; its
// original owner still controls its lifetime.
func (i *ComponentInstance) ExportedInstance(name string) *ComponentInstance {
	e, ok := i.exports[name]
	if !ok || e.kind != SortInstance {
		return nil
	}
	return e.instance
}

// ExportedModule returns the exported core module with the given name, or nil if not found.
func (i *ComponentInstance) ExportedModule(name string) *CompiledModule {
	e, ok := i.exports[name]
	if !ok || e.kind != SortCoreModule {
		return nil
	}
	return e.compiledModule
}

// ExportedComponent returns the exported component with the given name, or nil if not found.
func (i *ComponentInstance) ExportedComponent(name string) *Component {
	e, ok := i.exports[name]
	if !ok || e.kind != SortComponent {
		return nil
	}
	return e.component
}

// exportedType returns the Type exported under the given name, or nil if the
// export is missing or is not a type export. Unlinkable component pairs
// surface here as missing exports and must be reported as normal
// instantiation errors, not runtime panics.
func (i *ComponentInstance) exportedType(name string) Type {
	e, ok := i.exports[name]
	if !ok || e.kind != SortType {
		return nil
	}
	return e.typ
}

// ExportedType returns the Type exported under the given name, or nil
// if the export is missing or is not a type export. Used by the host
// layer to resolve cross-component resource type references.
func (i *ComponentInstance) ExportedType(name string) Type {
	return i.exportedType(name)
}

// Enter acquires the instance's reentrance lock for the duration of a
// component-model call. It returns an exit closure that the caller MUST
// invoke exactly once (typically via defer) to release the lock; failure
// to do so leaves the instance permanently inaccessible. If the instance
// is already entered, Enter returns a "reentrance trap" error per the
// component-model spec. If the instance was previously poisoned by a
// trap, Enter returns an error wrapping the original trap reason.
//
// Most users should never call this directly: the runtime invokes Enter
// around every cross-component call. Misuse — calling without a matching
// exit, or calling concurrently from another goroutine — corrupts the
// runtime's invariants and can cause deadlocks or trap leaks. Touch this
// only when implementing a low-level call path that the canonical-ABI
// runners do not already cover.
func (i *ComponentInstance) Enter(ctx context.Context) error {
	if i == nil {
		return nil
	}
	if i.poisoned != nil {
		return fmt.Errorf("component instance unusable: %w", i.poisoned)
	}
	if i.entered {
		return fmt.Errorf("cannot enter component instance (reentrance trap)")
	}
	i.entered = true
	i.beginOperation()
	return nil
}

// Exit releases the reentrance lock acquired by Enter. Idempotent on a nil
// receiver. The caller MUST NOT call Exit unless a matching Enter succeeded.
func (i *ComponentInstance) Exit(_ context.Context) {
	original := recover()
	defer finishTableCleanup(nil, i.finishCall)
	if original != nil {
		panic(original)
	}
}

func (i *ComponentInstance) finishCall() error {
	if i == nil {
		return nil
	}
	i.entered = false
	return i.endOperation()
}

// Poison marks the instance as unusable due to a trap. Subsequent Enter
// calls return an error wrapping reason. Idempotent — only the first
// reason is retained. The canonical-ABI runners call this from their
// outermost recover-on-trap defer; user code can also call it directly
// when an external failure (e.g. a Go-side resource error) makes the
// instance's state unreliable.
func (i *ComponentInstance) Poison(reason error) {
	if i == nil || i.poisoned != nil {
		return
	}
	if reason == nil {
		reason = fmt.Errorf("trap")
	}
	i.poisoned = reason
}

// Poisoned returns the trap reason that poisoned this instance, or nil
// when the instance is still usable.
func (i *ComponentInstance) Poisoned() error {
	if i == nil {
		return nil
	}
	return i.poisoned
}

// CanLeave reports whether the instance is currently in a state that
// permits a call to leave its boundary (the canonical-ABI may_leave
// flag). The runtime consults this at every lower point to decide
// whether allocations such as realloc are legal. Most users should
// never call this: it is exposed only for low-level call paths that
// build their own lift/lower steps.
func (i *ComponentInstance) CanLeave() bool {
	if i == nil {
		return true
	}
	return i.canLeave
}

// SuspendLeave clears the may_leave flag and returns the prior value. The
// caller MUST pass that prior value to RestoreLeave exactly once (typically
// via defer) to put it back. It is part of the canonical-ABI post-return
// protocol — invoked around the callee's post-return hook so that realloc
// and other leaving operations trap during teardown.
//
// Calling without a matching RestoreLeave, or restoring twice, corrupts the
// runtime invariants. Most users should never call this directly.
func (i *ComponentInstance) SuspendLeave() (prev bool) {
	if i == nil {
		return true
	}
	prev = i.canLeave
	i.canLeave = false
	return prev
}

// RestoreLeave restores the may_leave flag to prev. Idempotent on a nil
// receiver.
func (i *ComponentInstance) RestoreLeave(prev bool) {
	if i == nil {
		return
	}
	i.canLeave = prev
}

// RunInComponent runs fn with the instance entered, releasing on return
// regardless of whether fn errored.
func (i *ComponentInstance) RunInComponent(ctx context.Context, fn func() error) (err error) {
	if err := i.Enter(ctx); err != nil {
		return err
	}
	defer finishTableCleanup(&err, i.finishCall)
	return fn()
}

// Close releases all resources held by the component and its owned descendants.
// Resource destructors run to quiescence before any owned modules close, so
// callbacks may use providers in the closing subtree and acquire new resources.
// Handles are retired before destruction; repeated and reentrant Close calls
// are no-ops. Modules and adapters then close in reverse ownership order.
// Errors are joined, and individual destructor failures do not stop cleanup.
func (i *ComponentInstance) Close(ctx context.Context) (err error) {
	if i == nil || i.closed {
		return nil
	}
	var teardown instanceTeardown
	teardown.collect(i)
	for _, instance := range teardown.instances {
		instance.beginOperation()
		defer finishTableCleanup(&err, instance.endOperation)
	}
	var errs []error
	for {
		dropped := false
		for _, instance := range teardown.instances {
			more, err := instance.resources.dropAllOwns(ctx, instance)
			dropped = dropped || more
			if err != nil {
				errs = append(errs, err)
			}
		}
		if !dropped {
			break
		}
	}
	for _, instance := range teardown.instances {
		instance.Poison(fmt.Errorf("component instance closed"))
	}
	for _, owned := range teardown.owned {
		if err := owned.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// instanceTeardown separates resource destruction from physical teardown across
// the complete owned subtree. Its flattened closer list preserves the original
// reverse creation order without closing child providers during resource drains.
type instanceTeardown struct {
	instances []*ComponentInstance
	owned     []instanceCloser
}

func (t *instanceTeardown) collect(i *ComponentInstance) {
	if i == nil || i.closed {
		return
	}
	i.closed = true
	t.instances = append(t.instances, i)
	owned := i.owned
	i.owned = nil
	for idx := len(owned) - 1; idx >= 0; idx-- {
		if child, ok := owned[idx].(*ComponentInstance); ok {
			t.collect(child)
		} else {
			t.owned = append(t.owned, owned[idx])
		}
	}
}
