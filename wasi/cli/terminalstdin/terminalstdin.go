// Package terminalstdin is the stub wasi:cli/terminal-stdin host
// component.
package terminalstdin

import (
	"context"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/cli/terminalstdin"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	TerminalStdin               = gen.TerminalStdin
	OptionTerminalInputResource = gen.OptionTerminalInputResource
)

type impl struct{}

func (impl) GetTerminalStdin(_ context.Context) (OptionTerminalInputResource, error) {
	return OptionTerminalInputResource{}, nil
}

var _ gen.TerminalStdin = impl{}

func NewInstance(ctx context.Context, engine *wacogo.Engine, terminalInputInst *host.ComponentInstance) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, &gen.Deps{TerminalInput: terminalInputInst.Core()})
}
