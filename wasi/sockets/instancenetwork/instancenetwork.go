// Package instancenetwork is the not-yet-implemented wasi:sockets/instance-network host
// component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package instancenetwork

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	wasierr "github.com/partite-ai/wacogo/wasi/internal/wasierr"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/sockets/instancenetwork"
	"github.com/partite-ai/wacogo/wasi/sockets/network"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	InstanceNetwork = gen.InstanceNetwork
)

type impl struct{}

func (impl) InstanceNetwork(_ context.Context) (*network.NetworkResourceHandle, error) {
	return nil, fmt.Errorf("wasi:sockets/instance-network.instance-network: %w", wasierr.ErrNotImplemented)
}

var (
	_ gen.InstanceNetwork = impl{}
)

func NewInstance(
	ctx context.Context,
	engine *wacogo.Engine,
	networkInst *host.ComponentInstance,
	opts ...host.InstantiateOption,
) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, &gen.Deps{Network: networkInst.Core()}, opts...)
}
