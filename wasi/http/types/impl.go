package types

import (
	"context"
	"fmt"
	"net/http"

	"github.com/partite-ai/wacogo/host"
	wasierr "github.com/partite-ai/wacogo/wasi/internal/wasierr"
	"github.com/partite-ai/wacogo/internal/wasi/gen/wasi/http/types"
	"github.com/partite-ai/wacogo/internal/wasi/gen/wasi/io/wioerror"
)

// impl is the top-level Types implementation
type impl struct {
	streamInst *host.ComponentInstance
	pollInst   *host.ComponentInstance
	errorInst  *host.ComponentInstance
}

func (i *impl) HTTPErrorCode(_ context.Context, _ *wioerror.ErrorResourceHandle) (OptionErrorCode, error) {
	return OptionErrorCode{}, fmt.Errorf("wasi:http/types.http-error-code: %w", wasierr.ErrNotImplemented)
}

func (i *impl) NewFields(_ context.Context) (*FieldsHandle, error) {
	return NewFieldsHandle(newFieldsImpl(make(http.Header), false)), nil
}

func (i *impl) FieldsFromList(_ context.Context, values []TupleStringListU8) (ResultFieldsHeaderError, error) {
	headers := make(http.Header)
	for _, tuple := range values {
		headers.Add(tuple.F0, string(tuple.F1))
	}
	return types.ResultFieldsHeaderErrorOk{
		Value: NewFieldsHandle(newFieldsImpl(headers, false)),
	}, nil
}

func (i *impl) NewOutgoingRequest(ctx context.Context, fields *FieldsHandle) (*OutgoingRequestHandle, error) {
	defer fields.Drop(ctx)

	h, err := headersFromFields(ctx, fields)
	if err != nil {
		return nil, err
	}
	ri := newOutgoingRequestImpl(h, i.streamInst, i.pollInst, i.errorInst)
	return NewOutgoingRequestHandle(ri), nil
}

func (i *impl) NewRequestOptions(_ context.Context) (*RequestOptionsHandle, error) {
	impl := &requestOptionsImpl{}
	return NewRequestOptionsHandle(impl), nil
}

func (i *impl) ResponseOutparamSet(_ context.Context, _ *ResponseOutparamHandle, _ ResultOutgoingResponseErrorCode) error {
	return fmt.Errorf("wasi:http/types.response-outparam.set: %w", wasierr.ErrNotImplemented)
}

func (i *impl) IncomingBodyFinish(_ context.Context, _ *IncomingBodyHandle) (*FutureTrailersHandle, error) {
	return nil, fmt.Errorf("wasi:http/types.incoming-body.finish: %w", wasierr.ErrNotImplemented)
}

func (i *impl) NewOutgoingResponse(_ context.Context, _ *FieldsHandle) (*OutgoingResponseHandle, error) {
	return nil, fmt.Errorf("wasi:http/types.outgoing-response.constructor: %w", wasierr.ErrNotImplemented)
}

func (i *impl) OutgoingBodyFinish(ctx context.Context, bh *OutgoingBodyHandle, trailers OptionFields) (Result_ErrorCode, error) {
	defer bh.Drop(ctx)

	impl, ok := bh.LocalImpl()
	if !ok {
		return nil, fmt.Errorf("wasi:http/types.outgoing-body.finish: unknown outgoing body handle")
	}
	obImpl, ok := impl.(*outgoingBodyImpl)
	if !ok {
		return nil, fmt.Errorf("wasi:http/types.outgoing-body.finish: invalid outgoing body handle")
	}

	if trailers.IsSome {
		defer trailers.Value.Drop(ctx)
		h, err := headersFromFields(ctx, trailers.Value)
		if err != nil {
			return nil, err
		}
		obImpl.request.r.Trailer = h
	}

	if !obImpl.streamDropped {
		obImpl.streamDropped = true // prevent the finalizer from panicking
		return nil, fmt.Errorf("wasi:http/types.outgoing-body.finish: body stream not dropped")
	}

	if obImpl.writeOpened {
		obImpl.writer.Close()
		obImpl.writeOpened = false
		obImpl.writer = nil
	}

	return types.Result_ErrorCodeOk{}, nil
}
