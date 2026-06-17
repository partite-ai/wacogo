package canon

import (
	"context"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// CallAdapter is the factory return: a core func (Module/Name) plus any
// auxiliary modules that must be torn down at Close. Close is idempotent.
type CallAdapter struct {
	Module api.Module
	Name   string

	// aux holds host module + stub instances plus any other modules this
	// adapter owns. Teardown order is reverse of append order.
	aux []api.Module

	// compiled holds per-adapter compiled code (the host module's) that
	// must be freed after the module instances referencing it are closed.
	compiled []wazero.CompiledModule

	closed bool
}

// Close releases the adapter's owned modules. Safe to call multiple times;
// subsequent calls are no-ops returning nil.
func (a *CallAdapter) Close(ctx context.Context) error {
	if a == nil || a.closed {
		return nil
	}
	a.closed = true
	var firstErr error
	for i := len(a.aux) - 1; i >= 0; i-- {
		if err := a.aux[i].Close(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if a.Module != nil {
		if err := a.Module.Close(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	// Compiled code is freed only after every instance referencing it is
	// closed above.
	for _, cm := range a.compiled {
		if err := cm.Close(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
