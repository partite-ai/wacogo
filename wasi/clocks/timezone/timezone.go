// Package timezone is the not-yet-implemented wasi:clocks/timezone host
// component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package timezone

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	wasierr "github.com/partite-ai/wacogo/wasi/internal/wasierr"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/clocks/timezone"
	"github.com/partite-ai/wacogo/wasi/clocks/wallclock"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	Timezone        = gen.Timezone
	TimezoneDisplay = gen.TimezoneDisplay
)

type impl struct{}

func (impl) Display(_ context.Context, _ wallclock.Datetime) (TimezoneDisplay, error) {
	return TimezoneDisplay{}, fmt.Errorf("wasi:clocks/timezone.display: %w", wasierr.ErrNotImplemented)
}

func (impl) UtcOffset(_ context.Context, _ wallclock.Datetime) (int32, error) {
	return 0, fmt.Errorf("wasi:clocks/timezone.utc-offset: %w", wasierr.ErrNotImplemented)
}

var _ gen.Timezone = impl{}

func NewInstance(ctx context.Context, engine *wacogo.Engine, wallClockInst *host.ComponentInstance, opts ...host.InstantiateOption) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, &gen.Deps{WallClock: wallClockInst.Core()}, opts...)
}
