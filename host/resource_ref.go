package host

// ResourceTypeRef is a placeholder for a resource type that will be
// supplied by another component instance at Instantiate time. Obtain
// one from Builder.AddResourceRef and use Own / Borrow to reference
// it in function signatures.
//
// Every ResourceTypeRef declared on a Builder must be paired with a
// WithResourceFrom option at Instantiate time, or Instantiate
// returns an error.
type ResourceTypeRef struct {
	builder                 *Builder
	exportName              string
	componentResourceTypeID int
}

// Own returns a TypeExpr denoting own<R> for this referenced
// resource type.
func (r *ResourceTypeRef) Own() *TypeRef {
	return &TypeRef{ownOfResourceRef: r}
}

// Borrow returns a TypeExpr denoting borrow<R> for this referenced
// resource type.
func (r *ResourceTypeRef) Borrow() *TypeRef {
	return &TypeRef{borrowOfResourceRef: r}
}
