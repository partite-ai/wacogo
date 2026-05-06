// Package types is the not-yet-implemented wasi:http/types host
// component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package types

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	wasierr "github.com/partite-ai/wacogo/wasi/internal/wasierr"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/http/types"
	"github.com/partite-ai/wacogo/wasi/io/poll"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	Types = gen.Types

	Fields                       = gen.Fields
	FieldsHandle                 = gen.FieldsHandle
	IncomingRequest              = gen.IncomingRequest
	IncomingRequestHandle        = gen.IncomingRequestHandle
	OutgoingRequest              = gen.OutgoingRequest
	OutgoingRequestHandle        = gen.OutgoingRequestHandle
	RequestOptions               = gen.RequestOptions
	RequestOptionsHandle         = gen.RequestOptionsHandle
	ResponseOutparam             = gen.ResponseOutparam
	ResponseOutparamHandle       = gen.ResponseOutparamHandle
	IncomingResponse             = gen.IncomingResponse
	IncomingResponseHandle       = gen.IncomingResponseHandle
	IncomingBody                 = gen.IncomingBody
	IncomingBodyHandle           = gen.IncomingBodyHandle
	FutureTrailers               = gen.FutureTrailers
	FutureTrailersHandle         = gen.FutureTrailersHandle
	OutgoingResponse             = gen.OutgoingResponse
	OutgoingResponseHandle       = gen.OutgoingResponseHandle
	OutgoingBody                 = gen.OutgoingBody
	OutgoingBodyHandle           = gen.OutgoingBodyHandle
	FutureIncomingResponse       = gen.FutureIncomingResponse
	FutureIncomingResponseHandle = gen.FutureIncomingResponseHandle

	OptionErrorCode                              = gen.OptionErrorCode
	OptionFieldSizePayload                       = gen.OptionFieldSizePayload
	OptionFields                                 = gen.OptionFields
	OptionResultResultIncomingResponseErrorCode_ = gen.OptionResultResultIncomingResponseErrorCode_
	OptionResultResultOptionFieldsErrorCode_     = gen.OptionResultResultOptionFieldsErrorCode_
	OptionScheme                                 = gen.OptionScheme
	OptionString                                 = gen.OptionString
	OptionU16                                    = gen.OptionU16
	OptionU32                                    = gen.OptionU32
	OptionU64                                    = gen.OptionU64
	OptionU8                                     = gen.OptionU8

	ResultFieldsHeaderError    = gen.ResultFieldsHeaderError
	ResultFieldsHeaderErrorOk  = gen.ResultFieldsHeaderErrorOk
	ResultFieldsHeaderErrorErr = gen.ResultFieldsHeaderErrorErr

	ResultIncomingBody_    = gen.ResultIncomingBody_
	ResultIncomingBody_Ok  = gen.ResultIncomingBody_Ok
	ResultIncomingBody_Err = gen.ResultIncomingBody_Err

	ResultIncomingResponseErrorCode    = gen.ResultIncomingResponseErrorCode
	ResultIncomingResponseErrorCodeOk  = gen.ResultIncomingResponseErrorCodeOk
	ResultIncomingResponseErrorCodeErr = gen.ResultIncomingResponseErrorCodeErr

	ResultInputStream_    = gen.ResultInputStream_
	ResultInputStream_Ok  = gen.ResultInputStream_Ok
	ResultInputStream_Err = gen.ResultInputStream_Err

	ResultOptionFieldsErrorCode    = gen.ResultOptionFieldsErrorCode
	ResultOptionFieldsErrorCodeOk  = gen.ResultOptionFieldsErrorCodeOk
	ResultOptionFieldsErrorCodeErr = gen.ResultOptionFieldsErrorCodeErr

	ResultOutgoingBody_    = gen.ResultOutgoingBody_
	ResultOutgoingBody_Ok  = gen.ResultOutgoingBody_Ok
	ResultOutgoingBody_Err = gen.ResultOutgoingBody_Err

	ResultOutgoingResponseErrorCode    = gen.ResultOutgoingResponseErrorCode
	ResultOutgoingResponseErrorCodeOk  = gen.ResultOutgoingResponseErrorCodeOk
	ResultOutgoingResponseErrorCodeErr = gen.ResultOutgoingResponseErrorCodeErr

	ResultOutputStream_    = gen.ResultOutputStream_
	ResultOutputStream_Ok  = gen.ResultOutputStream_Ok
	ResultOutputStream_Err = gen.ResultOutputStream_Err

	ResultResultIncomingResponseErrorCode_    = gen.ResultResultIncomingResponseErrorCode_
	ResultResultIncomingResponseErrorCode_Ok  = gen.ResultResultIncomingResponseErrorCode_Ok
	ResultResultIncomingResponseErrorCode_Err = gen.ResultResultIncomingResponseErrorCode_Err

	ResultResultOptionFieldsErrorCode_    = gen.ResultResultOptionFieldsErrorCode_
	ResultResultOptionFieldsErrorCode_Ok  = gen.ResultResultOptionFieldsErrorCode_Ok
	ResultResultOptionFieldsErrorCode_Err = gen.ResultResultOptionFieldsErrorCode_Err

	Result_ErrorCode    = gen.Result_ErrorCode
	Result_ErrorCodeOk  = gen.Result_ErrorCodeOk
	Result_ErrorCodeErr = gen.Result_ErrorCodeErr

	Result_HeaderError    = gen.Result_HeaderError
	Result_HeaderErrorOk  = gen.Result_HeaderErrorOk
	Result_HeaderErrorErr = gen.Result_HeaderErrorErr

	Result__    = gen.Result__
	Result__Ok  = gen.Result__Ok
	Result__Err = gen.Result__Err

	DNSErrorPayload         = gen.DNSErrorPayload
	TLSAlertReceivedPayload = gen.TLSAlertReceivedPayload
	FieldSizePayload        = gen.FieldSizePayload
	TupleStringListU8       = gen.TupleStringListU8

	Method        = gen.Method
	MethodGet     = gen.MethodGet
	MethodHead    = gen.MethodHead
	MethodPost    = gen.MethodPost
	MethodPut     = gen.MethodPut
	MethodDelete  = gen.MethodDelete
	MethodConnect = gen.MethodConnect
	MethodOptions = gen.MethodOptions
	MethodTrace   = gen.MethodTrace
	MethodPatch   = gen.MethodPatch
	MethodOther   = gen.MethodOther

	Scheme      = gen.Scheme
	SchemeHTTP  = gen.SchemeHTTP
	SchemeHTTPS = gen.SchemeHTTPS
	SchemeOther = gen.SchemeOther

	ErrorCode                               = gen.ErrorCode
	ErrorCodeDNSTimeout                     = gen.ErrorCodeDNSTimeout
	ErrorCodeDNSError                       = gen.ErrorCodeDNSError
	ErrorCodeDestinationNotFound            = gen.ErrorCodeDestinationNotFound
	ErrorCodeDestinationUnavailable         = gen.ErrorCodeDestinationUnavailable
	ErrorCodeDestinationIPProhibited        = gen.ErrorCodeDestinationIPProhibited
	ErrorCodeDestinationIPUnroutable        = gen.ErrorCodeDestinationIPUnroutable
	ErrorCodeConnectionRefused              = gen.ErrorCodeConnectionRefused
	ErrorCodeConnectionTerminated           = gen.ErrorCodeConnectionTerminated
	ErrorCodeConnectionTimeout              = gen.ErrorCodeConnectionTimeout
	ErrorCodeConnectionReadTimeout          = gen.ErrorCodeConnectionReadTimeout
	ErrorCodeConnectionWriteTimeout         = gen.ErrorCodeConnectionWriteTimeout
	ErrorCodeConnectionLimitReached         = gen.ErrorCodeConnectionLimitReached
	ErrorCodeTLSProtocolError               = gen.ErrorCodeTLSProtocolError
	ErrorCodeTLSCertificateError            = gen.ErrorCodeTLSCertificateError
	ErrorCodeTLSAlertReceived               = gen.ErrorCodeTLSAlertReceived
	ErrorCodeHTTPRequestDenied              = gen.ErrorCodeHTTPRequestDenied
	ErrorCodeHTTPRequestLengthRequired      = gen.ErrorCodeHTTPRequestLengthRequired
	ErrorCodeHTTPRequestBodySize            = gen.ErrorCodeHTTPRequestBodySize
	ErrorCodeHTTPRequestMethodInvalid       = gen.ErrorCodeHTTPRequestMethodInvalid
	ErrorCodeHTTPRequestURIInvalid          = gen.ErrorCodeHTTPRequestURIInvalid
	ErrorCodeHTTPRequestURITooLong          = gen.ErrorCodeHTTPRequestURITooLong
	ErrorCodeHTTPRequestHeaderSectionSize   = gen.ErrorCodeHTTPRequestHeaderSectionSize
	ErrorCodeHTTPRequestHeaderSize          = gen.ErrorCodeHTTPRequestHeaderSize
	ErrorCodeHTTPRequestTrailerSectionSize  = gen.ErrorCodeHTTPRequestTrailerSectionSize
	ErrorCodeHTTPRequestTrailerSize         = gen.ErrorCodeHTTPRequestTrailerSize
	ErrorCodeHTTPResponseIncomplete         = gen.ErrorCodeHTTPResponseIncomplete
	ErrorCodeHTTPResponseHeaderSectionSize  = gen.ErrorCodeHTTPResponseHeaderSectionSize
	ErrorCodeHTTPResponseHeaderSize         = gen.ErrorCodeHTTPResponseHeaderSize
	ErrorCodeHTTPResponseBodySize           = gen.ErrorCodeHTTPResponseBodySize
	ErrorCodeHTTPResponseTrailerSectionSize = gen.ErrorCodeHTTPResponseTrailerSectionSize
	ErrorCodeHTTPResponseTrailerSize        = gen.ErrorCodeHTTPResponseTrailerSize
	ErrorCodeHTTPResponseTransferCoding     = gen.ErrorCodeHTTPResponseTransferCoding
	ErrorCodeHTTPResponseContentCoding      = gen.ErrorCodeHTTPResponseContentCoding
	ErrorCodeHTTPResponseTimeout            = gen.ErrorCodeHTTPResponseTimeout
	ErrorCodeHTTPUpgradeFailed              = gen.ErrorCodeHTTPUpgradeFailed
	ErrorCodeHTTPProtocolError              = gen.ErrorCodeHTTPProtocolError
	ErrorCodeLoopDetected                   = gen.ErrorCodeLoopDetected
	ErrorCodeConfigurationError             = gen.ErrorCodeConfigurationError
	ErrorCodeInternalError                  = gen.ErrorCodeInternalError

	HeaderError              = gen.HeaderError
	HeaderErrorInvalidSyntax = gen.HeaderErrorInvalidSyntax
	HeaderErrorForbidden     = gen.HeaderErrorForbidden
	HeaderErrorImmutable     = gen.HeaderErrorImmutable
)

