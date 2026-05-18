// Package network is the not-yet-implemented wasi:sockets/network host
// component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package network

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	wasierr "github.com/partite-ai/wacogo/wasi/internal/wasierr"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/sockets/network"
	"github.com/partite-ai/wacogo/wasi/io/wioerror"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	Network             = gen.Network
	NetworkResource     = gen.NetworkResource
	NetworkResourceHandle = gen.NetworkResourceHandle

	ErrorCode        = gen.ErrorCode
	IpAddressFamily  = gen.IpAddressFamily
	OptionErrorCode  = gen.OptionErrorCode

	IpAddress        = gen.IpAddress
	IpAddressIpv4    = gen.IpAddressIpv4
	IpAddressIpv6    = gen.IpAddressIpv6
	IpSocketAddress  = gen.IpSocketAddress
	IpSocketAddressIpv4 = gen.IpSocketAddressIpv4
	IpSocketAddressIpv6 = gen.IpSocketAddressIpv6

	Ipv4SocketAddress = gen.Ipv4SocketAddress
	Ipv6SocketAddress = gen.Ipv6SocketAddress

	TupleU8U8U8U8              = gen.TupleU8U8U8U8
	TupleU16U16U16U16U16U16U16U16 = gen.TupleU16U16U16U16U16U16U16U16
)

func NewNetworkResourceHandle(impl NetworkResource) *NetworkResourceHandle {
	return gen.NewNetworkResourceHandle(impl)
}
func NewNetworkResourceHandleIn(definer *host.ComponentInstance, impl NetworkResource) *NetworkResourceHandle {
	return gen.NewNetworkResourceHandleIn(definer, impl)
}

type impl struct{}

func (impl) NetworkErrorCode(_ context.Context, _ *wioerror.ErrorResourceHandle) (OptionErrorCode, error) {
	return OptionErrorCode{}, fmt.Errorf("wasi:sockets/network.network-error-code: %w", wasierr.ErrNotImplemented)
}

type networkResourceImpl struct{}

var (
	_ gen.Network         = impl{}
	_ gen.NetworkResource = networkResourceImpl{}
)

func NewInstance(
	ctx context.Context,
	engine *wacogo.Engine,
	errorInst *host.ComponentInstance,
	opts ...host.InstantiateOption,
) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, &gen.Deps{Error: errorInst.Core()}, opts...)
}
