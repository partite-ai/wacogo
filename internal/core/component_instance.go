package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/partite-ai/wacogo/internal/canon"
	"github.com/partite-ai/wacogo/wasmparser"
	"github.com/tetratelabs/wazero/api"
)

// ComponentInstance is a live instance of a Component.
// ComponentInstance is NOT safe for concurrent use from multiple goroutines.
// The component model defines components as single-threaded with no reentrance.
type ComponentInstance struct {
	engine        *Engine
	component     *Component
	coreInstances []api.Module
	// auxiliaryAdapters holds cross-component adapter instances (host module +
	// stub) created during instantiation. Closed in ComponentInstance.Close
	// alongside coreInstances.
	auxiliaryAdapters []*canon.CallAdapter
	exports           map[string]exportEntry
	// resources is the per-instance canonical-ABI resource table.
	// Populated during instantiation.
	resources *ResourceTable
	entered   bool
	canLeave  bool
	// poisoned is set by Poison when a trap leaves the instance in an
	// unreliable state. Non-nil makes Enter fail; nil means usable.
	poisoned error
	// types is the resolved type index space for this instance, populated by
	// planResolveType steps during instantiation.
	types []Type

	// parent points to the enclosing ComponentInstance (nil for the root).
	// Used by aliasResolver to walk up the scope chain.
	parent *ComponentInstance
	// instances is the component instance index space for this instance
	// (imported + locally-instantiated sub-instances, in declaration order).
	// Used by instanceImportResolver.
	instances []*ComponentInstance
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
	typeIdx        uint32          // populated only for SortType entries
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

// ExportedInstance returns the exported component instance with the given name, or nil if not found.
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
	if int(e.typeIdx) >= len(i.types) {
		return nil
	}
	return i.types[e.typeIdx]
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
func (i *ComponentInstance) Enter(ctx context.Context) (func(context.Context), error) {
	if i == nil {
		return func(context.Context) {}, nil
	}
	if i.poisoned != nil {
		return nil, fmt.Errorf("component instance unusable: %w", i.poisoned)
	}
	if i.entered {
		return nil, fmt.Errorf("cannot enter component instance (reentrance trap)")
	}
	i.entered = true
	return func(context.Context) { i.entered = false }, nil
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

// SuspendLeave clears the may_leave flag and returns a restorer
// closure that the caller MUST invoke exactly once (typically via
// defer) to put the prior value back. It is part of the canonical-ABI
// post-return protocol — invoked around the callee's post-return hook
// so that realloc and other leaving operations trap during teardown.
//
// Calling without a matching restore, or restoring twice, corrupts
// the runtime invariants. Most users should never call this directly.
func (i *ComponentInstance) SuspendLeave() func() {
	if i == nil {
		return func() {}
	}
	prev := i.canLeave
	i.canLeave = false
	return func() { i.canLeave = prev }
}

// RunInComponent runs fn with the instance entered, releasing on return
// regardless of whether fn errored.
func (i *ComponentInstance) RunInComponent(ctx context.Context, fn func() error) error {
	exit, err := i.Enter(ctx)
	if err != nil {
		return err
	}
	defer exit(ctx)
	return fn()
}

// Close releases all resources held by the component instance.
// Outstanding own-kind canonical-ABI handles are dropped first (running
// each registered destructor under recover so one bad dtor does not
// strand the rest) while the wazero runtime is still alive. Then the
// core wasm instances and any cross-component adapters are closed.
// The instance is finally marked poisoned so any further Enter call
// fails — Close is one-way. All errors are joined and returned;
// teardown always completes.
func (i *ComponentInstance) Close(ctx context.Context) error {
	var errs []error
	if i != nil && i.resources != nil {
		if err := i.resources.dropAllOwns(ctx, i); err != nil {
			errs = append(errs, err)
		}
	}
	for _, m := range i.coreInstances {
		if err := m.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	for _, a := range i.auxiliaryAdapters {
		if err := a.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if i != nil {
		i.Poison(fmt.Errorf("component instance closed"))
	}
	return errors.Join(errs...)
}

