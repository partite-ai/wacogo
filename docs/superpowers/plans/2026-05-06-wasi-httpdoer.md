# WASI outgoing-HTTP Doer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `wasi.Config.HttpClient` (currently `*http.Client`) with an `HTTPDoer` interface so embedders can deny or modify outgoing HTTP requests, and add a `*types.CodedError` wrapper so a Doer can surface a chosen WASI `ErrorCode` to the guest.

**Architecture:** Single-method interface `Do(*http.Request) (*http.Response, error)` declared in `wasi/http/types`, aliased from `wasi`. `*http.Client` already satisfies it. A new `*types.CodedError{Code, Msg}` carries any `wasi:http/types.ErrorCode`; `translateResultErr` (in `wasi/http/outgoinghandler/future_response.go`) gains one `errors.As` branch that unwraps the embedded code so it reaches the guest unchanged. All other existing translation cases (DNS, TLS, syscall, default `ErrorCodeInternalError`) are untouched.

**Tech Stack:** Go, wazero (transitively, via `wacogo`), Go `net/http`, `errors` for unwrap.

**Spec:** [`docs/superpowers/specs/2026-05-06-wasi-httpdoer-design.md`](../specs/2026-05-06-wasi-httpdoer-design.md)

---

## File Structure

**New files:**
- `wasi/http/types/coded_error.go` — `CodedError` struct + `Error()` method.
- `wasi/http/types/coded_error_test.go` — unit tests for `CodedError.Error()`.
- `wasi/http/types/doer.go` — `HTTPDoer` interface (placed here so `wasi/http/outgoinghandler` can reference it without depending on `wasi`).
- `wasi/http/outgoinghandler/translate_result_err_test.go` — internal-package tests for `translateResultErr`'s new `*CodedError` branch.

**Modified files:**
- `wasi/http/outgoinghandler/future_response.go` — prepend `*CodedError` unwrap to `translateResultErr`.
- `wasi/http/outgoinghandler/outgoinghandler.go` — retype `impl.httpClient` and `NewInstance`'s `httpClient` parameter from `*http.Client` to `types.HTTPDoer`.
- `wasi/config.go` — add `HTTPDoer = types.HTTPDoer` alias; retype `Config.HttpClient`.
- `wasi/world.go` — no behavioral change; `*http.Client` still satisfies `HTTPDoer`, so the nil-default path keeps using `http.DefaultClient`. (Confirm by re-reading.)
- `wasi/world_test.go` — add a smoke test for a custom Doer + a compile-time `var _ wasi.HTTPDoer = (*http.Client)(nil)`.
- `wasi/doc.go` — small package-level example showing a deny-by-host Doer.

## Test scope note

The spec's test plan lists six tests. Five are covered here:

- **#2, #3, #4** (deny semantics) — covered by `translateResultErr` unit tests in Task 2.
- **#5** (nil default) — already covered by existing `TestNewWorld` (passes `wasi.Config{}` with nil `HttpClient`).
- **#6** (`*http.Client` still works) — covered by a compile-time assertion + the smoke test in Task 4.

**#1 (request-modification end-to-end) is deferred.** Reaching `impl.Handle` from a test requires either (a) writing a guest wasm component that issues an outgoing request, or (b) constructing the unexported `impl` from outside the `outgoinghandler` package — but that package is imported by `wasi`, so an internal test in `outgoinghandler` cannot import `wasi.NewWorld` to obtain the `*host.ComponentInstance` deps it needs. The mechanical wiring is one line (`i.httpClient.Do(httpRequest)`), so this gap is low-risk; we will revisit when a guest-component test harness lands.

---

## Task 1: Add `CodedError` type

**Files:**
- Create: `wasi/http/types/coded_error.go`
- Create: `wasi/http/types/coded_error_test.go`

- [ ] **Step 1: Write the failing test**

Create `wasi/http/types/coded_error_test.go`:

