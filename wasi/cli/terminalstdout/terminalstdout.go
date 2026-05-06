// Package terminalstdout is the stub wasi:cli/terminal-stdout host
// component.
package terminalstdout

import (
	"context"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/cli/terminalstdout"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	TerminalStdout               = gen.TerminalStdout
	OptionTerminalOutputResource = gen.OptionTerminalOutputResource
)

type impl struct{}

func (impl) GetTerminalStdout(_ context.Context) (OptionTerminalOutputResource, error) {
	return OptionTerminalOutputResource{}, nil
}

var _ gen.TerminalStdout = impl{}

func NewInstance(ctx context.Context, engine *wacogo.Engine, terminalOutputInst *host.ComponentInstance) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, &gen.Deps{TerminalOutput: terminalOutputInst.Core()})
}
