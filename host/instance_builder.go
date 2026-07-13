package host

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo/internal/core"
)

// InstanceBuilder accumulates declarations for a nested instance
// export of a host component. Returned by Builder.AddNestedInstance
// and by InstanceBuilder.AddNestedInstance. Exposes the same
// declaration surface as the top-level Builder, scoped to this
// instance.
//
// An InstanceBuilder is not safe for concurrent use.
type InstanceBuilder struct {
	parent *Builder // root; carries the engine + ID counters
	scope  *scope   // this instance's scope node
}

// AddFunction declares a function export named exportName with the
// signature ty, implemented by fn. Names must be unique within this
// instance scope.
func (ib *InstanceBuilder) AddFunction(exportName string, ty *FuncType, fn Func) {
	ib.scope.funcs = append(ib.scope.funcs, funcDecl{name: exportName, ty: ty, fn: fn})
}

// AddResource declares a host-owned resource type exported as
// exportName from this nested instance. See Builder.AddResource for
// dtor semantics.
func (ib *InstanceBuilder) AddResource(exportName string, dtor ResourceDtor) *ResourceType {
	rt := &ResourceType{
		builder:                 ib.parent,
		exportName:              exportName,
		dtor:                    dtor,
		componentResourceTypeID: ib.parent.nextComponentResourceTypeID,
	}
	ib.parent.nextComponentResourceTypeID++
	ib.scope.resources = append(ib.scope.resources, resourceDecl{name: exportName, rt: rt})
	return rt
}

// AddResourceRef declares a resource type whose identity will be
// supplied by another component instance at Instantiate time. See
// Builder.AddResourceRef.
func (ib *InstanceBuilder) AddResourceRef(exportName string) *ResourceTypeRef {
	r := &ResourceTypeRef{
		builder:                 ib.parent,
		exportName:              exportName,
		componentResourceTypeID: ib.parent.nextComponentResourceTypeID,
	}
	ib.parent.nextComponentResourceTypeID++
	ib.scope.resourceRefs = append(ib.scope.resourceRefs, resourceRefDecl{name: exportName, ref: r})
	return r
}

// AddType registers typ in this nested instance's type space. A non-empty
// exportName is local to this instance scope; pass "" only for an anonymous
// structural type shared by declarations in this component. See Builder.AddType
// for the nominal/structural rules.
func (ib *InstanceBuilder) AddType(exportName string, typ TypeExpr) *TypeRef {
	ref := &TypeRef{declExpr: typ}
	ib.scope.types = append(ib.scope.types, typeDecl{name: exportName, expr: typ, ref: ref})
	return ref
}

// AddResourceAlias re-exports rt under name from this nested instance
// scope. rt must come from the same root *Builder.
func (ib *InstanceBuilder) AddResourceAlias(name string, rt *ResourceType) error {
	if rt == nil {
		return fmt.Errorf("wacogo/host: AddResourceAlias %q: nil *ResourceType", name)
	}
	if rt.builder != ib.parent {
		return fmt.Errorf("wacogo/host: AddResourceAlias %q: target *ResourceType belongs to a different Builder", name)
	}
	ib.scope.aliases = append(ib.scope.aliases, aliasDecl{name: name, rt: rt})
	return nil
}

// AddCoreModule registers a precompiled core wasm module in this
// nested instance scope. See Builder.AddCoreModule.
func (ib *InstanceBuilder) AddCoreModule(name string, wasmBytes []byte) error {
	rt := core.WazeroRuntime(ib.parent.engine)
	cm, err := rt.CompileModule(context.Background(), wasmBytes)
	if err != nil {
		return fmt.Errorf("wacogo/host: AddCoreModule %q: %w", name, err)
	}
	td, err := ib.parent.arena.CoreModuleTypeFromBytes(wasmBytes)
	if err != nil {
		return fmt.Errorf("wacogo/host: AddCoreModule %q: extract type: %w", name, err)
	}
	ib.scope.coreModules = append(ib.scope.coreModules, coreModuleDecl{
		name:     name,
		compiled: core.WrapCompiledModule(cm),
		typeDesc: td,
	})
	return nil
}

// AddNestedInstance declares another nested-instance export inside
// this instance.
func (ib *InstanceBuilder) AddNestedInstance(name string) *InstanceBuilder {
	child := &scope{parent: ib.scope, name: name}
	ib.scope.nested = append(ib.scope.nested, child)
	return &InstanceBuilder{parent: ib.parent, scope: child}
}
