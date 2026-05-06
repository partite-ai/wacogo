// Package wasi houses the wacogo bindings for wasi p2.
//
// HTTP egress is governed by Config.HttpClient, which is an HTTPDoer.
// To deny or modify outgoing requests, supply a custom Doer:
//
//	type policyDoer struct{ inner *http.Client }
//
//	func (p *policyDoer) Do(r *http.Request) (*http.Response, error) {
//		if strings.HasSuffix(r.URL.Hostname(), ".internal") {
//			return nil, &types.CodedError{
//				Code: types.ErrorCodeHTTPRequestDenied{},
//				Msg:  "egress to *.internal blocked by policy",
//			}
//		}
//		r.Header.Set("X-Trace-Id", newTraceID())
//		return p.inner.Do(r)
//	}
//
//	cfg := wasi.Config{HttpClient: &policyDoer{inner: http.DefaultClient}}
//
// Plain errors map to ErrorCodeInternalError; *types.CodedError surfaces
// the embedded ErrorCode to the guest unchanged.
package wasi

//go:generate go run github.com/partite-ai/wacogo/cmd/wacogo-witgen generate -w wasi:wacogo/imports -o ../internal/wasi/gen -p github.com/partite-ai/wacogo/internal/wasi/gen ../internal/wasi/wit/
