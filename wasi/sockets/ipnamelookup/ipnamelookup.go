// Package ipnamelookup is the not-yet-implemented wasi:sockets/ip-name-lookup host
// component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package ipnamelookup

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	wasierr "github.com/partite-ai/wacogo/wasi/internal/wasierr"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/sockets/ipnamelookup"
	"github.com/partite-ai/wacogo/wasi/io/poll"
	"github.com/partite-ai/wacogo/wasi/sockets/network"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	IpNameLookup            = gen.IpNameLookup
	ResolveAddressStream    = gen.ResolveAddressStream
	ResolveAddressStreamHandle = gen.ResolveAddressStreamHandle

	OptionNetworkIpAddress = gen.OptionNetworkIpAddress

	ResultOptionNetworkIpAddressNetworkErrorCode    = gen.ResultOptionNetworkIpAddressNetworkErrorCode
	ResultOptionNetworkIpAddressNetworkErrorCodeOk  = gen.ResultOptionNetworkIpAddressNetworkErrorCodeOk
	ResultOptionNetworkIpAddressNetworkErrorCodeErr = gen.ResultOptionNetworkIpAddressNetworkErrorCodeErr

	ResultResolveAddressStreamNetworkErrorCode    = gen.ResultResolveAddressStreamNetworkErrorCode
	ResultResolveAddressStreamNetworkErrorCodeOk  = gen.ResultResolveAddressStreamNetworkErrorCodeOk
	ResultResolveAddressStreamNetworkErrorCodeErr = gen.ResultResolveAddressStreamNetworkErrorCodeErr
)

func NewResolveAddressStreamHandle(impl ResolveAddressStream) *ResolveAddressStreamHandle {
	return gen.NewResolveAddressStreamHandle(impl)
}
func NewResolveAddressStreamHandleIn(definer *host.ComponentInstance, impl ResolveAddressStream) *ResolveAddressStreamHandle {
	return gen.NewResolveAddressStreamHandleIn(definer, impl)
}

type impl struct{}

func (impl) ResolveAddresses(_ context.Context, _ *network.NetworkResourceHandle, _ string) (ResultResolveAddressStreamNetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/ip-name-lookup.resolve-addresses: %w", wasierr.ErrNotImplemented)
}

type resolveAddressStreamImpl struct{}

func (resolveAddressStreamImpl) ResolveNextAddress(_ context.Context) (ResultOptionNetworkIpAddressNetworkErrorCode, error) {
	return nil, fmt.Errorf("wasi:sockets/ip-name-lookup.resolve-address-stream.resolve-next-address: %w", wasierr.ErrNotImplemented)
}
func (resolveAddressStreamImpl) Subscribe(_ context.Context) (*poll.PollableHandle, error) {
	return nil, fmt.Errorf("wasi:sockets/ip-name-lookup.resolve-address-stream.subscribe: %w", wasierr.ErrNotImplemented)
}

var (
	_ gen.IpNameLookup         = impl{}
	_ gen.ResolveAddressStream = resolveAddressStreamImpl{}
)

func NewInstance(
	ctx context.Context,
	engine *wacogo.Engine,
	networkInst *host.ComponentInstance,
	pollInst *host.ComponentInstance,
) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, &gen.Deps{
		Network: networkInst.Core(),
		Poll:    pollInst.Core(),
	})
}
