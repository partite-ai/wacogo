// Package incominghandler is the not-yet-implemented wasi:http/incoming-handler
// host component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package incominghandler

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	wasierr "github.com/partite-ai/wacogo/wasi/internal/wasierr"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/http/incominghandler"
	"github.com/partite-ai/wacogo/wasi/http/types"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type IncomingHandler = gen.IncomingHandler

type impl struct{}

func (impl) Handle(_ context.Context, _ *types.IncomingRequestHandle, _ *types.ResponseOutparamHandle) error {
	return fmt.Errorf("wasi:http/incoming-handler.handle: %w", wasierr.ErrNotImplemented)
}

var _ gen.IncomingHandler = impl{}

func NewInstance(
	ctx context.Context,
	engine *wacogo.Engine,
	errorInst *host.ComponentInstance,
	pollInst *host.ComponentInstance,
	streamsInst *host.ComponentInstance,
	typesInst *host.ComponentInstance,
	opts ...host.InstantiateOption,
) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, &gen.Deps{
		Error:   errorInst.Core(),
		Poll:    pollInst.Core(),
		Streams: streamsInst.Core(),
		Types:   typesInst.Core(),
	}, opts...)
}
