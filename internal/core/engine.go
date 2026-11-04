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
	maxLiftBytes  uint64
	parseLimits   wasmparser.ParseLimits
}

// WithRuntimeConfig overrides the wazero RuntimeConfig used to build the
// engine's runtime. If unset, wacogo uses a default config with
// CoreFeaturesV2 and the extended-const proposal enabled.
func WithRuntimeConfig(cfg wazero.RuntimeConfig) EngineOption {
	return func(c *engineConfig) { c.runtimeConfig = cfg }
}

// WithMaxLiftBytes bounds how much memory a component can make its
// receiver allocate in one go: for one call's arguments, or for one
// call's results, the bytes allocated to hold the lists and strings
// they contain. A component receiver is charged the bytes allocated in
// its memory; a Go receiver is charged an estimate of the Go memory the
// lifted values occupy. A call that exceeds it fails with an error wrapping
// ErrLiftLimitExceeded, and a component whose results exceed it becomes
// unusable as though it had trapped. 0, the default, means unlimited.
func WithMaxLiftBytes(n uint64) EngineOption {
	return func(c *engineConfig) { c.maxLiftBytes = n }
}

// ErrLiftLimitExceeded is wrapped by errors reported when a call's
// arguments or results exceed the limit set by WithMaxLiftBytes.
var ErrLiftLimitExceeded = canon.ErrLiftLimitExceeded

// ParseLimits bounds what LoadComponent accepts. A zero field means the default
// given for it, and Unlimited means no limit.
type ParseLimits = wasmparser.ParseLimits

// Unlimited is the ParseLimits value for a limit that is not enforced.
const Unlimited = wasmparser.Unlimited

// WithParseLimits sets the limits components are loaded with. Fields left at
// zero keep their defaults.
func WithParseLimits(limits ParseLimits) EngineOption {
	return func(c *engineConfig) { c.parseLimits = limits }
}

// ErrInstanceLimitExceeded is wrapped by the error LoadComponent reports when
// one instantiation of a component would create more instances than ParseLimits
// allows.
var ErrInstanceLimitExceeded = wasmparser.ErrInstanceLimitExceeded

// Engine owns the wazero runtime and is the entry point for loading components.
type Engine struct {
	runtime      wazero.Runtime
	canonHost    *canon.Host
	features     wasmparser.FeatureSet
	maxLiftBytes uint64
	parseLimits  ParseLimits
	closed       atomic.Bool

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
		runtime:      rt,
		canonHost:    canon.NewHost(rt, cfg.maxLiftBytes),
		features:     wasmparser.DefaultFeatures(),
		maxLiftBytes: cfg.maxLiftBytes,
		parseLimits:  cfg.parseLimits,
		bridges:      map[string]wazero.CompiledModule{},
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
