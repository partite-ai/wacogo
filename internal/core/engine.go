package core

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/partite-ai/wacogo/internal/canon"
	"github.com/partite-ai/wacogo/wasmparser"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

// ErrEngineClosed is returned by Engine operations attempted after Close.
var ErrEngineClosed = errors.New("engine is closed")

// EngineOption configures an Engine at construction time.
type EngineOption func(*engineConfig)

type engineConfig struct {
	runtimeConfig wazero.RuntimeConfig
}

// WithRuntimeConfig overrides the wazero RuntimeConfig used to build the
// engine's runtime. If unset, wacogo uses a default config with
// CoreFeaturesV2 and the extended-const proposal enabled.
func WithRuntimeConfig(cfg wazero.RuntimeConfig) EngineOption {
	return func(c *engineConfig) { c.runtimeConfig = cfg }
}

// Engine owns the wazero runtime and is the entry point for loading components.
type Engine struct {
	runtime   wazero.Runtime
	canonHost *canon.Host
	features  wasmparser.FeatureSet
	closed    atomic.Bool

	// bridges caches compiled instantiate-from-exports bridge modules by
	// their bytes, which depend only on the component, not the instance.
	bridgeMu sync.Mutex
	bridges  map[string]wazero.CompiledModule
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
		features:  wasmparser.DefaultFeatures(),
		bridges:   map[string]wazero.CompiledModule{},
	}
}

// compiledBridge returns the compiled bridge module for bridgeWasm,
// compiling it on first use.
func (e *Engine) compiledBridge(ctx context.Context, bridgeWasm []byte) (wazero.CompiledModule, error) {
	e.bridgeMu.Lock()
	defer e.bridgeMu.Unlock()
	if cm, ok := e.bridges[string(bridgeWasm)]; ok {
		return cm, nil
	}
	cm, err := e.runtime.CompileModule(ctx, bridgeWasm)
	if err != nil {
		return nil, err
	}
	e.bridges[string(bridgeWasm)] = cm
	return cm, nil
}

// Close releases all resources held by the engine. Idempotent: subsequent
// calls are no-ops returning nil. Once closed, the engine will not accept
// further LoadComponent / Instantiate calls.
func (e *Engine) Close(ctx context.Context) error {
	if !e.closed.CompareAndSwap(false, true) {
		return nil
	}
	return e.runtime.Close(ctx)
}

// IsClosed reports whether the engine has been closed.
func (e *Engine) IsClosed() bool { return e.closed.Load() }

// WazeroRuntime returns the engine's wazero runtime. A package-level
// function (not a method) so the host-internal accessor doesn't promote
// onto the public wacogo.Engine wrapper via embedding.
func WazeroRuntime(e *Engine) wazero.Runtime { return e.runtime }

// Features returns the engine's parser feature set. Package-level
// function (not a method) for the same reason as WazeroRuntime.
func Features(e *Engine) wasmparser.FeatureSet { return e.features }
