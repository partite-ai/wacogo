package wacogo

import "github.com/partite-ai/wacogo/internal/core"

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
