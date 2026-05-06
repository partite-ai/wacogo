// Package preopens implements the wasi:filesystem/preopens host
// component. The default Preopens impl is EmptyFS; FSPreopens adapts
// one or more io/fs.FS values into preopened directories.
package preopens

import (
	"context"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/filesystem/preopens"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	Preopens              = gen.Preopens
	TupleDescriptorString = gen.TupleDescriptorString
)

// Deps bundles the host component instances that an FS-backed Preopens
// implementation needs to construct descriptors, streams, and timestamps.
type Deps struct {
	Types     *host.ComponentInstance
	Streams   *host.ComponentInstance
	Error     *host.ComponentInstance
	Poll      *host.ComponentInstance
	WallClock *host.ComponentInstance
}

func NewInstance(
	ctx context.Context,
	engine *wacogo.Engine,
	errorInst *host.ComponentInstance,
	pollInst *host.ComponentInstance,
	streamsInst *host.ComponentInstance,
	typesInst *host.ComponentInstance,
	wallClockInst *host.ComponentInstance,
	impl Preopens,
) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl, &gen.Deps{
		Error:     errorInst.Core(),
		Poll:      pollInst.Core(),
		Streams:   streamsInst.Core(),
		Types:     typesInst.Core(),
		WallClock: wallClockInst.Core(),
	})
}
