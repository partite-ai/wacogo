// Package udpcreatesocket is the not-yet-implemented wasi:sockets/udp-create-socket host
// component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package udpcreatesocket

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	wasierr "github.com/partite-ai/wacogo/wasi/internal/wasierr"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/sockets/udpcreatesocket"
	"github.com/partite-ai/wacogo/wasi/sockets/network"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	UdpCreateSocket = gen.UdpCreateSocket

	ResultUdpSocketNetworkErrorCode    = gen.ResultUdpSocketNetworkErrorCode
	ResultUdpSocketNetworkErrorCodeOk  = gen.ResultUdpSocketNetworkErrorCodeOk
	ResultUdpSocketNetworkErrorCodeErr = gen.ResultUdpSocketNetworkErrorCodeErr
)

type impl struct{}

func (impl) CreateUdpSocket(_ context.Context, _ network.IpAddressFamily) (ResultUdpSocketNetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/udp-create-socket.create-udp-socket: %w", wasierr.ErrNotImplemented)
}

var (
	_ gen.UdpCreateSocket = impl{}
)

func NewInstance(
	ctx context.Context,
	engine *wacogo.Engine,
	networkInst *host.ComponentInstance,
	pollInst *host.ComponentInstance,
	udpInst *host.ComponentInstance,
	opts ...host.InstantiateOption,
) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, &gen.Deps{
		Network: networkInst.Core(),
		Poll:    pollInst.Core(),
		Udp:     udpInst.Core(),
	}, opts...)
}