// Option constructors (re-exported from the generated bindings).
var (
	SomeErrorCode                              = gen.SomeErrorCode
	NoneErrorCode                              = gen.NoneErrorCode
	SomeFieldSizePayload                       = gen.SomeFieldSizePayload
	NoneFieldSizePayload                       = gen.NoneFieldSizePayload
	SomeFields                                 = gen.SomeFields
	NoneFields                                 = gen.NoneFields
	SomeResultResultIncomingResponseErrorCode_ = gen.SomeResultResultIncomingResponseErrorCode_
	NoneResultResultIncomingResponseErrorCode_ = gen.NoneResultResultIncomingResponseErrorCode_
	SomeResultResultOptionFieldsErrorCode_     = gen.SomeResultResultOptionFieldsErrorCode_
	NoneResultResultOptionFieldsErrorCode_     = gen.NoneResultResultOptionFieldsErrorCode_
	SomeScheme                                 = gen.SomeScheme
	NoneScheme                                 = gen.NoneScheme
	SomeString                                 = gen.SomeString
	NoneString                                 = gen.NoneString
	SomeU16                                    = gen.SomeU16
	NoneU16                                    = gen.NoneU16
	SomeU32                                    = gen.SomeU32
	NoneU32                                    = gen.NoneU32
	SomeU64                                    = gen.SomeU64
	NoneU64                                    = gen.NoneU64
	SomeU8                                     = gen.SomeU8
	NoneU8                                     = gen.NoneU8
)

