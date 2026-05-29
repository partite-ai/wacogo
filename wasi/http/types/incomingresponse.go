package types

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/wasi/gen/wasi/http/types"
	"github.com/partite-ai/wacogo/wasi/io/poll"
	"github.com/partite-ai/wacogo/wasi/io/streams"
)

type incomingResponseImpl struct {
	resp         *http.Response
	bodyConsumed bool
	errorInst    *host.ComponentInstance
	pollInst     *host.ComponentInstance
	streamsInst  *host.ComponentInstance
}

func NewIncomingResponse(resp *http.Response, errorInst, pollInst, streamsInst *host.ComponentInstance) *incomingResponseImpl {
	return &incomingResponseImpl{
		resp:        resp,
		errorInst:   errorInst,
		pollInst:    pollInst,
		streamsInst: streamsInst,
	}
}

func (r *incomingResponseImpl) Consume(_ context.Context) (ResultIncomingBody_, error) {
	if r.bodyConsumed {
		return types.ResultIncomingBody_Err{}, nil
	}
	r.bodyConsumed = true
	return types.ResultIncomingBody_Ok{
		Value: NewIncomingBodyHandle(&incomingBodyImpl{
			resp:        r.resp,
			errorInst:   r.errorInst,
			pollInst:    r.pollInst,
			streamsInst: r.streamsInst,
		}),
	}, nil
}

func (r *incomingResponseImpl) Headers(_ context.Context) (*FieldsHandle, error) {
	return NewFieldsHandle(newFieldsImpl(r.resp.Header, true)), nil
}

func (r *incomingResponseImpl) Status(_ context.Context) (uint16, error) {
	return uint16(r.resp.StatusCode), nil
}

type incomingBodyImpl struct {
	resp        *http.Response
	stream      *incomingBodyStreamImpl
	errorInst   *host.ComponentInstance
	pollInst    *host.ComponentInstance
	streamsInst *host.ComponentInstance
}

// Drop matches the Drop(ctx) error shape that the witgen-emitted
// resourceDtor type-asserts on guest-driven resource.drop, and that
// the host-extern-table drain calls at close. The spec requires the
// body's input-stream child to be dropped first; violating that
// returns an error. When no stream was opened, drain the response
// body and close it so the underlying connection can be reused.
func (b *incomingBodyImpl) Drop(_ context.Context) error {
	if b.stream != nil && !b.stream.closed {
		return fmt.Errorf("wasi:http/types.incoming-body.drop: body stream not closed")
	}
	if b.stream == nil {
		io.Copy(io.Discard, b.resp.Body)
		return b.resp.Body.Close()
	}
	return nil
}

func (b *incomingBodyImpl) Stream(_ context.Context) (ResultInputStream_, error) {
	if b.stream != nil {
		return types.ResultInputStream_Err{}, nil
	}
	b.stream = &incomingBodyStreamImpl{ReadCloser: b.resp.Body}
	h := streams.NewInputStreamHandleIn(b.streamsInst, streams.NewIOReaderInputStream(b.errorInst, b.pollInst, b.stream))
	return types.ResultInputStream_Ok{Value: h}, nil
}

type incomingBodyStreamImpl struct {
	io.ReadCloser
	closed bool
}

func (s *incomingBodyStreamImpl) Close() error {
	s.closed = true
	io.Copy(io.Discard, s.ReadCloser) // ensure the body is fully consumed so that the connection can be reused
	return s.ReadCloser.Close()
}

type futureTrailersImpl struct {
	headers  http.Header
	pollInst *host.ComponentInstance
}

func (f *futureTrailersImpl) Get(ctx context.Context) (OptionResultResultOptionFieldsErrorCode_, error) {
	// Returns a pollable which becomes ready when either the trailers have
	// been received, or an error has occurred. When this pollable is ready,
	// the `get` method will return `some`.
	return OptionResultResultOptionFieldsErrorCode_{}, nil
}

func (f *futureTrailersImpl) Subscribe(ctx context.Context) (*poll.PollableHandle, error) {
	return poll.NewPollableHandleIn(f.pollInst, futureTrailersPollable{}), nil
}

type futureTrailersPollable struct {
}

func (p futureTrailersPollable) Block(ctx context.Context) error {
	return nil
}

func (p futureTrailersPollable) Ready(ctx context.Context) (bool, error) {
	return true, nil
}

func (p futureTrailersPollable) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
