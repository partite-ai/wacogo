// Package stdin is the wasi:cli/stdin host component. It exposes a
// single input stream backed by an io.Reader supplied by the embedder.
package stdin

import (
	"bytes"
	"context"
	"io"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/wasi/io/streams"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/cli/stdin"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type Stdin = gen.Stdin

type impl struct {
	stdin *streams.InputStreamHandle
}

func (i *impl) GetStdin(_ context.Context) (*streams.InputStreamHandle, error) {
	return i.stdin, nil
}

var _ gen.Stdin = (*impl)(nil)

func NewInstance(ctx context.Context, engine *wacogo.Engine, streamsInst, errorInst, pollInst *host.ComponentInstance, stdin io.Reader) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	if stdin == nil {
		stdin = bytes.NewReader(nil)
	}
	is := streams.NewIOReaderInputStream(errorInst, pollInst, stdin)
	handle := streams.NewInputStreamHandleIn(streamsInst, is)
	return fac.NewInstance(ctx, &impl{stdin: handle}, &gen.Deps{
		Error:   errorInst.Core(),
		Poll:    pollInst.Core(),
		Streams: streamsInst.Core(),
	})
}
