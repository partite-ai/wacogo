// Package monotonicclock implements the wasi:clocks/monotonic-clock host
// component on top of Go's time package. The clock's epoch is the moment
// the instance is constructed; instants are nanoseconds since that epoch.
package monotonicclock

import (
	"context"
	"math"
	"time"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/clocks/monotonicclock"
	"github.com/partite-ai/wacogo/wasi/io/poll"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	MonotonicClock = gen.MonotonicClock
)

type clock struct {
	pollInst *host.ComponentInstance
}

func newClock(pollInst *host.ComponentInstance) *clock {
	return &clock{
		pollInst: pollInst,
	}
}

func (c *clock) Now(_ context.Context) (uint64, error) {
	return uint64(time.Now().UnixNano()), nil
}

func (c *clock) Resolution(_ context.Context) (uint64, error) {
	return 1, nil
}

func (c *clock) SubscribeDuration(_ context.Context, ns uint64) (*poll.PollableHandle, error) {
	if ns > math.MaxInt64 {
		ns = math.MaxInt64
	}
	ch := make(chan struct{})
	if ns == 0 {
		close(ch)
		return c.subscribe(ch), nil
	}
	time.AfterFunc(time.Duration(ns), func() { close(ch) })
	return c.subscribe(ch), nil
}

func (c *clock) SubscribeInstant(_ context.Context, ns uint64) (*poll.PollableHandle, error) {
	ch := make(chan struct{})
	target := time.Unix(0, int64(ns))
	now := time.Now()
	if target.After(now) {
		time.AfterFunc(target.Sub(now), func() { close(ch) })
	} else {
		close(ch)
	}
	return c.subscribe(ch), nil
}

func (c *clock) subscribe(ch <-chan struct{}) *poll.PollableHandle {
	p := newDeadlinePollable(ch)
	return poll.NewPollableHandleIn(c.pollInst, p)
}

// deadlinePollable is the Pollable backing a subscribe-{duration,instant}
// result. Ready becomes true once the deadline has elapsed or Drop is called.
type deadlinePollable struct {
	done <-chan struct{}
}

func newDeadlinePollable(ch <-chan struct{}) *deadlinePollable {
	return &deadlinePollable{
		done: ch,
	}
}

func (p *deadlinePollable) Done() <-chan struct{} {
	return p.done
}

func (p *deadlinePollable) Ready(_ context.Context) (bool, error) {
	select {
	case <-p.done:
		return true, nil
	default:
	}
	return false, nil
}

func (p *deadlinePollable) Block(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return nil
	}
}

var _ gen.MonotonicClock = (*clock)(nil)

// NewInstance creates a monotonic-clock host component instance backed by
// Go's time package. pollInst must be the wasi:io/poll instance that owns
// the pollable resource type so subscribe-* results can be registered there.
func NewInstance(ctx context.Context, engine *wacogo.Engine, pollInst *host.ComponentInstance) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, newClock(pollInst), &gen.Deps{Poll: pollInst.Core()})
}
