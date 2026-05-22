package core

import (
	"context"

	"github.com/partite-ai/wacogo/internal/canon"
	"github.com/partite-ai/wacogo/wasmparser"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

// EngineOption configures an Engine at construction time.
type EngineOption func(*engineConfig)

// CoreModuleReplacer inspects a freshly-compiled inline core module
// during LoadComponent and optionally returns a replacement. Returning
// nil keeps the original. The replacement must be compiled against the
// same wazero runtime as the engine.
type CoreModuleReplacer func(wazero.CompiledModule) wazero.CompiledModule

type engineConfig struct {
	runtimeConfig wazero.RuntimeConfig
	replacer      CoreModuleReplacer
}

// WithRuntimeConfig overrides the wazero RuntimeConfig used to build the
// engine's runtime. If unset, wacogo uses a default config with
// CoreFeaturesV2 and the extended-const proposal enabled.
func WithRuntimeConfig(cfg wazero.RuntimeConfig) EngineOption {
	return func(c *engineConfig) { c.runtimeConfig = cfg }
}

// WithCoreModuleReplacer registers a hook called for every inline core
// module compiled during LoadComponent, including modules declared in
// nested subcomponents. Returning a non-nil module substitutes it for
// the engine's freshly-compiled one; the original is then closed.
// Returning nil keeps the original. Passing this option twice replaces
// the previous hook.
func WithCoreModuleReplacer(r CoreModuleReplacer) EngineOption {
	return func(c *engineConfig) { c.replacer = r }
}

// Engine owns the wazero runtime and is the entry point for loading components.
type Engine struct {
	runtime   wazero.Runtime
	canonHost *canon.Host
	validator *wasmparser.Validator
	replacer  CoreModuleReplacer
}

// NewEngine creates a new Engine with the given options.
func NewEngine(ctx context.Context, opts ...EngineOption) *Engine {
	cfg := &engineConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	cnf := cfg.runtimeConfig
	if cnf == nil {
		cnf = wazero.NewRuntimeConfig().WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExtendedConst)
	}
	rt := wazero.NewRuntimeWithConfig(ctx, cnf)
	return &Engine{
		runtime:   rt,
		canonHost: canon.NewHost(rt),
		validator: wasmparser.NewValidator(wasmparser.DefaultFeatures()),
		replacer:  cfg.replacer,
	}
}

// Close releases all resources held by the engine.
func (e *Engine) Close(ctx context.Context) error { return e.runtime.Close(ctx) }

// WazeroRuntime returns the engine's wazero runtime. A package-level
// function (not a method) so the host-internal accessor doesn't promote
// onto the public wacogo.Engine wrapper via embedding.
func WazeroRuntime(e *Engine) wazero.Runtime { return e.runtime }

// Validator returns the engine's shared wasmparser validator. A
// package-level function (not a method) for the same reason as
// WazeroRuntime.
func Validator(e *Engine) *wasmparser.Validator { return e.validator }
