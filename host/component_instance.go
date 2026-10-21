package host

import (
	"context"
	"errors"
	"fmt"

	"github.com/partite-ai/wacogo/internal/core"
)

// DestroyOrphan, when implemented by a value stored in the extern table,
// is invoked in preference to Drop while Close drains the extern table.
// Use it when orphan cleanup at instance shutdown should diverge from
// the normal release path — for example, when the wasm side is already
// torn down and the normal Drop would attempt to communicate with it.
type DestroyOrphan interface {
	DestroyOrphan(ctx context.Context) error
}

// Drop, when implemented by a value stored in the extern table, is
// invoked during Close drain on entries that survived normal teardown
// and do not implement DestroyOrphan. Matches the signature witgen-
// emitted resourceDtors call on guest-driven resource.drop, so one
// Drop method covers both cleanup paths.
type Drop interface {
	Drop(ctx context.Context) error
}

// ExternHandle identifies a Go object registered with a
// ComponentInstance via RegisterResource. Handles are 1-indexed; the
// zero value is reserved as "invalid".
type ExternHandle uint32

// ComponentInstance is a live host component. Obtain one from
// Component.Instantiate.
//
// A ComponentInstance is not safe for concurrent use from multiple
// goroutines.
type ComponentInstance struct {
	core         *core.ComponentInstance
	extTable     *externTable
	userState    any
	callListener CallListener
	cleanups     []func()
	hostCallCC   *core.CallContext
}

// Core returns the underlying *wacogo.ComponentInstance, suitable for
// passing to wacogo.WithInstanceImport when wiring this host component
// as an import.
func (h *ComponentInstance) Core() *core.ComponentInstance { return h.core }

// RegisterResource records obj in this instance and returns a fresh
// ExternHandle. Pass the exported resource type and the returned handle as
// the rep value to wacogo.NewValOwnHandle when minting an own<R> for a
// host-defined resource.
func (h *ComponentInstance) RegisterResource(obj any) ExternHandle {
	return h.extTable.put(obj)
}

// LookupResource returns the object previously stored under eh and
// true on success, or (nil, false) if eh is zero, out of range, or
// has already been released.
func (h *ComponentInstance) LookupResource(eh ExternHandle) (any, bool) {
	return h.extTable.lookup(eh)
}

// UserState returns the value attached at Instantiate time via
// WithUserState. UserState is set once and is immutable.
func (h *ComponentInstance) UserState() any { return h.userState }

// Close releases the resources held by this instance. The core
// instance closes first — that pass drops any outstanding own-kind
// canon handles via their registered destructors before the wazero
// runtime tears down. User cleanups run next, then the extern table
// is drained in LIFO insertion order: each entry's DestroyOrphan is
// called when implemented, otherwise Drop. All errors are joined and
// returned; teardown always completes.
func (h *ComponentInstance) Close(ctx context.Context) error {
	var errs []error
	if h.core != nil {
		if err := h.core.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	for _, fn := range h.cleanups {
		fn()
	}
	if err := h.drainExternTable(ctx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// drainExternTable walks the extern table from highest ExternHandle
// down (LIFO of insertion). For each live entry, DestroyOrphan is
// preferred over Drop; entries that implement neither are released
// without invoking cleanup. Errors are collected and joined.
func (h *ComponentInstance) drainExternTable(ctx context.Context) error {
	if h.extTable == nil || h.extTable.liveCount() == 0 {
		return nil
	}
	var errs []error
	for i := len(h.extTable.entries) - 1; i >= 0; i-- {
		if !h.extTable.entries[i].occupied {
			continue
		}
		eh := ExternHandle(uint32(i) + 1)
		obj, ok := h.extTable.lookup(eh)
		if !ok {
			continue
		}
		switch v := obj.(type) {
		case DestroyOrphan:
			if err := v.DestroyOrphan(ctx); err != nil {
				errs = append(errs, fmt.Errorf("DestroyOrphan(handle=%d): %w", eh, err))
			}
		case Drop:
			if err := v.Drop(ctx); err != nil {
				errs = append(errs, fmt.Errorf("Drop(handle=%d): %w", eh, err))
			}
		}
		h.extTable.release(eh)
	}
	return errors.Join(errs...)
}

// releaseResource removes the entry at eh and returns the stored obj
// plus true on success. Returns (nil, false) if eh is 0, out-of-range,
// or already free.
func (h *ComponentInstance) releaseResource(eh ExternHandle) (any, bool) {
	return h.extTable.release(eh)
}
