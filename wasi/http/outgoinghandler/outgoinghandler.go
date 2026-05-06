// Package outgoinghandler is the not-yet-implemented wasi:http/outgoing-handler
// host component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package outgoinghandler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/http/outgoinghandler"
	"github.com/partite-ai/wacogo/wasi/http/types"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	OutgoingHandler = gen.OutgoingHandler

	OptionRequestOptions = gen.OptionRequestOptions

	ResultFutureIncomingResponseTypesErrorCode    = gen.ResultFutureIncomingResponseTypesErrorCode
	ResultFutureIncomingResponseTypesErrorCodeOk  = gen.ResultFutureIncomingResponseTypesErrorCodeOk
	ResultFutureIncomingResponseTypesErrorCodeErr = gen.ResultFutureIncomingResponseTypesErrorCodeErr
)

type impl struct {
	httpClient  *http.Client
	typesInst   *host.ComponentInstance
	errorInst   *host.ComponentInstance
	pollInst    *host.ComponentInstance
	streamsInst *host.ComponentInstance
}

func (i *impl) Handle(_ context.Context, rh *types.OutgoingRequestHandle, ro OptionRequestOptions) (ResultFutureIncomingResponseTypesErrorCode, error) {
	rli, ok := rh.LocalImpl()
	if !ok {
		return nil, fmt.Errorf("wasi:http/outgoing-handler.handle: request handle is not a local implementation")
	}
	request, ok := rli.(types.GoOutgoingRequest)
	if !ok {
		return nil, fmt.Errorf("wasi:http/outgoing-handler.handle: request handle's local implementation does not implement types.GoOutgoingRequest")
	}

	httpRequest := request.ToHTTPRequest()

	ch := make(chan struct{})
	respFuture := &futureIncomingResponseImpl{
		availableCh: ch,
		result:      nil, // will be set when the response is received
		typesInst:   i.typesInst,
		errorInst:   i.errorInst,
		pollInst:    i.pollInst,
		streamsInst: i.streamsInst,
	}

	go func() {
		httpResp, err := i.httpClient.Do(httpRequest)

		if err != nil {
			respFuture.result = &httpResponseFuture{resp: nil, err: err}
		} else {
			respFuture.result = &httpResponseFuture{resp: httpResp, err: nil}
		}
		close(respFuture.availableCh)
	}()

	return ResultFutureIncomingResponseTypesErrorCodeOk{
		Value: types.NewFutureIncomingResponseHandleIn(i.typesInst, respFuture),
	}, nil
}

var _ gen.OutgoingHandler = &impl{}

func NewInstance(
	ctx context.Context,
	engine *wacogo.Engine,
	typesInst *host.ComponentInstance,
	errorInst *host.ComponentInstance,
	pollInst *host.ComponentInstance,
	streamsInst *host.ComponentInstance,
	httpClient *http.Client,
) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, &impl{
		httpClient:  httpClient,
		typesInst:   typesInst,
		errorInst:   errorInst,
		pollInst:    pollInst,
		streamsInst: streamsInst,
	}, &gen.Deps{
		Error:   errorInst.Core(),
		Poll:    pollInst.Core(),
		Streams: streamsInst.Core(),
		Types:   typesInst.Core(),
	})
}
