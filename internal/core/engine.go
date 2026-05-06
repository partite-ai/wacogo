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

type engineConfig struct{}

// Engine owns the wazero runtime and is the entry point for loading components.
type Engine struct {
	runtime   wazero.Runtime
	canonHost *canon.Host
	validator *wasmparser.Validator
}

// NewEngine creates a new Engine with the given options.
func NewEngine(ctx context.Context, opts ...EngineOption) *Engine {
	cfg := &engineConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	cnf := wazero.NewRuntimeConfig().WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExtendedConst)
	rt := wazero.NewRuntimeWithConfig(ctx, cnf)
	return &Engine{
		runtime:   rt,
		canonHost: canon.NewHost(rt),
		validator: wasmparser.NewValidator(wasmparser.DefaultFeatures()),
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
