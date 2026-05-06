package types

import "net/http"

// HTTPDoer is the minimal interface wasi:http/outgoing-handler needs to
// issue an outgoing request. *http.Client satisfies it, so the standard
// library client is a valid implementation.
//
// Embedders supply an HTTPDoer through wasi.Config.HttpClient to inspect,
// modify, or deny outgoing requests. To surface a chosen wasi:http/types
// ErrorCode to the guest on deny, return a *CodedError; any other error
// is mapped via the existing translation (DNS, TLS, syscall, default
// ErrorCodeInternalError).
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}
