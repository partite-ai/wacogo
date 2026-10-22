package core

import (
	"context"
	"errors"
	"slices"
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
	liftLimits    canon.LiftLimits
}

type LiftLimits = canon.LiftLimits

var ErrLiftLimit = canon.ErrLiftLimit

// WithLiftLimits configures per-call canonical lifting budgets. Zero fields
// are unlimited. CallRaw result decoders, including generated typed bindings,
// remain the caller's responsibility and are not covered by these limits.
func WithLiftLimits(limits LiftLimits) EngineOption {
	return func(c *engineConfig) { c.liftLimits = limits }
}

// WithRuntimeConfig overrides the wazero RuntimeConfig used to build the
// engine's runtime. If unset, wacogo uses a default config with
// CoreFeaturesV2 and the extended-const proposal enabled.
func WithRuntimeConfig(cfg wazero.RuntimeConfig) EngineOption {
	return func(c *engineConfig) { c.runtimeConfig = cfg }
}

// Engine owns the wazero runtime and is the entry point for loading components.
type Engine struct {
	runtime    wazero.Runtime
	canonHost  *canon.Host
	features   wasmparser.FeatureSet
	closed     atomic.Bool
	liftLimits canon.LiftLimits

	lifecycleMu     sync.Mutex
	runtimeClosed   bool
	tableModules    map[uint64]*accountedModule
	nextTableModule uint64

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
		runtime:    rt,
		canonHost:  canon.NewHost(rt),
		features:   wasmparser.DefaultFeatures(),
		bridges:    map[string]wazero.CompiledModule{},
		liftLimits: cfg.liftLimits,
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

// Close releases resources held by the engine and rejects further loading or
// instantiation. Reentrant and repeated calls are no-ops. Callbacks may call
// Close synchronously; reservations acquired or already being torn down by an
// in-flight callback are released when that operation unwinds.
func (e *Engine) Close(ctx context.Context) error {
	e.lifecycleMu.Lock()
	if !e.closed.CompareAndSwap(false, true) {
		e.lifecycleMu.Unlock()
		return nil
	}
	e.lifecycleMu.Unlock()
	runtimeErr := e.runtime.Close(ctx)
	e.lifecycleMu.Lock()
	e.runtimeClosed = true
	ids := make([]uint64, 0, len(e.tableModules))
	for id := range e.tableModules {
		if m := e.tableModules[id]; m.phase != moduleClosing && m.owner.activeOperations == 0 {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	modules := make([]*accountedModule, 0, len(ids))
	for _, id := range ids {
		modules = append(modules, e.tableModules[id])
	}
	e.lifecycleMu.Unlock()
	errs := []error{runtimeErr}
	for i := len(modules) - 1; i >= 0; i-- {
		if err := modules[i].refund(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// IsClosed reports whether the engine has been closed.
func (e *Engine) IsClosed() bool { return e.closed.Load() }

// WazeroRuntime returns the engine's wazero runtime. A package-level
// function (not a method) so the host-internal accessor doesn't promote
// onto the public wacogo.Engine wrapper via embedding.
func WazeroRuntime(e *Engine) wazero.Runtime { return e.runtime }

// EngineLiftLimits exposes immutable engine policy to the host package.
func EngineLiftLimits(e *Engine) LiftLimits { return e.liftLimits }

// Features returns the engine's parser feature set. Package-level
// function (not a method) for the same reason as WazeroRuntime.
func Features(e *Engine) wasmparser.FeatureSet { return e.features }