func NewFieldsHandle(impl Fields) *FieldsHandle {
	return gen.NewFieldsHandle(impl)
}
func NewFieldsHandleIn(definer *host.ComponentInstance, impl Fields) *FieldsHandle {
	return gen.NewFieldsHandleIn(definer, impl)
}

func NewIncomingRequestHandle(impl IncomingRequest) *IncomingRequestHandle {
	return gen.NewIncomingRequestHandle(impl)
}
func NewIncomingRequestHandleIn(definer *host.ComponentInstance, impl IncomingRequest) *IncomingRequestHandle {
	return gen.NewIncomingRequestHandleIn(definer, impl)
}

func NewOutgoingRequestHandle(impl OutgoingRequest) *OutgoingRequestHandle {
	return gen.NewOutgoingRequestHandle(impl)
}
func NewOutgoingRequestHandleIn(definer *host.ComponentInstance, impl OutgoingRequest) *OutgoingRequestHandle {
	return gen.NewOutgoingRequestHandleIn(definer, impl)
}

func NewRequestOptionsHandle(impl RequestOptions) *RequestOptionsHandle {
	return gen.NewRequestOptionsHandle(impl)
}
func NewRequestOptionsHandleIn(definer *host.ComponentInstance, impl RequestOptions) *RequestOptionsHandle {
	return gen.NewRequestOptionsHandleIn(definer, impl)
}

