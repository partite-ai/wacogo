package wasmparser

import "bytes"

// HostTypeBuilder is the arena-aware construction primitive set for
// synthesising component types outside the normal parse flow. Purpose-
// built for callers that need to declare host-component instance types
// (e.g. wacogo's host/ package). All pushes write directly into the
// validator's shared arena; types persist for the lifetime of the
// validator and can be referenced by subsequently-loaded components.
//
// Not goroutine-safe; callers must serialise access.
type HostTypeBuilder struct {
	arena *TypeArena
}

// HostTypeBuilder returns a builder over the validator's shared arena.
func (v *Validator) HostTypeBuilder() *HostTypeBuilder {
	return &HostTypeBuilder{arena: v.arena}
}

// AllocResourceID allocates and returns a fresh ResourceID in the arena.
func (b *HostTypeBuilder) AllocResourceID() ResourceID {
	return b.arena.allocResourceID()
}

// PushDefinedType appends t to the arena's defined-type table and
// returns its ID.
func (b *HostTypeBuilder) PushDefinedType(t DefinedTypeDesc) ComponentDefinedTypeID {
	return b.arena.pushDefinedType(t)
}

// PushFuncType appends t to the arena's func-type table and returns
// its ID.
func (b *HostTypeBuilder) PushFuncType(t FuncTypeDesc) ComponentFuncTypeID {
	return b.arena.pushFuncType(t)
}

// PushCoreModuleType appends t to the arena's core-module-type table
// and returns its ID. Used when building an instance type that
// re-exports a precompiled core wasm module.
func (b *HostTypeBuilder) PushCoreModuleType(t CoreModuleTypeDesc) CoreModuleTypeID {
	return b.arena.pushCoreModuleType(t)
}

// CoreModuleTypeFromBytes extracts the imports and exports of a core
// wasm module by parsing its binary. The returned CoreModuleTypeDesc
// can be passed to PushCoreModuleType. Returns an error for malformed
// input.
func (v *Validator) CoreModuleTypeFromBytes(wasmBytes []byte) (CoreModuleTypeDesc, error) {
	p := NewParser(bytes.NewReader(wasmBytes))
	return v.parseInnerCoreModule(p)
}

// PushComponentType appends t to the arena's component-type table and
// returns its ID.
func (b *HostTypeBuilder) PushComponentType(t ComponentTypeDesc) ComponentTypeID {
	return b.arena.pushComponentType(t)
}

// PushInstanceType appends t to the arena's instance-type table and
// returns its ID. Useful when constructing a ComponentTypeDesc whose
// imports reference an instance type built on the fly.
func (b *HostTypeBuilder) PushInstanceType(t InstanceTypeDesc) ComponentInstanceTypeID {
	return b.arena.pushInstanceType(t)
}

// NewComponentTypeHandle wraps an already-pushed ComponentTypeID in the
// public *ComponentType handle used by (*ComponentType).NewInstance and
// (*ComponentType).CheckInstantiation.
func (b *HostTypeBuilder) NewComponentTypeHandle(id ComponentTypeID) *ComponentType {
	return &ComponentType{arena: b.arena, id: id}
}

// NewFuncTypeHandle wraps an already-pushed ComponentFuncTypeID in the
// public *FuncType handle used by CheckInstantiation for func imports.
func (b *HostTypeBuilder) NewFuncTypeHandle(id ComponentFuncTypeID) *FuncType {
	return &FuncType{arena: b.arena, id: id}
}
