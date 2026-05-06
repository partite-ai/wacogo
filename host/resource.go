package host

import (
	"context"
)

// ResourceType is an opaque handle for a host-owned resource type
// declared via Builder.AddResource. Call Own or Borrow to produce
// TypeExpr values for use in function signatures. Identity is by
// pointer value: each AddResource call returns a distinct
// ResourceType.
type ResourceType struct {
	builder                 *Builder
	exportName              string
	dtor                    ResourceDtor
	componentResourceTypeID int
}

// ResourceDtor is the destructor invoked when the last own handle
// for a host resource is dropped. h is the ComponentInstance that
// minted the handle and obj is the Go value previously registered
// for it via RegisterResource (nil when the dropped handle's rep
// was not produced by RegisterResource — e.g., a guest that mints
// handles via canon resource.new with arbitrary rep values).
// Returning a non-nil error traps the wasm caller that triggered
// the drop.
type ResourceDtor func(ctx context.Context, h *ComponentInstance, obj any) error

// Own returns a TypeExpr denoting own<R> for this resource. Each
// call returns a fresh handle; multiple uses share the same resource
// identity at runtime.
func (r *ResourceType) Own() *TypeRef {
	return &TypeRef{ownOfResource: r}
}

// Borrow returns a TypeExpr denoting borrow<R> for this resource.
func (r *ResourceType) Borrow() *TypeRef {
	return &TypeRef{borrowOfResource: r}
}
