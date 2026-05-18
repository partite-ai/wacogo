// Package environment is wasi:cli/environment host component.
package environment

import (
	"context"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/cli/environment"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	Environment       = gen.Environment
	OptionString      = gen.OptionString
	TupleStringString = gen.TupleStringString
)

type impl struct {
	args       []string
	env        [][2]string
	initialCwd string
}

func (i *impl) GetArguments(_ context.Context) ([]string, error) {
	return i.args, nil
}

func (i *impl) GetEnvironment(_ context.Context) ([]TupleStringString, error) {
	result := make([]TupleStringString, len(i.env))
	for idx, pair := range i.env {
		result[idx] = TupleStringString{F0: pair[0], F1: pair[1]}
	}
	return result, nil
}

func (i *impl) InitialCwd(_ context.Context) (OptionString, error) {
	if i.initialCwd == "" {
		return OptionString{}, nil
	}
	return OptionString{IsSome: true, Value: i.initialCwd}, nil
}

var _ gen.Environment = &impl{}

func NewInstance(ctx context.Context, engine *wacogo.Engine, args []string, env [][2]string, initialCwd string, opts ...host.InstantiateOption) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, &impl{
		args:       args,
		env:        env,
		initialCwd: initialCwd,
	}, nil)
}
