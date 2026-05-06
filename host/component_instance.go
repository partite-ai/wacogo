package host

import (
	"context"

	"github.com/partite-ai/wacogo/internal/core"
)

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
	core           *core.ComponentInstance
	extTable       *externTable
	userState      any
	cleanups       []func()
	preCloseChecks []func() error
}

// Core returns the underlying *wacogo.ComponentInstance, suitable for
// passing to wacogo.WithInstanceImport when wiring this host component
// as an import.
func (h *ComponentInstance) Core() *core.ComponentInstance { return h.core }

// RegisterResource records obj in this instance and returns a fresh
// ExternHandle. Use the returned handle as the rep value when
// minting an own<R> for a host-defined resource.
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

// Close releases the resources held by this instance. If any resource
// handles minted by this instance are still outstanding, Close returns
// an error; teardown completes regardless.
func (h *ComponentInstance) Close(ctx context.Context) error {
	var firstErr error
	for _, fn := range h.preCloseChecks {
		if err := fn(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if h.core != nil {
		if err := h.core.Close(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for _, fn := range h.cleanups {
		fn()
	}
	return firstErr
}

// releaseResource removes the entry at eh and returns the stored obj
// plus true on success. Returns (nil, false) if eh is 0, out-of-range,
// or already free.
func (h *ComponentInstance) releaseResource(eh ExternHandle) (any, bool) {
	return h.extTable.release(eh)
}
