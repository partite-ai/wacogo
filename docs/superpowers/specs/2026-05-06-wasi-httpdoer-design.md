# WASI outgoing-HTTP Doer interface

Date: 2026-05-06
Status: Approved

## Problem

`wasi.Config.HttpClient` is currently typed `*http.Client`, so embedders
who want to deny or modify outgoing HTTP requests from a guest component
must construct a custom `http.RoundTripper` and wire it through a
`*http.Client`. That is more ceremony than the use case warrants, and it
hides the fact that the only behavior `wasi:http/outgoing-handler` cares
about is `Do(*http.Request) (*http.Response, error)`.

We want a one-line way to plug in a function that can:

- inspect / modify an outgoing `*http.Request` before it leaves the host, or
- deny the request and surface a chosen `wasi:http/types` `ErrorCode`
  to the guest.

## Design

### 1. `HTTPDoer` interface

A new single-method interface, declared in `wasi/http/types` and
re-exported from `wasi`:

```go
// wasi/http/types/types.go (or a new file in the same package)
type HTTPDoer interface {
    Do(*http.Request) (*http.Response, error)
}
```

```go
// wasi/config.go
type HTTPDoer = types.HTTPDoer

type Config struct {
    // ... existing fields ...

    // HttpClient executes outgoing HTTP requests for
    // wasi:http/outgoing-handler. When nil, http.DefaultClient is used.
    //
    // Implementations may modify *http.Request before issuing it (URL,
    // headers, body) or deny it by returning an error. To surface a
    // specific wasi:http/types ErrorCode to the guest, return a
    // *types.CodedError (from wasi/http/types); any other error maps to
    // ErrorCodeInternalError via the existing translation.
    HttpClient HTTPDoer
}
```

`*http.Client` already satisfies this interface, so existing call sites
(`cfg.HttpClient = http.DefaultClient`, `cfg.HttpClient = &http.Client{...}`)
continue to compile and behave identically.

### 2. `CodedError` for chosen error codes

A new exported error type in `wasi/http/types`:

```go
// wasi/http/types/coded_error.go
package types

import "fmt"

// CodedError carries a wasi:http/types ErrorCode as a Go error.
// Returning a *CodedError from an HTTPDoer surfaces Code to the guest
// instead of the default ErrorCodeInternalError mapping.
type CodedError struct {
    Code ErrorCode
    Msg  string // optional, used by Error() if non-empty
}

func (e *CodedError) Error() string {
    if e.Msg != "" {
        return e.Msg
    }
    return fmt.Sprintf("wasi:http error %T", e.Code)
}
```

In `wasi/http/outgoinghandler/future_response.go`, prepend one case to
`translateResultErr`:

```go
var coded *types.CodedError
if errors.As(err, &coded) && coded.Code != nil {
    return coded.Code
}
```

If `Code` is nil, the new case falls through to the existing chain so a
malformed `CodedError` does not silently lose information. All other
paths (DNS, TLS, syscall, timeout, generic fallback to
`ErrorCodeInternalError`) are unchanged.

### 3. Wiring

`wasi/http/outgoinghandler/outgoinghandler.go`:

- `impl.httpClient` field is retyped from `*http.Client` to
  `types.HTTPDoer`.
- `NewInstance` parameter `httpClient *http.Client` becomes
  `httpClient types.HTTPDoer`.

`wasi/world.go`:

- `NewWorld` keeps its nil-default: when `cfg.HttpClient == nil`, pass
  `http.DefaultClient` (which satisfies `HTTPDoer`).
- Otherwise pass `cfg.HttpClient` straight through.

No changes to the outgoing-request resource impl, the future plumbing,
or the rest of the world graph. Guests see the same observable
behavior unless the embedder provides a Doer that diverges.

## User-facing example

```go
type denyAllExamples struct{ inner *http.Client }

func (d *denyAllExamples) Do(r *http.Request) (*http.Response, error) {
    if strings.HasSuffix(r.URL.Hostname(), ".example.com") {
        return nil, &types.CodedError{
            Code: types.ErrorCodeHTTPRequestDenied{},
            Msg:  "egress to *.example.com blocked by policy",
        }
    }
    r.Header.Set("X-Trace-Id", newTraceID())
    return d.inner.Do(r)
}

cfg := &wasi.Config{HttpClient: &denyAllExamples{inner: http.DefaultClient}}
```

## Test plan

In `wasi/http/outgoinghandler/` (new test file) or `wasi/world_test.go`:

1. **Modify request** — Doer adds `X-Trace` header and rewrites Host;
   `httptest.Server` records the received request; assert mutations
   reached the wire.
2. **Deny with default mapping** — Doer returns `errors.New("blocked")`;
   guest receives `ErrorCodeInternalError{Value: SomeString("blocked")}`
   (pins the existing fallback).
3. **Deny with `ErrorCodeHTTPRequestDenied`** — Doer returns
   `&types.CodedError{Code: types.ErrorCodeHTTPRequestDenied{}}`; guest
   receives that exact case.
4. **Deny with another code** — same shape but
   `ErrorCodeDestinationIPProhibited{}`; guest receives that exact case.
   Confirms the unwrap path is generic, not hardcoded to "Denied".
5. **Nil default** — `cfg.HttpClient == nil` continues to use
   `http.DefaultClient`.
6. **`*http.Client` compatibility** — `cfg.HttpClient = http.DefaultClient`
   compiles and round-trips a request unchanged.

Tests run a small `httptest.Server` for the success and modify cases;
deny cases need no server (the Doer short-circuits).

## Out of scope

- Per-component-instance hooks (would require plumbing the calling
  `*host.ComponentInstance` through `Handle` and the Doer signature).
- Response modification (a different code path on the future side; not
  asked for).
- Equivalent hooks for other WASI subsystems (filesystem, sockets).
  The same pattern would work but is a separate spec.

## Backward compatibility

`Config.HttpClient`'s declared type widens from `*http.Client` to the
`HTTPDoer` interface. Code that assigns a `*http.Client` value
continues to compile. Code that read `*http.Client`-specific fields off
`cfg.HttpClient` (e.g., `cfg.HttpClient.Transport`) would break, but
the project is at commit `f9bb869` and has no such call sites.

The `NewInstance` signature in `wasi/http/outgoinghandler` similarly
widens its `httpClient` parameter; the same compatibility note applies.