func NewResponseOutparamHandle(impl ResponseOutparam) *ResponseOutparamHandle {
	return gen.NewResponseOutparamHandle(impl)
}
func NewResponseOutparamHandleIn(definer *host.ComponentInstance, impl ResponseOutparam) *ResponseOutparamHandle {
	return gen.NewResponseOutparamHandleIn(definer, impl)
}

func NewIncomingResponseHandle(impl IncomingResponse) *IncomingResponseHandle {
	return gen.NewIncomingResponseHandle(impl)
}
func NewIncomingResponseHandleIn(definer *host.ComponentInstance, impl IncomingResponse) *IncomingResponseHandle {
	return gen.NewIncomingResponseHandleIn(definer, impl)
}

func NewIncomingBodyHandle(impl IncomingBody) *IncomingBodyHandle {
	return gen.NewIncomingBodyHandle(impl)
}
func NewIncomingBodyHandleIn(definer *host.ComponentInstance, impl IncomingBody) *IncomingBodyHandle {
	return gen.NewIncomingBodyHandleIn(definer, impl)
}

func NewFutureTrailersHandle(impl FutureTrailers) *FutureTrailersHandle {
	return gen.NewFutureTrailersHandle(impl)
}
func NewFutureTrailersHandleIn(definer *host.ComponentInstance, impl FutureTrailers) *FutureTrailersHandle {
	return gen.NewFutureTrailersHandleIn(definer, impl)
}

func NewOutgoingResponseHandle(impl OutgoingResponse) *OutgoingResponseHandle {
	return gen.NewOutgoingResponseHandle(impl)
}
func NewOutgoingResponseHandleIn(definer *host.ComponentInstance, impl OutgoingResponse) *OutgoingResponseHandle {
	return gen.NewOutgoingResponseHandleIn(definer, impl)
}

func NewOutgoingBodyHandle(impl OutgoingBody) *OutgoingBodyHandle {
	return gen.NewOutgoingBodyHandle(impl)
}
func NewOutgoingBodyHandleIn(definer *host.ComponentInstance, impl OutgoingBody) *OutgoingBodyHandle {
	return gen.NewOutgoingBodyHandleIn(definer, impl)
}

func NewFutureIncomingResponseHandle(impl FutureIncomingResponse) *FutureIncomingResponseHandle {
	return gen.NewFutureIncomingResponseHandle(impl)
}
func NewFutureIncomingResponseHandleIn(definer *host.ComponentInstance, impl FutureIncomingResponse) *FutureIncomingResponseHandle {
	return gen.NewFutureIncomingResponseHandleIn(definer, impl)
}

// incomingRequestImpl is the IncomingRequest resource implementation returning ErrNotImplemented.
type incomingRequestImpl struct{}

func (incomingRequestImpl) Authority(_ context.Context) (OptionString, error) {
	return OptionString{}, fmt.Errorf("wasi:http/types.incoming-request.authority: %w", wasierr.ErrNotImplemented)
}
func (incomingRequestImpl) Consume(_ context.Context) (ResultIncomingBody_, error) {
	return nil, fmt.Errorf("wasi:http/types.incoming-request.consume: %w", wasierr.ErrNotImplemented)
}
func (incomingRequestImpl) Headers(_ context.Context) (*FieldsHandle, error) {
	return nil, fmt.Errorf("wasi:http/types.incoming-request.headers: %w", wasierr.ErrNotImplemented)
}
func (incomingRequestImpl) Method(_ context.Context) (Method, error) {
	return nil, fmt.Errorf("wasi:http/types.incoming-request.method: %w", wasierr.ErrNotImplemented)
}
func (incomingRequestImpl) PathWithQuery(_ context.Context) (OptionString, error) {
	return OptionString{}, fmt.Errorf("wasi:http/types.incoming-request.path-with-query: %w", wasierr.ErrNotImplemented)
}
func (incomingRequestImpl) Scheme(_ context.Context) (OptionScheme, error) {
	return OptionScheme{}, fmt.Errorf("wasi:http/types.incoming-request.scheme: %w", wasierr.ErrNotImplemented)
}

