package host

import (
	"github.com/partite-ai/wacogo/internal/core"
)

// InstantiateOption configures a single Component.Instantiate call.
// Construct via WithResourceFrom or WithUserState.
type InstantiateOption interface {
	apply(*instantiateOpts)
}

type instantiateOpts struct {
	resourceFroms []resourceFromOpt
	userState     any
}

type resourceFromOpt struct {
	ref          *ResourceTypeRef
	lender       *core.ComponentInstance
	lenderExport string
}

func (o resourceFromOpt) apply(s *instantiateOpts) {
	s.resourceFroms = append(s.resourceFroms, o)
}

// WithUserState attaches an opaque value to the host component
// instance, retrievable via ComponentInstance.UserState.
func WithUserState(s any) InstantiateOption {
	return userStateOpt{s: s}
}

type userStateOpt struct{ s any }

func (o userStateOpt) apply(s *instantiateOpts) { s.userState = o.s }

// WithResourceFrom binds a ResourceTypeRef placeholder (returned
// from Builder.AddResourceRef) to the resource type that lender
// exports under lenderExport. Instantiate fails if lender does not
// export a resource type with that name.
func WithResourceFrom(ref *ResourceTypeRef, lender *core.ComponentInstance, lenderExport string) InstantiateOption {
	return resourceFromOpt{ref: ref, lender: lender, lenderExport: lenderExport}
}
