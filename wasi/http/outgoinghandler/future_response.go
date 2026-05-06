package outgoinghandler

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"syscall"

	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/wasi/http/types"
	"github.com/partite-ai/wacogo/wasi/io/poll"
)

type httpResponseFuture struct {
	resp *http.Response
	err  error
}

type futureIncomingResponseImpl struct {
	availableCh     chan struct{}
	result          *httpResponseFuture
	deliveredResult bool
	typesInst       *host.ComponentInstance
	errorInst       *host.ComponentInstance
	pollInst        *host.ComponentInstance
	streamsInst     *host.ComponentInstance
}

func (f *futureIncomingResponseImpl) Get(ctx context.Context) (types.OptionResultResultIncomingResponseErrorCode_, error) {
	/// Returns the incoming HTTP Response, or an error, once one is ready.
	///
	/// The outer `option` represents future readiness. Users can wait on this
	/// `option` to become `some` using the `subscribe` method.
	///
	/// The outer `result` is used to retrieve the response or error at most
	/// once. It will be success on the first call in which the outer option
	/// is `some`, and error on subsequent calls.
	///
	/// The inner `result` represents that either the incoming HTTP Response
	/// status and headers have received successfully, or that an error
	/// occurred. Errors may also occur while consuming the response body,
	/// but those will be reported by the `incoming-body` and its

	select {
	case <-f.availableCh:
	case <-ctx.Done():
		return types.OptionResultResultIncomingResponseErrorCode_{}, ctx.Err()
	default:
		// The future is not yet ready, so the option is none.
		return types.OptionResultResultIncomingResponseErrorCode_{}, nil
	}

	if f.deliveredResult {
		// The future is ready, but the result has already been delivered once, so return an error.
		return types.OptionResultResultIncomingResponseErrorCode_{
			IsSome: true,
			Value:  types.ResultResultIncomingResponseErrorCode_Err{},
		}, nil
	}

	f.deliveredResult = true

	result, err := f.result.resp, f.result.err
	if err != nil {
		return types.OptionResultResultIncomingResponseErrorCode_{
			IsSome: true,
			Value: types.ResultResultIncomingResponseErrorCode_Ok{
				Value: types.ResultIncomingResponseErrorCodeErr{
					Value: translateResultErr(err),
				},
			},
		}, nil
	}

	resp := types.NewIncomingResponse(result, f.errorInst, f.pollInst, f.streamsInst)

	return types.OptionResultResultIncomingResponseErrorCode_{
		IsSome: true,
		Value: types.ResultResultIncomingResponseErrorCode_Ok{
			Value: types.ResultIncomingResponseErrorCodeOk{
				Value: types.NewIncomingResponseHandleIn(f.typesInst, resp),
			},
		},
	}, nil
}

func (f *futureIncomingResponseImpl) Subscribe(ctx context.Context) (*poll.PollableHandle, error) {
	return poll.NewPollableHandleIn(f.pollInst, &futureResponsePollable{future: f}), nil
}

type futureResponsePollable struct {
	future *futureIncomingResponseImpl
}

func (p *futureResponsePollable) Done() <-chan struct{} {
	return p.future.availableCh
}

func (p *futureResponsePollable) Block(ctx context.Context) error {
	select {
	case <-p.future.availableCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *futureResponsePollable) Ready(ctx context.Context) (bool, error) {
	select {
	case <-p.future.availableCh:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	default:
		return false, nil
	}
}

func translateResultErr(err error) types.ErrorCode {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return types.ErrorCodeConnectionTimeout{}
	}
	if errors.Is(err, context.Canceled) {
		return types.ErrorCodeInternalError{Value: types.SomeString(err.Error())}
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		switch {
		case dnsErr.IsTimeout:
			return types.ErrorCodeDNSTimeout{}
		case dnsErr.IsNotFound:
			return types.ErrorCodeDestinationNotFound{}
		}
		return types.ErrorCodeDNSError{
			Value: types.DNSErrorPayload{Rcode: types.SomeString(dnsErr.Err)},
		}
	}

	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return types.ErrorCodeTLSCertificateError{}
	}
	var recordHeaderErr tls.RecordHeaderError
	if errors.As(err, &recordHeaderErr) {
		return types.ErrorCodeTLSProtocolError{}
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Timeout() {
			switch opErr.Op {
			case "read":
				return types.ErrorCodeConnectionReadTimeout{}
			case "write":
				return types.ErrorCodeConnectionWriteTimeout{}
			}
			return types.ErrorCodeConnectionTimeout{}
		}
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) {
			if errno, ok := sysErr.Err.(syscall.Errno); ok {
				switch errno {
				case syscall.ECONNREFUSED:
					return types.ErrorCodeConnectionRefused{}
				case syscall.ECONNRESET, syscall.EPIPE:
					return types.ErrorCodeConnectionTerminated{}
				case syscall.ETIMEDOUT:
					return types.ErrorCodeConnectionTimeout{}
				case syscall.EHOSTUNREACH, syscall.ENETUNREACH:
					return types.ErrorCodeDestinationIPUnroutable{}
				}
			}
		}
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return types.ErrorCodeConnectionTimeout{}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return types.ErrorCodeConnectionTimeout{}
	}

	return types.ErrorCodeInternalError{Value: types.SomeString(err.Error())}
}
