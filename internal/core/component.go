package core

import (
	"context"

	"github.com/partite-ai/wacogo/wasmparser"
	"github.com/tetratelabs/wazero"
)

// ExportDesc describes a component export.
type ExportDesc struct {
	Name  string
	Kind  Sort
	Index uint32
	// ParserFunctionType is the wasmparser function-type handle for
	// this export. Non-nil for func exports loaded from a validated
	// component. Use it for extended type information beyond the
	// runtime API.
	ParserFunctionType *wasmparser.FuncType
}

// ImportDesc describes a component import.
type ImportDesc struct {
	Name string
	Kind Sort
}

// CompiledModule wraps wazero.CompiledModule with extra metadata.
type CompiledModule struct {
	module        wazero.CompiledModule
	syntheticName string

	// wpModuleType is the opaque wasmparser handle for this module's
	// core-module type. Non-nil for modules defined inline in a loaded
	// component; nil for host-built modules (adapters, bridges, test
	// host modules).
	wpModuleType *wasmparser.ModuleType
}

// WrapCompiledModule returns a *CompiledModule wrapping cm. Used by
// the host package to surface a precompiled core module as a
// component-level export.
func WrapCompiledModule(cm wazero.CompiledModule) *CompiledModule {
	return &CompiledModule{module: cm}
}

// Component is an immutable, compiled component ready for instantiation.
type Component struct {
	engine          *Engine
	compiledModules []CompiledModule
	subComponents   []*Component
	plan            []planStep
	// typeResolvers is the component's type index space (spec: type). One
	// entry per TypeID in declaration order; resolved per instance via
	// planResolveType.
	typeResolvers []typeResolver
	exports       []ExportDesc
	imports       []ImportDesc

	// outerModuleClosures maps slot indices in compiledModules to the
	// (outerCount, outerIndex) of the parent scope they should be resolved
	// from at instantiation time. Only populated for slots that couldn't be
	// resolved at load time (i.e., the parent's module was an import placeholder).
	outerModuleClosures map[int]outerRef

	// outerComponentClosures is the same as outerModuleClosures but for the
	// subComponents index space.
	outerComponentClosures map[int]outerRef

	// wpType is the opaque wasmparser type handle for this component,
	// populated at load time. Nil if the engine wasn't able to produce
	// one. Used by Instantiate to subtype-check supplied instance args
	// via CheckInstantiation.
	wpType *wasmparser.ComponentType
}

// outerRef records the nesting count and index for an outer alias that must
// be resolved at instantiation time.
type outerRef struct {
	count uint32 // number of parent scopes to traverse
	index uint32 // index in the target parent's index space
}

// Exports returns the component's export descriptors.
func (c *Component) Exports() []ExportDesc { return c.exports }

// Imports returns the component's import descriptors.
func (c *Component) Imports() []ImportDesc { return c.imports }

// InstantiateOption configures a component instantiation.
type InstantiateOption func(*instantiateConfig)

// instantiateConfig stores the arguments passed to a component instantiation.
// Each imports entry maps an import name to the supplied value: *Func for
// func imports, *ComponentInstance for instance imports, *CompiledModule for
// core module imports, *Component for component imports, Type for type
// imports, Val for value imports. The runtime validates the Go-level type
// matches the import's declared Sort before running the plan.
type instantiateConfig struct {
	imports map[string]any
}

func (cfg *instantiateConfig) put(name string, arg any) {
	if cfg.imports == nil {
		cfg.imports = make(map[string]any)
	}
	cfg.imports[wasmparser.CanonicalizeImportName(name)] = arg
}

func applyInstantiateOptions(opts []InstantiateOption) *instantiateConfig {
	cfg := &instantiateConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// WithFuncImport satisfies a named func import with the given *ExportedFunc.
func WithFuncImport(name string, f *ExportedFunc) InstantiateOption {
	return func(cfg *instantiateConfig) { cfg.put(name, f) }
}

// WithInstanceImport satisfies a named instance import with the given *ComponentInstance.
func WithInstanceImport(name string, i *ComponentInstance) InstantiateOption {
	return func(cfg *instantiateConfig) { cfg.put(name, i) }
}

// WithModuleImport satisfies a named core module import with the given *CompiledModule.
func WithModuleImport(name string, m *CompiledModule) InstantiateOption {
	return func(cfg *instantiateConfig) { cfg.put(name, m) }
}

// WithComponentImport satisfies a named component import with the given *Component.
func WithComponentImport(name string, c *Component) InstantiateOption {
	return func(cfg *instantiateConfig) { cfg.put(name, c) }
}

// WithTypeImport satisfies a named type import with the given Type.
func WithTypeImport(name string, t Type) InstantiateOption {
	return func(cfg *instantiateConfig) { cfg.put(name, t) }
}

// Instantiate creates a running instance of this component.
func (c *Component) Instantiate(ctx context.Context, opts ...InstantiateOption) (*ComponentInstance, error) {
	return c.engine.instantiate(ctx, c, opts...)
}

// CheckInstantiation validates the supplied imports without executing the
// component's instantiation plan or any core start function. It checks that
// every declared import is present, non-nil, and of the expected runtime kind.
// Providers carrying parser type metadata are also checked for component-model
// subtype compatibility.
//
// CheckInstantiation does not check whether the component's engine or a
// provider is open and usable, and providers without parser type metadata get
// only presence and runtime-kind checks. A nil result does not guarantee that
// Instantiate will succeed.
func (c *Component) CheckInstantiation(opts ...InstantiateOption) error {
	return c.checkInstantiationImports(applyInstantiateOptions(opts))
}
