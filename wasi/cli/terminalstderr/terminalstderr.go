// Package terminalstderr is the stub wasi:cli/terminal-stderr host
// component.
package terminalstderr

import (
	"context"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/cli/terminalstderr"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	TerminalStderr               = gen.TerminalStderr
	OptionTerminalOutputResource = gen.OptionTerminalOutputResource
)

type impl struct{}

func (impl) GetTerminalStderr(_ context.Context) (OptionTerminalOutputResource, error) {
	return OptionTerminalOutputResource{}, nil
}

var _ gen.TerminalStderr = impl{}

func NewInstance(ctx context.Context, engine *wacogo.Engine, terminalOutputInst *host.ComponentInstance, opts ...host.InstantiateOption) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, &gen.Deps{TerminalOutput: terminalOutputInst.Core()}, opts...)
}
