// Package poll is the not-yet-implemented wasi:io/poll host
// component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package poll

import (
	"context"
	"fmt"
	"reflect"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	wasierr "github.com/partite-ai/wacogo/wasi/internal/wasierr"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/io/poll"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	Poll           = gen.Poll
	PollableHandle = gen.PollableHandle
)

type Pollable interface {
	gen.Pollable
	Done() <-chan struct{}
}

func NewPollableHandle(impl Pollable) *PollableHandle {
	return gen.NewPollableHandle(impl)
}
func NewPollableHandleIn(definer *host.ComponentInstance, impl Pollable) *PollableHandle {
	return gen.NewPollableHandleIn(definer, impl)
}

type impl struct{}

func (impl) Poll(ctx context.Context, pollables []*PollableHandle) ([]uint32, error) {
	var doneCases []reflect.SelectCase
	var doneChans []<-chan struct{}
	for _, p := range pollables {
		local, ok := p.LocalImpl()
		if !ok {
			return nil, fmt.Errorf("wasi:io/poll.poll: %w: pollable is not local", wasierr.ErrNotImplemented)
		}
		ch := local.(Pollable).Done()
		doneCases = append(doneCases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ch),
		})
		doneChans = append(doneChans, ch)
	}
	doneCases = append(doneCases, reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(ctx.Done()),
	})

	reflect.Select(doneCases)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var readyIndexes []uint32
	for i := range pollables {
		select {
		case <-doneChans[i]:
			readyIndexes = append(readyIndexes, uint32(i))
		default:
		}
	}
	return readyIndexes, nil
}

var (
	_ gen.Poll = impl{}
)

func NewInstance(ctx context.Context, engine *wacogo.Engine, opts ...host.InstantiateOption) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, nil, opts...)
}
