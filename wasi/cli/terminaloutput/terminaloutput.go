// Package terminaloutput is the not-yet-implemented wasi:cli/terminal-output host
// component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package terminaloutput

import (
	"context"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/cli/terminaloutput"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	TerminalOutput               = gen.TerminalOutput
	TerminalOutputResource       = gen.TerminalOutputResource
	TerminalOutputResourceHandle = gen.TerminalOutputResourceHandle
)

func NewTerminalOutputResourceHandle(impl TerminalOutputResource) *TerminalOutputResourceHandle {
	return gen.NewTerminalOutputResourceHandle(impl)
}
func NewTerminalOutputResourceHandleIn(definer *host.ComponentInstance, impl TerminalOutputResource) *TerminalOutputResourceHandle {
	return gen.NewTerminalOutputResourceHandleIn(definer, impl)
}

type impl struct{}

type terminalOutputResourceImpl struct{}

var (
	_ gen.TerminalOutput         = impl{}
	_ gen.TerminalOutputResource = terminalOutputResourceImpl{}
)

func NewInstance(ctx context.Context, engine *wacogo.Engine, opts ...host.InstantiateOption) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, nil, opts...)
}
