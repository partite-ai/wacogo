package canon

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// CallAdapter is the factory return: a core function implemented in Go, with its
// signature, plus any batched-realloc helpers that must be torn down at Close.
// Close is idempotent.
type CallAdapter struct {
	Func            api.GoModuleFunction
	Params, Results []api.ValueType

	// reallocHelpers holds the batched-realloc helpers this adapter owns.
	// Teardown order is reverse of append order.
	reallocHelpers []*batchReallocHelper

	closed bool
}

// Close releases the adapter's realloc helpers. Safe to call multiple times;
// subsequent calls are no-ops returning nil.
func (a *CallAdapter) Close(ctx context.Context) error {
	if a == nil || a.closed {
		return nil
	}
	a.closed = true
	var firstErr error
	for i := len(a.reallocHelpers) - 1; i >= 0; i-- {
		if err := a.reallocHelpers[i].Close(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
