package wacogo

import (
	"github.com/partite-ai/wacogo/internal/core"
	"github.com/tetratelabs/wazero"
)

// WithRuntimeConfig overrides the wazero RuntimeConfig used to build the
// engine's runtime. If unset, wacogo uses a default config with
// CoreFeaturesV2 and the extended-const proposal enabled.
func WithRuntimeConfig(cfg wazero.RuntimeConfig) EngineOption {
	return core.WithRuntimeConfig(cfg)
}

// WithCoreModuleReplacer registers a hook called for every inline core
// module compiled during LoadComponent, including modules declared in
// nested subcomponents. Returning a non-nil module substitutes it for
// the engine's freshly-compiled one; the original is then closed.
// Returning nil keeps the original. The replacement must be compiled
// against the same runtime as the engine (Engine.WazeroRuntime).
// Passing this option twice replaces the previous hook.
func WithCoreModuleReplacer(r CoreModuleReplacer) EngineOption {
	return core.WithCoreModuleReplacer(r)
}

// WithFuncImport satisfies a named func import with the given *ExportedFunc.
func WithFuncImport(name string, f *ExportedFunc) InstantiateOption {
	return core.WithFuncImport(name, f)
}

// WithInstanceImport satisfies a named instance import with the given *ComponentInstance.
func WithInstanceImport(name string, i *ComponentInstance) InstantiateOption {
	return core.WithInstanceImport(name, i)
}

// WithModuleImport satisfies a named core module import with the given *CompiledModule.
func WithModuleImport(name string, m *CompiledModule) InstantiateOption {
	return core.WithModuleImport(name, m)
}

// WithComponentImport satisfies a named component import with the given *Component.
func WithComponentImport(name string, c *Component) InstantiateOption {
	return core.WithComponentImport(name, c)
}

// WithTypeImport satisfies a named type import with the given Type.
func WithTypeImport(name string, t Type) InstantiateOption {
	return core.WithTypeImport(name, t)
}
