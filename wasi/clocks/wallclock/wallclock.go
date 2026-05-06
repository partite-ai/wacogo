// Package wallclock implements the wasi:clocks/wall-clock host component
// on top of Go's time package. Now reports the current Unix time;
// Resolution reports 1 ns.
package wallclock

import (
	"context"
	"time"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/clocks/wallclock"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	WallClock = gen.WallClock
	Datetime  = gen.Datetime
)

type clock struct {
	now func() time.Time
}

func newClock() *clock {
	return &clock{now: time.Now}
}

func (c *clock) Now(_ context.Context) (Datetime, error) {
	t := c.now()
	sec := t.Unix()
	nsec := t.Nanosecond()
	if sec < 0 {
		return Datetime{}, nil
	}
	return Datetime{
		Seconds:     uint64(sec),
		Nanoseconds: uint32(nsec),
	}, nil
}

func (c *clock) Resolution(_ context.Context) (Datetime, error) {
	return Datetime{Seconds: 0, Nanoseconds: 1}, nil
}

var _ gen.WallClock = (*clock)(nil)

// NewInstance creates a wall-clock host component instance backed by
// Go's time.Now.
func NewInstance(ctx context.Context, engine *wacogo.Engine) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, newClock(), nil)
}
