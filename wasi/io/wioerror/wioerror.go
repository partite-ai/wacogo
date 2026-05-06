// Package wioerror is the not-yet-implemented wasi:io/error host
// component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package wioerror

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	wasierr "github.com/partite-ai/wacogo/wasi/internal/wasierr"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/io/wioerror"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	Error               = gen.Error
	ErrorResource       = gen.ErrorResource
	ErrorResourceHandle = gen.ErrorResourceHandle
)

func NewErrorResourceHandle(impl ErrorResource) *ErrorResourceHandle {
	return gen.NewErrorResourceHandle(impl)
}
func NewErrorResourceHandleIn(definer *host.ComponentInstance, impl ErrorResource) *ErrorResourceHandle {
	return gen.NewErrorResourceHandleIn(definer, impl)
}

type impl struct{}

type errorResourceImpl struct{}

func (errorResourceImpl) ToDebugString(_ context.Context) (string, error) {
	return "", fmt.Errorf("wasi:io/error.error.to-debug-string: %w", wasierr.ErrNotImplemented)
}

var (
	_ gen.Error         = impl{}
	_ gen.ErrorResource = errorResourceImpl{}
)

func NewInstance(ctx context.Context, engine *wacogo.Engine) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, nil)
}
