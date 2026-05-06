package types

import (
	"context"
	"net/http"
	"net/url"

	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/wasi/gen/wasi/http/types"
)

type GoOutgoingRequest interface {
	ToHTTPRequest() *http.Request
}

type outgoingRequestImpl struct {
	r          *http.Request
	streamInst *host.ComponentInstance
	pollInst   *host.ComponentInstance
	errorInst  *host.ComponentInstance
	body       *outgoingBodyImpl
}

func newOutgoingRequestImpl(h http.Header, streamInst, pollInst, errorInst *host.ComponentInstance) *outgoingRequestImpl {
	req := &http.Request{
		Method: http.MethodGet,
		URL: &url.URL{
			OmitHost: true,
		},
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     h,
	}
	return &outgoingRequestImpl{
		r:          req,
		streamInst: streamInst,
		pollInst:   pollInst,
		errorInst:  errorInst,
	}
}

func (r *outgoingRequestImpl) ToHTTPRequest() *http.Request {
	if r.body != nil && r.body.writeOpened {
		r.r.Body = r.body.reader
	}
	return r.r
}

func (r *outgoingRequestImpl) Authority(_ context.Context) (OptionString, error) {
	if r.r.URL.Host == "" && r.r.URL.OmitHost {
		return OptionString{}, nil
	}

	host := r.r.URL.Host
	return OptionString{Value: host, IsSome: true}, nil
}

func (r *outgoingRequestImpl) Body(_ context.Context) (ResultOutgoingBody_, error) {
	if r.body != nil {
		return types.ResultOutgoingBody_Err{}, nil
	}
	r.body = newOutgoingBodyImpl(r)
	return types.ResultOutgoingBody_Ok{Value: NewOutgoingBodyHandle(r.body)}, nil
}

func (r *outgoingRequestImpl) Headers(_ context.Context) (*FieldsHandle, error) {
	return NewFieldsHandle(newFieldsImpl(r.r.Header, true)), nil
}

func (r *outgoingRequestImpl) Method(_ context.Context) (Method, error) {
	switch r.r.Method {
	case http.MethodGet:
		return MethodGet{}, nil
	case http.MethodHead:
		return MethodHead{}, nil
	case http.MethodPost:
		return MethodPost{}, nil
	case http.MethodPut:
		return MethodPut{}, nil
	case http.MethodDelete:
		return MethodDelete{}, nil
	case http.MethodConnect:
		return MethodConnect{}, nil
	case http.MethodOptions:
		return MethodOptions{}, nil
	case http.MethodTrace:
		return MethodTrace{}, nil
	case http.MethodPatch:
		return MethodPatch{}, nil
	default:
		return MethodOther{
			Value: string(r.r.Method),
		}, nil
	}
}

func (r *outgoingRequestImpl) PathWithQuery(_ context.Context) (OptionString, error) {
	if r.r.URL.Path == "" && r.r.URL.RawQuery == "" {
		return OptionString{}, nil
	}
	path := r.r.URL.RequestURI()
	return OptionString{Value: path, IsSome: true}, nil
}

func (r *outgoingRequestImpl) Scheme(_ context.Context) (OptionScheme, error) {
	if r.r.URL.Scheme == "" {
		return OptionScheme{}, nil
	}
	scheme := r.r.URL.Scheme
	switch scheme {
	case "http":
		return OptionScheme{Value: SchemeHTTP{}, IsSome: true}, nil
	case "https":
		return OptionScheme{Value: SchemeHTTPS{}, IsSome: true}, nil
	default:
		return OptionScheme{Value: SchemeOther{Value: scheme}, IsSome: true}, nil
	}
}

