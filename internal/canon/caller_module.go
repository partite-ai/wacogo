package canon

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// callerCoreModuleKey is the unexported key used to attach the caller's
// core wasm module to a context for the duration of a cross-component
// call.
type callerCoreModuleKey struct{}

// WithCallerCoreModule returns ctx with mod attached as the caller's
// core wasm module for an in-flight cross-component call. The host
// package surfaces this to user code so that host-function callbacks
// can recover the wasm caller from their context.
func WithCallerCoreModule(ctx context.Context, mod api.Module) context.Context {
	return context.WithValue(ctx, callerCoreModuleKey{}, mod)
}

// CallerCoreModule returns the caller's core wasm module previously
// attached via WithCallerCoreModule, or nil when no cross-component
// call is in flight or the caller had no wasm module.
func CallerCoreModule(ctx context.Context) api.Module {
	m, _ := ctx.Value(callerCoreModuleKey{}).(api.Module)
	return m
}
