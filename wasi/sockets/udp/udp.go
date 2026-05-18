// Package udp is the not-yet-implemented wasi:sockets/udp host
// component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package udp

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	wasierr "github.com/partite-ai/wacogo/wasi/internal/wasierr"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/sockets/udp"
	"github.com/partite-ai/wacogo/wasi/io/poll"
	"github.com/partite-ai/wacogo/wasi/sockets/network"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	Udp       = gen.Udp
	UdpSocket = gen.UdpSocket
	UdpSocketHandle = gen.UdpSocketHandle

	IncomingDatagramStream       = gen.IncomingDatagramStream
	IncomingDatagramStreamHandle = gen.IncomingDatagramStreamHandle
	OutgoingDatagramStream       = gen.OutgoingDatagramStream
	OutgoingDatagramStreamHandle = gen.OutgoingDatagramStreamHandle

	IncomingDatagram = gen.IncomingDatagram
	OutgoingDatagram = gen.OutgoingDatagram

	OptionNetworkIpSocketAddress = gen.OptionNetworkIpSocketAddress

	ResultListIncomingDatagramNetworkErrorCode    = gen.ResultListIncomingDatagramNetworkErrorCode
	ResultListIncomingDatagramNetworkErrorCodeOk  = gen.ResultListIncomingDatagramNetworkErrorCodeOk
	ResultListIncomingDatagramNetworkErrorCodeErr = gen.ResultListIncomingDatagramNetworkErrorCodeErr

	ResultNetworkIpSocketAddressNetworkErrorCode    = gen.ResultNetworkIpSocketAddressNetworkErrorCode
	ResultNetworkIpSocketAddressNetworkErrorCodeOk  = gen.ResultNetworkIpSocketAddressNetworkErrorCodeOk
	ResultNetworkIpSocketAddressNetworkErrorCodeErr = gen.ResultNetworkIpSocketAddressNetworkErrorCodeErr

	ResultTupleIncomingDatagramStreamOutgoingDatagramStreamNetworkErrorCode    = gen.ResultTupleIncomingDatagramStreamOutgoingDatagramStreamNetworkErrorCode
	ResultTupleIncomingDatagramStreamOutgoingDatagramStreamNetworkErrorCodeOk  = gen.ResultTupleIncomingDatagramStreamOutgoingDatagramStreamNetworkErrorCodeOk
	ResultTupleIncomingDatagramStreamOutgoingDatagramStreamNetworkErrorCodeErr = gen.ResultTupleIncomingDatagramStreamOutgoingDatagramStreamNetworkErrorCodeErr

	ResultU64NetworkErrorCode    = gen.ResultU64NetworkErrorCode
	ResultU64NetworkErrorCodeOk  = gen.ResultU64NetworkErrorCodeOk
	ResultU64NetworkErrorCodeErr = gen.ResultU64NetworkErrorCodeErr

	ResultU8NetworkErrorCode    = gen.ResultU8NetworkErrorCode
	ResultU8NetworkErrorCodeOk  = gen.ResultU8NetworkErrorCodeOk
	ResultU8NetworkErrorCodeErr = gen.ResultU8NetworkErrorCodeErr

	Result_NetworkErrorCode    = gen.Result_NetworkErrorCode
	Result_NetworkErrorCodeOk  = gen.Result_NetworkErrorCodeOk
	Result_NetworkErrorCodeErr = gen.Result_NetworkErrorCodeErr

	TupleIncomingDatagramStreamOutgoingDatagramStream = gen.TupleIncomingDatagramStreamOutgoingDatagramStream
)

func NewUdpSocketHandle(impl UdpSocket) *UdpSocketHandle {
	return gen.NewUdpSocketHandle(impl)
}
func NewUdpSocketHandleIn(definer *host.ComponentInstance, impl UdpSocket) *UdpSocketHandle {
	return gen.NewUdpSocketHandleIn(definer, impl)
}
func NewIncomingDatagramStreamHandle(impl IncomingDatagramStream) *IncomingDatagramStreamHandle {
	return gen.NewIncomingDatagramStreamHandle(impl)
}
func NewIncomingDatagramStreamHandleIn(definer *host.ComponentInstance, impl IncomingDatagramStream) *IncomingDatagramStreamHandle {
	return gen.NewIncomingDatagramStreamHandleIn(definer, impl)
}
func NewOutgoingDatagramStreamHandle(impl OutgoingDatagramStream) *OutgoingDatagramStreamHandle {
	return gen.NewOutgoingDatagramStreamHandle(impl)
}
func NewOutgoingDatagramStreamHandleIn(definer *host.ComponentInstance, impl OutgoingDatagramStream) *OutgoingDatagramStreamHandle {
	return gen.NewOutgoingDatagramStreamHandleIn(definer, impl)
}

