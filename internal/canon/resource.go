package canon

import "context"

// ResourceType describes a resource type to canon: identity (via
// pointer-equal interface values), defining instance, and destructor.
// The interface keeps canon/ free of a hard dependency on the parent
// package, and the marker method is exported so parent-package types (a
// different Go package) can implement it.
type ResourceType interface {
	IsResourceType()
	// DefiningInstance returns the component instance that defines this
	// resource. canon uses it only for pointer-equality comparison (same-
	// component shortcut).
	DefiningInstance() Instance
	// Destructor returns a Go-callable destructor closure to be invoked
	// when the last own handle for this resource type is dropped, or nil
	// if no destructor is registered. The closure receives the rep stored
	// against the handle being dropped. canon orchestrates invocation
	// (defining-instance Enter, same-component shortcut) but never
	// constructs the closure itself.
	Destructor() func(ctx context.Context, rep uint32) error
}
