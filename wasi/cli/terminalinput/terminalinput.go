// Package terminalinput is the not-yet-implemented wasi:cli/terminal-input host
// component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package terminalinput

import (
	"context"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/cli/terminalinput"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	TerminalInput               = gen.TerminalInput
	TerminalInputResource       = gen.TerminalInputResource
	TerminalInputResourceHandle = gen.TerminalInputResourceHandle
)

func NewTerminalInputResourceHandle(impl TerminalInputResource) *TerminalInputResourceHandle {
	return gen.NewTerminalInputResourceHandle(impl)
}
func NewTerminalInputResourceHandleIn(definer *host.ComponentInstance, impl TerminalInputResource) *TerminalInputResourceHandle {
	return gen.NewTerminalInputResourceHandleIn(definer, impl)
}

type impl struct{}

type terminalInputResourceImpl struct{}

var (
	_ gen.TerminalInput         = impl{}
	_ gen.TerminalInputResource = terminalInputResourceImpl{}
)

func NewInstance(ctx context.Context, engine *wacogo.Engine, opts ...host.InstantiateOption) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, nil, opts...)
}