type impl struct{}

type udpSocketImpl struct{}

func (udpSocketImpl) AddressFamily(_ context.Context) (network.IpAddressFamily, error) {
	return 0, fmt.Errorf("wasi:sockets/udp.udp-socket.address-family: %w", wasierr.ErrNotImplemented)
}
func (udpSocketImpl) FinishBind(_ context.Context) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.udp-socket.finish-bind: %w", wasierr.ErrNotImplemented)
}
func (udpSocketImpl) LocalAddress(_ context.Context) (ResultNetworkIpSocketAddressNetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.udp-socket.local-address: %w", wasierr.ErrNotImplemented)
}
func (udpSocketImpl) ReceiveBufferSize(_ context.Context) (ResultU64NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.udp-socket.receive-buffer-size: %w", wasierr.ErrNotImplemented)
}
func (udpSocketImpl) RemoteAddress(_ context.Context) (ResultNetworkIpSocketAddressNetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.udp-socket.remote-address: %w", wasierr.ErrNotImplemented)
}
func (udpSocketImpl) SendBufferSize(_ context.Context) (ResultU64NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.udp-socket.send-buffer-size: %w", wasierr.ErrNotImplemented)
}
func (udpSocketImpl) SetReceiveBufferSize(_ context.Context, _ uint64) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.udp-socket.set-receive-buffer-size: %w", wasierr.ErrNotImplemented)
}
func (udpSocketImpl) SetSendBufferSize(_ context.Context, _ uint64) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.udp-socket.set-send-buffer-size: %w", wasierr.ErrNotImplemented)
}
func (udpSocketImpl) SetUnicastHopLimit(_ context.Context, _ uint8) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.udp-socket.set-unicast-hop-limit: %w", wasierr.ErrNotImplemented)
}
func (udpSocketImpl) StartBind(_ context.Context, _ *network.NetworkResourceHandle, _ network.IpSocketAddress) (Result_NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.udp-socket.start-bind: %w", wasierr.ErrNotImplemented)
}
func (udpSocketImpl) Stream(_ context.Context, _ OptionNetworkIpSocketAddress) (ResultTupleIncomingDatagramStreamOutgoingDatagramStreamNetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.udp-socket.stream: %w", wasierr.ErrNotImplemented)
}
func (udpSocketImpl) Subscribe(_ context.Context) (*poll.PollableHandle, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.udp-socket.subscribe: %w", wasierr.ErrNotImplemented)
}
func (udpSocketImpl) UnicastHopLimit(_ context.Context) (ResultU8NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.udp-socket.unicast-hop-limit: %w", wasierr.ErrNotImplemented)
}

type incomingDatagramStreamImpl struct{}

func (incomingDatagramStreamImpl) Receive(_ context.Context, _ uint64) (ResultListIncomingDatagramNetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.incoming-datagram-stream.receive: %w", wasierr.ErrNotImplemented)
}
func (incomingDatagramStreamImpl) Subscribe(_ context.Context) (*poll.PollableHandle, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.incoming-datagram-stream.subscribe: %w", wasierr.ErrNotImplemented)
}

type outgoingDatagramStreamImpl struct{}

func (outgoingDatagramStreamImpl) CheckSend(_ context.Context) (ResultU64NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.outgoing-datagram-stream.check-send: %w", wasierr.ErrNotImplemented)
}
func (outgoingDatagramStreamImpl) Send(_ context.Context, _ []OutgoingDatagram) (ResultU64NetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.outgoing-datagram-stream.send: %w", wasierr.ErrNotImplemented)
}
func (outgoingDatagramStreamImpl) Subscribe(_ context.Context) (*poll.PollableHandle, error) {
	return nil, fmt.Errorf("wasi:sockets/udp.outgoing-datagram-stream.subscribe: %w", wasierr.ErrNotImplemented)
}

var (
	_ gen.Udp                    = impl{}
	_ gen.UdpSocket               = udpSocketImpl{}
	_ gen.IncomingDatagramStream  = incomingDatagramStreamImpl{}
	_ gen.OutgoingDatagramStream  = outgoingDatagramStreamImpl{}
)

func NewInstance(
	ctx context.Context,
	engine *wacogo.Engine,
	networkInst *host.ComponentInstance,
	pollInst *host.ComponentInstance,
	opts ...host.InstantiateOption,
) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, &gen.Deps{
		Network: networkInst.Core(),
		Poll:    pollInst.Core(),
	}, opts...)
}
