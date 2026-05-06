package host

import (
	"github.com/partite-ai/wacogo/internal/core"
	"github.com/partite-ai/wacogo/wasmparser"
)

// scope holds the declarations contributed in one lexical scope of a
// host component — either the root (the *Builder) or one nested
// instance (an *InstanceBuilder). The shape mirrors the slice fields
// formerly held directly on *Builder.
type scope struct {
	parent *scope // nil at the root
	name   string // export name in parent; "" at root

	funcs        []funcDecl
	types        []typeDecl
	resources    []resourceDecl
	resourceRefs []resourceRefDecl
	aliases      []aliasDecl
	coreModules  []coreModuleDecl
	nested       []*scope
}

// aliasDecl re-exports an existing host-defined resource type under
// a second name with shared identity.
type aliasDecl struct {
	name string
	rt   *ResourceType
}

// coreModuleDecl is a precompiled core wasm module exported under a
// component-level name. The compiled module is shared across every
// Instantiate call.
type coreModuleDecl struct {
	name     string
	compiled *core.CompiledModule
	typeDesc wasmparser.CoreModuleTypeDesc
}