func (r *outgoingRequestImpl) SetAuthority(_ context.Context, authority OptionString) (Result__, error) {
	if !authority.IsSome {
		r.r.URL.Host = ""
		r.r.URL.User = nil
		r.r.URL.OmitHost = true
		return types.Result__Ok{}, nil
	}

	parsed, err := url.Parse("http://" + authority.Value + "/")
	if err != nil {
		return types.Result__Err{}, nil
	}

	computedAuthority := parsed.Host
	if parsed.User != nil {
		computedAuthority = parsed.User.String() + "@" + computedAuthority
	}

	if computedAuthority != authority.Value {
		return types.Result__Err{}, nil
	}

	r.r.URL.Host = parsed.Host
	r.r.URL.User = parsed.User
	r.r.URL.OmitHost = parsed.OmitHost
	return types.Result__Ok{}, nil
}

func (r *outgoingRequestImpl) SetMethod(_ context.Context, method Method) (Result__, error) {
	switch m := method.(type) {
	case MethodGet:
		r.r.Method = http.MethodGet
	case MethodHead:
		r.r.Method = http.MethodHead
	case MethodPost:
		r.r.Method = http.MethodPost
	case MethodPut:
		r.r.Method = http.MethodPut
	case MethodDelete:
		r.r.Method = http.MethodDelete
	case MethodConnect:
		r.r.Method = http.MethodConnect
	case MethodOptions:
		r.r.Method = http.MethodOptions
	case MethodTrace:
		r.r.Method = http.MethodTrace
	case MethodPatch:
		r.r.Method = http.MethodPatch
	case MethodOther:
		r.r.Method = m.Value
	default:
		return types.Result__Err{}, nil
	}
	return types.Result__Ok{}, nil
}

func (r *outgoingRequestImpl) SetPathWithQuery(_ context.Context, pathQuery OptionString) (Result__, error) {
	if !pathQuery.IsSome {
		r.r.URL.Path = ""
		r.r.URL.RawQuery = ""
		return types.Result__Ok{}, nil
	}

	parsed, err := url.Parse(pathQuery.Value)
	if err != nil {
		return types.Result__Err{}, nil
	}

	r.r.URL.Path = parsed.Path
	r.r.URL.RawQuery = parsed.RawQuery
	return types.Result__Ok{}, nil
}

func (r *outgoingRequestImpl) SetScheme(_ context.Context, scheme OptionScheme) (Result__, error) {
	if !scheme.IsSome {
		r.r.URL.Scheme = ""
		return types.Result__Ok{}, nil
	}

	switch s := scheme.Value.(type) {
	case SchemeHTTP:
		r.r.URL.Scheme = "http"
	case SchemeHTTPS:
		r.r.URL.Scheme = "https"
	case SchemeOther:
		r.r.URL.Scheme = s.Value
	default:
		return types.Result__Err{}, nil
	}

	return types.Result__Ok{}, nil
}

// requestOptionsImpl is the RequestOptions resource implementation returning ErrNotImplemented.
type requestOptionsImpl struct {
	betweenBytesTimeout OptionU64
	connectTimeout      OptionU64
	firstByteTimeout    OptionU64
}

func (o *requestOptionsImpl) BetweenBytesTimeout(_ context.Context) (OptionU64, error) {
	return o.betweenBytesTimeout, nil
}
func (o *requestOptionsImpl) ConnectTimeout(_ context.Context) (OptionU64, error) {
	return o.connectTimeout, nil
}
func (o *requestOptionsImpl) FirstByteTimeout(_ context.Context) (OptionU64, error) {
	return o.firstByteTimeout, nil
}
func (o *requestOptionsImpl) SetBetweenBytesTimeout(_ context.Context, timeout OptionU64) (Result__, error) {
	o.betweenBytesTimeout = timeout
	return types.Result__Ok{}, nil
}
func (o *requestOptionsImpl) SetConnectTimeout(_ context.Context, timeout OptionU64) (Result__, error) {
	o.connectTimeout = timeout
	return types.Result__Ok{}, nil
}
func (o *requestOptionsImpl) SetFirstByteTimeout(_ context.Context, timeout OptionU64) (Result__, error) {
	o.firstByteTimeout = timeout
	return types.Result__Ok{}, nil
}
