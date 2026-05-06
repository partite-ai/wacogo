package types

import (
	"context"
	"net/http"

	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/wasi/gen/wasi/http/types"
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
	consumed    bool
	errorInst   *host.ComponentInstance
	pollInst    *host.ComponentInstance
	streamsInst *host.ComponentInstance
}

func (b *incomingBodyImpl) Stream(_ context.Context) (ResultInputStream_, error) {
	if b.consumed {
		return types.ResultInputStream_Err{}, nil
	}
	b.consumed = true
	h := streams.NewInputStreamHandleIn(b.streamsInst, streams.NewIOReaderInputStream(b.errorInst, b.pollInst, b.resp.Body))
	return types.ResultInputStream_Ok{Value: h}, nil
}
