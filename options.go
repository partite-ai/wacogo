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

// WithMaxLiftBytes bounds how much memory a component can make its
// receiver allocate in one go: for one call's arguments, or for one
// call's results, the bytes allocated to hold the lists and strings
// they contain. A component receiver is charged the bytes allocated in
// its memory; a Go receiver is charged an estimate of the Go memory the
// lifted values occupy. A call that exceeds it fails with an error wrapping
// ErrLiftLimitExceeded, and a component whose results exceed it becomes
// unusable as though it had trapped. 0, the default, means unlimited.
func WithMaxLiftBytes(n uint64) EngineOption {
	return core.WithMaxLiftBytes(n)
}

// ErrLiftLimitExceeded is wrapped by errors reported when a call's
// arguments or results exceed the limit set by WithMaxLiftBytes.
var ErrLiftLimitExceeded = core.ErrLiftLimitExceeded

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

// WithResourceTypeImport satisfies a named resource type import with the
// resource that lender exports under the given name.
func WithResourceTypeImport(name string, lender *ComponentInstance, export string) InstantiateOption {
	return core.WithResourceTypeImport(name, lender, export)
}
