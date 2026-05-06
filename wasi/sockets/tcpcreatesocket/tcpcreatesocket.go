// Package tcpcreatesocket is the not-yet-implemented wasi:sockets/tcp-create-socket host
// component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package tcpcreatesocket

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	wasierr "github.com/partite-ai/wacogo/wasi/internal/wasierr"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/sockets/tcpcreatesocket"
	"github.com/partite-ai/wacogo/wasi/sockets/network"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	TcpCreateSocket = gen.TcpCreateSocket

	ResultTcpSocketNetworkErrorCode    = gen.ResultTcpSocketNetworkErrorCode
	ResultTcpSocketNetworkErrorCodeOk  = gen.ResultTcpSocketNetworkErrorCodeOk
	ResultTcpSocketNetworkErrorCodeErr = gen.ResultTcpSocketNetworkErrorCodeErr
)

type impl struct{}

func (impl) CreateTcpSocket(_ context.Context, _ network.IpAddressFamily) (ResultTcpSocketNetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp-create-socket.create-tcp-socket: %w", wasierr.ErrNotImplemented)
}

var (
	_ gen.TcpCreateSocket = impl{}
)

func NewInstance(
	ctx context.Context,
	engine *wacogo.Engine,
	errorInst *host.ComponentInstance,
	networkInst *host.ComponentInstance,
	pollInst *host.ComponentInstance,
	streamsInst *host.ComponentInstance,
	tcpInst *host.ComponentInstance,
) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, &gen.Deps{
		Error:   errorInst.Core(),
		Network: networkInst.Core(),
		Poll:    pollInst.Core(),
		Streams: streamsInst.Core(),
		Tcp:     tcpInst.Core(),
	})
}
