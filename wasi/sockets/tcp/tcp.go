// Package tcp is the not-yet-implemented wasi:sockets/tcp host
// component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package tcp

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	wasierr "github.com/partite-ai/wacogo/wasi/internal/wasierr"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/sockets/tcp"
	"github.com/partite-ai/wacogo/wasi/io/poll"
	"github.com/partite-ai/wacogo/wasi/sockets/network"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	Tcp          = gen.Tcp
	TcpSocket    = gen.TcpSocket
	TcpSocketHandle = gen.TcpSocketHandle

	ShutdownType = gen.ShutdownType

	ResultBoolNetworkErrorCode    = gen.ResultBoolNetworkErrorCode
	ResultBoolNetworkErrorCodeOk  = gen.ResultBoolNetworkErrorCodeOk
	ResultBoolNetworkErrorCodeErr = gen.ResultBoolNetworkErrorCodeErr

	ResultNetworkIpSocketAddressNetworkErrorCode    = gen.ResultNetworkIpSocketAddressNetworkErrorCode
	ResultNetworkIpSocketAddressNetworkErrorCodeOk  = gen.ResultNetworkIpSocketAddressNetworkErrorCodeOk
	ResultNetworkIpSocketAddressNetworkErrorCodeErr = gen.ResultNetworkIpSocketAddressNetworkErrorCodeErr

	ResultTupleInputStreamOutputStreamNetworkErrorCode    = gen.ResultTupleInputStreamOutputStreamNetworkErrorCode
	ResultTupleInputStreamOutputStreamNetworkErrorCodeOk  = gen.ResultTupleInputStreamOutputStreamNetworkErrorCodeOk
	ResultTupleInputStreamOutputStreamNetworkErrorCodeErr = gen.ResultTupleInputStreamOutputStreamNetworkErrorCodeErr

	ResultTupleTcpSocketInputStreamOutputStreamNetworkErrorCode    = gen.ResultTupleTcpSocketInputStreamOutputStreamNetworkErrorCode
	ResultTupleTcpSocketInputStreamOutputStreamNetworkErrorCodeOk  = gen.ResultTupleTcpSocketInputStreamOutputStreamNetworkErrorCodeOk
	ResultTupleTcpSocketInputStreamOutputStreamNetworkErrorCodeErr = gen.ResultTupleTcpSocketInputStreamOutputStreamNetworkErrorCodeErr

	ResultU32NetworkErrorCode    = gen.ResultU32NetworkErrorCode
	ResultU32NetworkErrorCodeOk  = gen.ResultU32NetworkErrorCodeOk
	ResultU32NetworkErrorCodeErr = gen.ResultU32NetworkErrorCodeErr

	ResultU64NetworkErrorCode    = gen.ResultU64NetworkErrorCode
	ResultU64NetworkErrorCodeOk  = gen.ResultU64NetworkErrorCodeOk
	ResultU64NetworkErrorCodeErr = gen.ResultU64NetworkErrorCodeErr

	ResultU8NetworkErrorCode    = gen.ResultU8NetworkErrorCode
	ResultU8NetworkErrorCodeOk  = gen.ResultU8NetworkErrorCodeOk
	ResultU8NetworkErrorCodeErr = gen.ResultU8NetworkErrorCodeErr

	Result_NetworkErrorCode    = gen.Result_NetworkErrorCode
	Result_NetworkErrorCodeOk  = gen.Result_NetworkErrorCodeOk
	Result_NetworkErrorCodeErr = gen.Result_NetworkErrorCodeErr

	TupleInputStreamOutputStream              = gen.TupleInputStreamOutputStream
	TupleTcpSocketInputStreamOutputStream     = gen.TupleTcpSocketInputStreamOutputStream
)

func NewTcpSocketHandle(impl TcpSocket) *TcpSocketHandle {
	return gen.NewTcpSocketHandle(impl)
}
func NewTcpSocketHandleIn(definer *host.ComponentInstance, impl TcpSocket) *TcpSocketHandle {
	return gen.NewTcpSocketHandleIn(definer, impl)
}

type impl struct{}

type tcpSocketImpl struct{}

func (tcpSocketImpl) Accept(_ context.Context) (ResultTupleTcpSocketInputStreamOutputStreamNetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.accept: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) AddressFamily(_ context.Context) (network.IpAddressFamily, error) {
	return 0, fmt.Errorf("wasi:sockets/tcp.tcp-socket.address-family: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) FinishBind(_ context.Context) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.finish-bind: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) FinishConnect(_ context.Context) (ResultTupleInputStreamOutputStreamNetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.finish-connect: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) FinishListen(_ context.Context) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.finish-listen: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) HopLimit(_ context.Context) (ResultU8NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.hop-limit: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) IsListening(_ context.Context) (bool, error) {
	return false, fmt.Errorf("wasi:sockets/tcp.tcp-socket.is-listening: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) KeepAliveCount(_ context.Context) (ResultU32NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.keep-alive-count: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) KeepAliveEnabled(_ context.Context) (ResultBoolNetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.keep-alive-enabled: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) KeepAliveIdleTime(_ context.Context) (ResultU64NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.keep-alive-idle-time: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) KeepAliveInterval(_ context.Context) (ResultU64NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.keep-alive-interval: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) LocalAddress(_ context.Context) (ResultNetworkIpSocketAddressNetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.local-address: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) ReceiveBufferSize(_ context.Context) (ResultU64NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.receive-buffer-size: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) RemoteAddress(_ context.Context) (ResultNetworkIpSocketAddressNetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.remote-address: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) SendBufferSize(_ context.Context) (ResultU64NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.send-buffer-size: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) SetHopLimit(_ context.Context, _ uint8) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.set-hop-limit: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) SetKeepAliveCount(_ context.Context, _ uint32) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.set-keep-alive-count: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) SetKeepAliveEnabled(_ context.Context, _ bool) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.set-keep-alive-enabled: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) SetKeepAliveIdleTime(_ context.Context, _ uint64) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.set-keep-alive-idle-time: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) SetKeepAliveInterval(_ context.Context, _ uint64) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.set-keep-alive-interval: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) SetListenBacklogSize(_ context.Context, _ uint64) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.set-listen-backlog-size: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) SetReceiveBufferSize(_ context.Context, _ uint64) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.set-receive-buffer-size: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) SetSendBufferSize(_ context.Context, _ uint64) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.set-send-buffer-size: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) Shutdown(_ context.Context, _ ShutdownType) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.shutdown: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) StartBind(_ context.Context, _ *network.NetworkResourceHandle, _ network.IpSocketAddress) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.start-bind: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) StartConnect(_ context.Context, _ *network.NetworkResourceHandle, _ network.IpSocketAddress) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.start-connect: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) StartListen(_ context.Context) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.start-listen: %w", wasierr.ErrNotImplemented)
}
func (tcpSocketImpl) Subscribe(_ context.Context) (*poll.PollableHandle, error) {
	return nil, fmt.Errorf("wasi:sockets/tcp.tcp-socket.subscribe: %w", wasierr.ErrNotImplemented)
}

var (
	_ gen.Tcp       = impl{}
	_ gen.TcpSocket = tcpSocketImpl{}
)

func NewInstance(
	ctx context.Context,
	engine *wacogo.Engine,
	errorInst *host.ComponentInstance,
	networkInst *host.ComponentInstance,
	pollInst *host.ComponentInstance,
	streamsInst *host.ComponentInstance,
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
	})
}
