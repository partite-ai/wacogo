// Package wacogo is a WebAssembly Component Model (MVP) runtime. It
// builds on wazero as the core wasm execution engine and adds
// component-model linking, loading, and instantiation on top.
//
// The entry point is Engine: construct one with NewEngine, load
// components from binaries with LoadComponent, call Instantiate on
// the returned Component to obtain a running ComponentInstance, and
// mint host-side components via NewHostBuilder.
package wacogo

import (
	"context"
	"io"

	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/core"
	"github.com/tetratelabs/wazero"
)

// Engine owns the wazero runtime and is the entry point for loading
// components, instantiating them, and building host-side components.
//
// Engine values are safe for concurrent use from multiple goroutines
// for Load/Instantiate; per-Engine state such as the validator arena
// is guarded by the runtime.
type Engine struct {
	core *core.Engine
}

// NewEngine creates a new Engine with the given options.
func NewEngine(ctx context.Context, opts ...EngineOption) *Engine {
	return &Engine{core: core.NewEngine(ctx, opts...)}
}

// Close releases all resources held by the engine, including the
// underlying wazero runtime. After Close, the engine cannot be used.
func (e *Engine) Close(ctx context.Context) error {
	return e.core.Close(ctx)
}

// LoadComponent parses, validates, and compiles a component-model
// binary read from r. The returned *Component is immutable and safe
// for concurrent Instantiate calls.
func (e *Engine) LoadComponent(ctx context.Context, r io.Reader) (*Component, error) {
	return e.core.LoadComponent(ctx, r)
}

// WazeroRuntime returns the underlying wazero runtime. Use it to
// register host modules or compile raw core modules that sit alongside
// the component-model machinery; the runtime is shared with all
// components loaded through this engine.
func (e *Engine) WazeroRuntime() wazero.Runtime {
	return core.WazeroRuntime(e.core)
}

// WrapCompiledModule lifts a wazero.CompiledModule into a *CompiledModule
// suitable for use as a core-module import (see WithModuleImport). The
// caller retains ownership of the underlying compiled module.
func WrapCompiledModule(cm wazero.CompiledModule) *CompiledModule {
	return core.WrapCompiledModule(cm)
}

// NewHostBuilder returns a fresh host.Builder bound to this engine.
// Register functions, types, and resources on the returned builder
// and call Build to produce a reusable *host.Component template.
// name is used in error messages and synthesized module names; keep
// it short and descriptive.
func (e *Engine) NewHostBuilder(name string) *host.Builder {
	return host.NewBuilder(e.core, name)
}
