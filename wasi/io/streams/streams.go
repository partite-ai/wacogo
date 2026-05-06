// Package streams is the not-yet-implemented wasi:io/streams host
// component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package streams

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	wasierr "github.com/partite-ai/wacogo/wasi/internal/wasierr"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/io/streams"
	"github.com/partite-ai/wacogo/wasi/io/poll"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	Streams             = gen.Streams
	InputStream         = gen.InputStream
	InputStreamHandle   = gen.InputStreamHandle
	OutputStream        = gen.OutputStream
	OutputStreamHandle  = gen.OutputStreamHandle

	StreamError                      = gen.StreamError
	StreamErrorLastOperationFailed   = gen.StreamErrorLastOperationFailed
	StreamErrorClosed                = gen.StreamErrorClosed

	ResultListU8StreamError    = gen.ResultListU8StreamError
	ResultListU8StreamErrorOk  = gen.ResultListU8StreamErrorOk
	ResultListU8StreamErrorErr = gen.ResultListU8StreamErrorErr

	ResultU64StreamError    = gen.ResultU64StreamError
	ResultU64StreamErrorOk  = gen.ResultU64StreamErrorOk
	ResultU64StreamErrorErr = gen.ResultU64StreamErrorErr

	Result_StreamError    = gen.Result_StreamError
	Result_StreamErrorOk  = gen.Result_StreamErrorOk
	Result_StreamErrorErr = gen.Result_StreamErrorErr
)

func NewInputStreamHandle(impl InputStream) *InputStreamHandle {
	return gen.NewInputStreamHandle(impl)
}
func NewInputStreamHandleIn(definer *host.ComponentInstance, impl InputStream) *InputStreamHandle {
	return gen.NewInputStreamHandleIn(definer, impl)
}
func NewOutputStreamHandle(impl OutputStream) *OutputStreamHandle {
	return gen.NewOutputStreamHandle(impl)
}
func NewOutputStreamHandleIn(definer *host.ComponentInstance, impl OutputStream) *OutputStreamHandle {
	return gen.NewOutputStreamHandleIn(definer, impl)
}

type impl struct{}

type inputStreamImpl struct{}

func (inputStreamImpl) Read(_ context.Context, _ uint64) (ResultListU8StreamError, error) {
	return nil, fmt.Errorf("wasi:io/streams.input-stream.read: %w", wasierr.ErrNotImplemented)
}
func (inputStreamImpl) BlockingRead(_ context.Context, _ uint64) (ResultListU8StreamError, error) {
	return nil, fmt.Errorf("wasi:io/streams.input-stream.blocking-read: %w", wasierr.ErrNotImplemented)
}
func (inputStreamImpl) Skip(_ context.Context, _ uint64) (ResultU64StreamError, error) {
	return nil, fmt.Errorf("wasi:io/streams.input-stream.skip: %w", wasierr.ErrNotImplemented)
}
func (inputStreamImpl) BlockingSkip(_ context.Context, _ uint64) (ResultU64StreamError, error) {
	return nil, fmt.Errorf("wasi:io/streams.input-stream.blocking-skip: %w", wasierr.ErrNotImplemented)
}
func (inputStreamImpl) Subscribe(_ context.Context) (*poll.PollableHandle, error) {
	return nil, fmt.Errorf("wasi:io/streams.input-stream.subscribe: %w", wasierr.ErrNotImplemented)
}

type outputStreamImpl struct{}

func (outputStreamImpl) CheckWrite(_ context.Context) (ResultU64StreamError, error) {
	return nil, fmt.Errorf("wasi:io/streams.output-stream.check-write: %w", wasierr.ErrNotImplemented)
}
func (outputStreamImpl) Write(_ context.Context, _ []uint8) (Result_StreamError, error) {
	return nil, fmt.Errorf("wasi:io/streams.output-stream.write: %w", wasierr.ErrNotImplemented)
}
func (outputStreamImpl) BlockingWriteAndFlush(_ context.Context, _ []uint8) (Result_StreamError, error) {
	return nil, fmt.Errorf("wasi:io/streams.output-stream.blocking-write-and-flush: %w", wasierr.ErrNotImplemented)
}
func (outputStreamImpl) Flush(_ context.Context) (Result_StreamError, error) {
	return nil, fmt.Errorf("wasi:io/streams.output-stream.flush: %w", wasierr.ErrNotImplemented)
}
func (outputStreamImpl) BlockingFlush(_ context.Context) (Result_StreamError, error) {
	return nil, fmt.Errorf("wasi:io/streams.output-stream.blocking-flush: %w", wasierr.ErrNotImplemented)
}
func (outputStreamImpl) WriteZeroes(_ context.Context, _ uint64) (Result_StreamError, error) {
	return nil, fmt.Errorf("wasi:io/streams.output-stream.write-zeroes: %w", wasierr.ErrNotImplemented)
}
func (outputStreamImpl) BlockingWriteZeroesAndFlush(_ context.Context, _ uint64) (Result_StreamError, error) {
	return nil, fmt.Errorf("wasi:io/streams.output-stream.blocking-write-zeroes-and-flush: %w", wasierr.ErrNotImplemented)
}
func (outputStreamImpl) Splice(_ context.Context, _ *InputStreamHandle, _ uint64) (ResultU64StreamError, error) {
	return nil, fmt.Errorf("wasi:io/streams.output-stream.splice: %w", wasierr.ErrNotImplemented)
}
func (outputStreamImpl) BlockingSplice(_ context.Context, _ *InputStreamHandle, _ uint64) (ResultU64StreamError, error) {
	return nil, fmt.Errorf("wasi:io/streams.output-stream.blocking-splice: %w", wasierr.ErrNotImplemented)
}
func (outputStreamImpl) Subscribe(_ context.Context) (*poll.PollableHandle, error) {
	return nil, fmt.Errorf("wasi:io/streams.output-stream.subscribe: %w", wasierr.ErrNotImplemented)
}

var (
	_ gen.Streams      = impl{}
	_ gen.InputStream  = inputStreamImpl{}
	_ gen.OutputStream = outputStreamImpl{}
)

func NewInstance(
	ctx context.Context,
	engine *wacogo.Engine,
	errorInst *host.ComponentInstance,
	pollInst *host.ComponentInstance,
) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, &gen.Deps{
		Error: errorInst.Core(),
		Poll:  pollInst.Core(),
	})
}