```go
package types_test

import (
	"testing"

	"github.com/partite-ai/wacogo/wasi/http/types"
)

func TestCodedError_Error_UsesMsgWhenSet(t *testing.T) {
	err := &types.CodedError{
		Code: types.ErrorCodeHTTPRequestDenied{},
		Msg:  "blocked by policy",
	}
	if got, want := err.Error(), "blocked by policy"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestCodedError_Error_DefaultIncludesCodeType(t *testing.T) {
	err := &types.CodedError{Code: types.ErrorCodeHTTPRequestDenied{}}
	got := err.Error()
	want := "wasi:http error types.ErrorCodeHTTPRequestDenied"
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestCodedError_Error_NilCode(t *testing.T) {
	err := &types.CodedError{}
	got := err.Error()
	want := "wasi:http error <nil>"
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./wasi/http/types/ -run TestCodedError -v`
Expected: build failure (`undefined: types.CodedError`).

- [ ] **Step 3: Write minimal implementation**

Create `wasi/http/types/coded_error.go`:

```go
package types

import "fmt"

// CodedError carries a wasi:http/types ErrorCode as a Go error.
//
// Returning a *CodedError from an HTTPDoer surfaces Code to the guest
// instead of the default ErrorCodeInternalError mapping. If Msg is
// empty, Error() falls back to a description naming the Code's Go type.
type CodedError struct {
	Code ErrorCode
	Msg  string
}

func (e *CodedError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("wasi:http error %T", e.Code)
}
```

Note on the `%T` of a nil `Code`: Go formats it as `<nil>` (the test pins this).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./wasi/http/types/ -run TestCodedError -v`
Expected: PASS for all three subtests.

- [ ] **Step 5: Run the full package test to make sure nothing regressed**

Run: `go test ./wasi/http/types/ -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add wasi/http/types/coded_error.go wasi/http/types/coded_error_test.go
git commit -m "feat(wasi/http/types): add CodedError for surfacing chosen ErrorCode"
```

---

## Task 2: Wire `*CodedError` into `translateResultErr`

**Files:**
- Modify: `wasi/http/outgoinghandler/future_response.go:123-196` (function `translateResultErr`)
- Create: `wasi/http/outgoinghandler/translate_result_err_test.go`

- [ ] **Step 1: Write the failing tests**

Create `wasi/http/outgoinghandler/translate_result_err_test.go` (internal package test — note `package outgoinghandler`):

```go
package outgoinghandler

import (
	"errors"
	"fmt"
	"testing"

	"github.com/partite-ai/wacogo/wasi/http/types"
)

func TestTranslateResultErr_CodedErrorHTTPRequestDenied(t *testing.T) {
	in := &types.CodedError{Code: types.ErrorCodeHTTPRequestDenied{}}
	got := translateResultErr(in)
	if _, ok := got.(types.ErrorCodeHTTPRequestDenied); !ok {
		t.Fatalf("got %T, want ErrorCodeHTTPRequestDenied", got)
	}
}

func TestTranslateResultErr_CodedErrorDestinationIPProhibited(t *testing.T) {
	in := &types.CodedError{Code: types.ErrorCodeDestinationIPProhibited{}}
	got := translateResultErr(in)
	if _, ok := got.(types.ErrorCodeDestinationIPProhibited); !ok {
		t.Fatalf("got %T, want ErrorCodeDestinationIPProhibited", got)
	}
}

func TestTranslateResultErr_CodedErrorWrapped(t *testing.T) {
	inner := &types.CodedError{Code: types.ErrorCodeHTTPRequestDenied{}}
	wrapped := fmt.Errorf("doer rejected: %w", inner)
	got := translateResultErr(wrapped)
	if _, ok := got.(types.ErrorCodeHTTPRequestDenied); !ok {
		t.Fatalf("got %T, want ErrorCodeHTTPRequestDenied (errors.As must unwrap)", got)
	}
}

func TestTranslateResultErr_CodedErrorNilCodeFallsThrough(t *testing.T) {
	in := &types.CodedError{Code: nil, Msg: "unspecific deny"}
	got := translateResultErr(in)
	internal, ok := got.(types.ErrorCodeInternalError)
	if !ok {
		t.Fatalf("got %T, want ErrorCodeInternalError fallback", got)
	}
	if !internal.Value.IsSome || internal.Value.Value != "unspecific deny" {
		t.Fatalf("ErrorCodeInternalError.Value = %+v, want SomeString(\"unspecific deny\")", internal.Value)
	}
}

