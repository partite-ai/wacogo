// Package exit is the not-yet-implemented wasi:cli/exit host
// component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package exit

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	wasierr "github.com/partite-ai/wacogo/wasi/internal/wasierr"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/cli/exit"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	Exit      = gen.Exit
	Result__  = gen.Result__
	Result__Ok  = gen.Result__Ok
	Result__Err = gen.Result__Err
)

type impl struct{}

func (impl) Exit(_ context.Context, _ Result__) error {
	return fmt.Errorf("wasi:cli/exit.exit: %w", wasierr.ErrNotImplemented)
}

func (impl) ExitWithCode(_ context.Context, _ uint8) error {
	return fmt.Errorf("wasi:cli/exit.exit-with-code: %w", wasierr.ErrNotImplemented)
}

var _ gen.Exit = impl{}

func NewInstance(ctx context.Context, engine *wacogo.Engine) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, nil)
}