// responseOutparamImpl is the ResponseOutparam resource implementation returning ErrNotImplemented.
type responseOutparamImpl struct{}

func (responseOutparamImpl) SendInformational(_ context.Context, _ uint16, _ *FieldsHandle) (Result_ErrorCode, error) {
	return nil, fmt.Errorf("wasi:http/types.response-outparam.send-informational: %w", wasierr.ErrNotImplemented)
}

// futureTrailersImpl is the FutureTrailers resource implementation returning ErrNotImplemented.
type futureTrailersImpl struct{}

func (futureTrailersImpl) Get(_ context.Context) (OptionResultResultOptionFieldsErrorCode_, error) {
	return OptionResultResultOptionFieldsErrorCode_{}, fmt.Errorf("wasi:http/types.future-trailers.get: %w", wasierr.ErrNotImplemented)
}
func (futureTrailersImpl) Subscribe(_ context.Context) (*poll.PollableHandle, error) {
	return nil, fmt.Errorf("wasi:http/types.future-trailers.subscribe: %w", wasierr.ErrNotImplemented)
}

// outgoingResponseImpl is the OutgoingResponse resource implementation returning ErrNotImplemented.
type outgoingResponseImpl struct{}

func (outgoingResponseImpl) Body(_ context.Context) (ResultOutgoingBody_, error) {
	return nil, fmt.Errorf("wasi:http/types.outgoing-response.body: %w", wasierr.ErrNotImplemented)
}
func (outgoingResponseImpl) Headers(_ context.Context) (*FieldsHandle, error) {
	return nil, fmt.Errorf("wasi:http/types.outgoing-response.headers: %w", wasierr.ErrNotImplemented)
}
func (outgoingResponseImpl) SetStatusCode(_ context.Context, _ uint16) (Result__, error) {
	return nil, fmt.Errorf("wasi:http/types.outgoing-response.set-status-code: %w", wasierr.ErrNotImplemented)
}
func (outgoingResponseImpl) StatusCode(_ context.Context) (uint16, error) {
	return 0, fmt.Errorf("wasi:http/types.outgoing-response.status-code: %w", wasierr.ErrNotImplemented)
}

// futureIncomingResponseImpl is the FutureIncomingResponse resource implementation returning ErrNotImplemented.
type futureIncomingResponseImpl struct{}

func (futureIncomingResponseImpl) Get(_ context.Context) (OptionResultResultIncomingResponseErrorCode_, error) {
	return OptionResultResultIncomingResponseErrorCode_{}, fmt.Errorf("wasi:http/types.future-incoming-response.get: %w", wasierr.ErrNotImplemented)
}
func (futureIncomingResponseImpl) Subscribe(_ context.Context) (*poll.PollableHandle, error) {
	return nil, fmt.Errorf("wasi:http/types.future-incoming-response.subscribe: %w", wasierr.ErrNotImplemented)
}

var (
	_ gen.Types                  = &impl{}
	_ gen.Fields                 = &fieldsImpl{}
	_ gen.IncomingRequest        = incomingRequestImpl{}
	_ gen.OutgoingRequest        = &outgoingRequestImpl{}
	_ gen.RequestOptions         = &requestOptionsImpl{}
	_ gen.ResponseOutparam       = responseOutparamImpl{}
	_ gen.IncomingResponse       = &incomingResponseImpl{}
	_ gen.IncomingBody           = &incomingBodyImpl{}
	_ gen.FutureTrailers         = futureTrailersImpl{}
	_ gen.OutgoingResponse       = outgoingResponseImpl{}
	_ gen.OutgoingBody           = &outgoingBodyImpl{}
	_ gen.FutureIncomingResponse = futureIncomingResponseImpl{}
)

// NewInstance instantiates the wasi:http/types host component.
func NewInstance(
	ctx context.Context,
	engine *wacogo.Engine,
	errorInst *host.ComponentInstance,
	pollInst *host.ComponentInstance,
	streamsInst *host.ComponentInstance,
) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, &impl{
		streamInst: streamsInst,
		pollInst:   pollInst,
		errorInst:  errorInst,
	}, &gen.Deps{
		Error:   errorInst.Core(),
		Poll:    pollInst.Core(),
		Streams: streamsInst.Core(),
	})
}
