// Package stdout is the wasi:cli/stdout host component. It exposes a
// single output stream backed by an io.Writer supplied by the embedder.
package stdout

import (
	"context"
	"io"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/wasi/io/streams"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/cli/stdout"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type Stdout = gen.Stdout

type impl struct {
	errorInst, pollInst, streamsInst *host.ComponentInstance
	stdout                           io.Writer
}

func (i *impl) GetStdout(_ context.Context) (*streams.OutputStreamHandle, error) {
	os := streams.NewIOWriterOutputStream(i.errorInst, i.pollInst, i.stdout)
	handle := streams.NewOutputStreamHandleIn(i.streamsInst, os)
	return handle, nil
}

var _ gen.Stdout = (*impl)(nil)

func NewInstance(ctx context.Context, engine *wacogo.Engine, streamsInst, errorInst, pollInst *host.ComponentInstance, stdout io.Writer, opts ...host.InstantiateOption) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}

	return fac.NewInstance(ctx, &impl{errorInst: errorInst, pollInst: pollInst, streamsInst: streamsInst, stdout: stdout}, &gen.Deps{
		Error:   errorInst.Core(),
		Poll:    pollInst.Core(),
		Streams: streamsInst.Core(),
	}, opts...)
}