func TestTranslateResultErr_PlainErrorStillFallsBack(t *testing.T) {
	got := translateResultErr(errors.New("blocked"))
	internal, ok := got.(types.ErrorCodeInternalError)
	if !ok {
		t.Fatalf("got %T, want ErrorCodeInternalError", got)
	}
	if !internal.Value.IsSome || internal.Value.Value != "blocked" {
		t.Fatalf("ErrorCodeInternalError.Value = %+v, want SomeString(\"blocked\")", internal.Value)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./wasi/http/outgoinghandler/ -run TestTranslateResultErr -v -count=1`
Expected: the four `CodedError*` tests fail (the function ignores `*CodedError`, so they get `ErrorCodeInternalError` instead). `TestTranslateResultErr_PlainErrorStillFallsBack` should already pass.

- [ ] **Step 3: Update `translateResultErr`**

Edit `wasi/http/outgoinghandler/future_response.go`. Find this block at the top of `translateResultErr`:

```go
func translateResultErr(err error) types.ErrorCode {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) {
```

Insert the `*CodedError` unwrap immediately after the `nil` check:

```go
func translateResultErr(err error) types.ErrorCode {
	if err == nil {
		return nil
	}

	var coded *types.CodedError
	if errors.As(err, &coded) && coded.Code != nil {
		return coded.Code
	}

	if errors.Is(err, context.DeadlineExceeded) {
```

The `coded.Code != nil` guard ensures a malformed `CodedError` (no Code set) keeps falling through to the existing chain so its `Msg` reaches the guest as `ErrorCodeInternalError{Value: SomeString(Msg)}`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./wasi/http/outgoinghandler/ -run TestTranslateResultErr -v -count=1`
Expected: all five tests PASS.

- [ ] **Step 5: Run the full outgoinghandler test set**

Run: `go test ./wasi/http/outgoinghandler/ -count=1`
Expected: PASS (no other tests in this package today, so this is a sanity check).

- [ ] **Step 6: Commit**

```bash
git add wasi/http/outgoinghandler/future_response.go wasi/http/outgoinghandler/translate_result_err_test.go
git commit -m "feat(wasi/http/outgoinghandler): unwrap *CodedError in translateResultErr"
```

---

## Task 3: Add `HTTPDoer` interface and retype `Config.HttpClient`

**Files:**
- Create: `wasi/http/types/doer.go`
- Modify: `wasi/http/outgoinghandler/outgoinghandler.go:30-103` (struct field, `NewInstance` signature)
- Modify: `wasi/config.go:11-29`
- Modify: `wasi/world.go:87-90` (verify-only — should still compile unchanged)

This task is one atomic change: the interface lives in `wasi/http/types`, the field type widens in `Config`, and the `NewInstance` parameter type widens in `outgoinghandler`. They have to land together because each consumer references the new type.

- [ ] **Step 1: Add the `HTTPDoer` interface to `wasi/http/types`**

Create `wasi/http/types/doer.go`:

```go
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
```

- [ ] **Step 2: Retype the `outgoinghandler` impl and constructor**

Edit `wasi/http/outgoinghandler/outgoinghandler.go`. Two changes:

1. The `impl` struct field at line ~31:

   Before:
   ```go
   type impl struct {
   	httpClient  *http.Client
   	typesInst   *host.ComponentInstance
   ```

   After:
   ```go
   type impl struct {
   	httpClient  types.HTTPDoer
   	typesInst   *host.ComponentInstance
   ```

2. The `NewInstance` parameter at line ~85:

   Before:
   ```go
   func NewInstance(
   	ctx context.Context,
   	engine *wacogo.Engine,
   	typesInst *host.ComponentInstance,
   	errorInst *host.ComponentInstance,
   	pollInst *host.ComponentInstance,
   	streamsInst *host.ComponentInstance,
   	httpClient *http.Client,
   ) (*host.ComponentInstance, error) {
   ```

   After:
   ```go
   func NewInstance(
   	ctx context.Context,
   	engine *wacogo.Engine,
   	typesInst *host.ComponentInstance,
   	errorInst *host.ComponentInstance,
   	pollInst *host.ComponentInstance,
   	streamsInst *host.ComponentInstance,
   	httpClient types.HTTPDoer,
   ) (*host.ComponentInstance, error) {
   ```

3. Remove the now-unused `"net/http"` import if no other code in the file references `http`. Check: `Handle` returns a future — does it reference `http`? Look at the imports block; if `net/http` is now only used through `types.HTTPDoer`, drop it. (The file's existing `httpRequest := request.ToHTTPRequest()` returns `*http.Request`, so `net/http` is still needed implicitly through `request`'s return type; keep the import if `go vet` complains either way. Run `go build` after the edit and let the compiler tell you.)

- [ ] **Step 3: Add the alias and retype `Config.HttpClient`**

Edit `wasi/config.go`. Replace the file's contents with:

```go
package wasi

import (
	"io"

	"github.com/partite-ai/wacogo/wasi/filesystem/preopens"
	"github.com/partite-ai/wacogo/wasi/http/types"
)

// HTTPDoer is the minimal interface wasi:http/outgoing-handler uses to
// issue outgoing requests. It is an alias for types.HTTPDoer; *http.Client
// satisfies it.
type HTTPDoer = types.HTTPDoer

// Config configures NewWorld.
type Config struct {
	Args       []string
	Env        [][2]string
	InitialCwd string

	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader

	// Preopens, if non-nil, is invoked after the host instances it
	// depends on are constructed. It returns the Preopens
	// implementation to bind to the wasi:filesystem/preopens host
	// component. When nil, an empty preopen set is exposed.
	Preopens func(preopens.Deps) preopens.Preopens

	// HttpClient executes outgoing HTTP requests for
	// wasi:http/outgoing-handler. When nil, http.DefaultClient is used.
	//
	// Implementations may modify *http.Request before issuing it (URL,
	// headers, body) or deny it by returning an error. To surface a
	// specific wasi:http/types ErrorCode to the guest, return a
	// *types.CodedError (from wasi/http/types); any other error is
	// mapped via the existing translation, falling back to
	// ErrorCodeInternalError.
	HttpClient HTTPDoer
}
```

(The `"net/http"` import is no longer needed in this file because `http.Client` is replaced with the interface. The `types` import takes its place.)

- [ ] **Step 4: Verify `wasi/world.go` still compiles unchanged**

Read `wasi/world.go:87-90`. The existing block:

```go
httpClient := cfg.HttpClient
if httpClient == nil {
	httpClient = http.DefaultClient
}
```

`http.DefaultClient` is `*http.Client`, which satisfies `HTTPDoer`, so this assignment continues to compile. The `httpClient` local now has type `HTTPDoer`, which is what `httpoutgoing.NewInstance` expects after Step 2. No edit needed; this step is a verification.

If `go build ./...` fails on `world.go` because of the type narrowing-on-assignment behavior, change the `var` form: replace the three lines above with

```go
httpClient := wasi.HTTPDoer(cfg.HttpClient)
if httpClient == nil {
	httpClient = http.DefaultClient
}
```

— but try the unchanged form first.

- [ ] **Step 5: Build everything**

Run: `go build ./...`
Expected: clean build. If it fails, fix the offending file (most likely an unused-import error in `outgoinghandler.go` or `config.go`) and re-run.

- [ ] **Step 6: Run the full test suite**

Run: `go test ./... -count=1`
Expected: all tests PASS, including the existing `TestNewWorld` (which exercises the `cfg.HttpClient == nil` default path).

- [ ] **Step 7: Commit**

```bash
git add wasi/http/types/doer.go wasi/http/outgoinghandler/outgoinghandler.go wasi/config.go
# wasi/world.go only if Step 4 required an edit
git commit -m "feat(wasi): replace *http.Client with HTTPDoer interface in Config"
```

---

## Task 4: Smoke test — custom Doer through `NewWorld`

**Files:**
- Modify: `wasi/world_test.go`

This task adds two checks: a compile-time assertion that `*http.Client` satisfies `wasi.HTTPDoer`, and a runtime test that `NewWorld` accepts and stores a custom Doer (proving the field plumbs through; the deeper Doer-invocation path is covered by Task 2's unit tests).

- [ ] **Step 1: Write the failing test**

Edit `wasi/world_test.go`. Append to the existing file:

```go
import (
	"net/http"
	// keep the existing imports (context, testing, wacogo, wasi)
)

// Compile-time assertion: *http.Client satisfies wasi.HTTPDoer.
var _ wasi.HTTPDoer = (*http.Client)(nil)

type recordingDoer struct {
	called bool
}

func (d *recordingDoer) Do(r *http.Request) (*http.Response, error) {
	d.called = true
	return nil, errors.New("recordingDoer never issues real requests")
}

func TestNewWorld_AcceptsCustomDoer(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	t.Cleanup(func() {
		if err := e.Close(ctx); err != nil {
			t.Errorf("engine.Close: %v", err)
		}
	})

	doer := &recordingDoer{}
	cfg := wasi.Config{HttpClient: doer}
	w, err := wasi.NewWorld(ctx, e, &cfg)
	if err != nil {
		t.Fatalf("NewWorld with custom Doer: %v", err)
	}
	if err := w.Close(ctx); err != nil {
		t.Fatalf("World.Close: %v", err)
	}
	// recordingDoer.called is intentionally not asserted here: NewWorld
	// builds the world without issuing requests. The compile-time
	// assertion above and the successful construction together verify
	// that HTTPDoer-typed values flow through cfg.HttpClient.
}
```

(Add `"errors"` to the import block alongside the existing imports.)

- [ ] **Step 2: Run the test to verify it builds and passes**

Run: `go test ./wasi/ -run TestNewWorld -v -count=1`
Expected: both `TestNewWorld` (existing) and `TestNewWorld_AcceptsCustomDoer` PASS.

If it doesn't compile, fix the import block (likely missing `errors` or `net/http`).

- [ ] **Step 3: Commit**

```bash
git add wasi/world_test.go
git commit -m "test(wasi): smoke-test custom HTTPDoer through NewWorld"
```

---

## Task 5: Doc example

**Files:**
- Modify: `wasi/doc.go`

- [ ] **Step 1: Replace the file**

Edit `wasi/doc.go`. Replace its contents with:

```go
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
```

- [ ] **Step 2: Verify the package builds (the example is in a comment, so it's not compiled, but make sure the file is syntactically valid)**

Run: `go build ./wasi/`
Expected: clean.

- [ ] **Step 3: Verify the doc renders**

Run: `go doc ./wasi/`
Expected: output includes the Doer example block.

- [ ] **Step 4: Commit**

```bash
git add wasi/doc.go
git commit -m "docs(wasi): show HTTPDoer policy example in package comment"
```

---

## Final verification

- [ ] **Step 1: Full build + test**

Run: `go build ./... && go test ./... -count=1`
Expected: clean build, all tests PASS.

- [ ] **Step 2: Confirm no stray `*http.Client` types in public surface**

Run: `grep -rn "HttpClient\b\|httpClient\b" wasi/ --include="*.go"`
Expected: every match either declares the field/parameter as `HTTPDoer` / `types.HTTPDoer`, or is a local variable of that interface type. No remaining `*http.Client` typings except where comments mention default behavior.

- [ ] **Step 3: Confirm spec ↔ plan coverage**

Re-read `docs/superpowers/specs/2026-05-06-wasi-httpdoer-design.md`. Walk each section:

- Section 1 (HTTPDoer interface) → Tasks 1, 3.
- Section 2 (CodedError + translateResultErr) → Tasks 1, 2.
- Section 3 (wiring) → Task 3.
- Test plan items #2, #3, #4 → Task 2.
- Test plan items #5, #6 → Task 4 (existing `TestNewWorld` + compile-time assertion).
- Test plan item #1 (modify request, end-to-end) → deferred (see "Test scope note" above).
- User-facing example → Task 5.
- Backward compatibility → Task 3 (interface widening preserves `*http.Client` assignment).
