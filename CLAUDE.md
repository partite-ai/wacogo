# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Wacogo is a WebAssembly Component Model (MVP) runtime in Go. It uses wazero as the core wasm execution engine and implements component model linking, loading, and instantiation on top.

**Module path**: `github.com/partite-ai/wacogo`

## Build & Test Commands

```bash
go build ./...                          # build all packages
go test ./wasmparser/ -count=1          # run all wasmparser tests
go test ./wasmparser/ -run TestName     # run a single test
go test ./wasmparser/ -run TestComponentModelWast/empty -v  # run a specific wast suite
go generate ./wasmparser/...            # regenerate test fixtures (requires wasm-tools >= 1.245.0)
```

Test infrastructure uses `wasm-tools json-from-wast` to pre-compile `.wast` test files into JSON + binary fixtures. Source `.wast` files live in `wasmparser/testdata/wast/` (committed). Generated fixtures live in `wasmparser/testdata/generated/` (gitignored — run `go generate` to create them).

## Architecture

### wasmparser Package

Streaming parser and validator for WebAssembly Component Model binaries. Two separate layers:

- **Parser** (`parser.go`, `parse_all.go`, `validating_parser.go`) — reads from `io.Reader`, yields `Payload` values via `Next()`. `ParseAll` manages nesting. `ValidatingParser` wraps a `Parser` + `Validator` for parse-and-validate in a single `Next()` call, supporting recursive sub-component consumption via `SubParser()`.
- **Validator** (`validator.go`, `validator_state.go`, `validator_types.go`, `validator_names.go`) — consumes payloads, checks semantic correctness. Tracks `ComponentState` per nesting level with index spaces for all definition kinds.

**Type system conventions:**

- Unions → interface with private marker method + concrete types (use type switch)
- Optional fields → `Optional[T]` generic (never pointers)
- All struct fields exported
- Types decode themselves via `BinaryUnmarshaler` interface

**Key design: Load/Instantiate Split** (future phases)

Mirrors wazero's compile/instantiate pattern:

- **Load** — expensive, parses and validates a component binary
- **Instantiate** — cheap, wires up imports and creates a running instance

### canon/ Package — Canonical ABI Machinery

The `canon/` package is the sole implementation of the canonical ABI lowering, lifting, and cross-component transfer pipeline. It uses a visitor-driven closure-emission architecture:

1. **Visitors** (`canon/visitor_*.go`) — six concrete `TypeVisitor` implementations walk the `Type` tree emitting pre-bound closures. Each side of a call is covered: `flatTransferVisitor`/`memTransferVisitor` for component→component transfer, `valToFlatVisitor`/`valToMemVisitor` for Go→wasm lowering, and `flatToValVisitor`/`memToValVisitor` for wasm→Go lifting. Each `Accept(visitor)` call emits exactly one step per type; composites (record, tuple, list, variant, option, result) compile children via child visitors and wrap them in a single outer step.
2. **Plans** (`canon/plan.go`) — `compileTransferPlan` (internal) and `compileGocallPlan` (internal) run the appropriate visitors and return a `*transferPlan` or `*gocallPlan` holding the emitted step closures. `pickFlat` selects flat vs mem mode per the canonical-ABI `maxFlatParams` / `maxFlatResults` caps. Three step closure types: `transferPlanStep` for component→component (takes `srcBase`, `dstBase`), `gocallLowerStep` for Go→wasm (takes input `Val` and `base`), `gocallLiftStep` for wasm→Go (takes `base`, returns `Val`).
3. **Contexts** (`canon/context.go`) — per-call state. `transferContext` holds caller and callee `transferSide` descriptors (memory, realloc, encoding, resource table) plus the shared wazero register stack; `gocallContext` holds only the callee `transferSide`, the core-register slice, and lent-borrows teardown list — input args and lifted results flow through `runGocallPlan`'s parameters and return value, not through the context. Enter/Exit hooks on `transferSide` gate reentrance.
4. **Runners** (`canon/run_transfer.go`, `canon/run_gocall.go`) — `runTransferPlan` (internal) and `runGocallPlan` (internal) execute the compiled step closures against a context, invoke the callee core function, and drive post-return. `runGocallPlan` pairs each `args[i]` with `paramSteps[i]` and collects each `resultSteps[i]` return into the output `[]Val`.

### Package Layout

- Root `wacogo/` is a thin public API layer: `Engine` struct (embedding `*core.Engine`), type aliases to `internal/core`, `InstantiateOption` trampolines, and the `(*Engine).NewHostBuilder` entry. No implementation lives here.
- `internal/core/` holds the Engine, types, canon-bridge, component/instance/func implementation, and wasm-level plumbing (`NewInstance`, `WazeroRuntime`, `Validator` — package-level functions so they don't promote onto the public wrapper). It carries no host-side state.
- `host/` imports `internal/core` (not root). Everything host users see — `host.Builder`, `host.Component`, `host.ComponentInstance` — is built on `core.*` types. `*host.ComponentInstance` is the canonical wrapper that owns the extern table, per-instance `TypeResource` slot space, resource-source map, user state, and lifecycle hooks.

### internal/core/canon_bridge.go — Type-System Adapter

Glues core's `Type` universe to canon's interfaces:

- `canonType{t: Type}.Accept(TypeVisitor)` dispatches every `core.Type*` into the right `Visit*` call. Implements the canon `Type` interface.
- `resourceTypeID(*TypeResource)` wraps `*TypeResource` to satisfy canon's `ResourceTypeID` via pointer identity.
- `FuncTypeParamsAsCanon(ft *FuncType) []canon.Type` / `FuncTypeResultsAsCanon(ft *FuncType) []canon.Type` — slice walkers used by the host package to build canon plans.

The concrete `Val*` types live in `internal/canon/val.go` and are re-exported from `internal/core` and then from root `wacogo` via Go type aliases. No `Val`-level conversion exists — they share identical types across all three layers.

### Resource Handles

Each live `*core.ComponentInstance` holds an opaque `*canon.ComponentInstance`, which owns a package-private `ResourceTable` — 1-indexed, kind-discriminated (own vs borrow). `DropOwn` rejects borrow handles and vice versa. The table stores a `ResourceTypeID` per entry and enforces pointer-identity matching on every lookup. Handle-transfer steps inside canon call `IssueOwn`/`DropOwn`/`Rep`/`Lend`/`Unlend` on the caller and callee tables; a nil table (Go-Val side) signals passthrough semantics. Core code never sees the table — resource-table access flows through the canon factories.

For host-defined resources, `*host.ComponentInstance` owns an additional extern table (an opaque `externTable`) that maps `ExternHandle` integers to Go objects. Host code registers and releases objects through `RegisterResource`/`ReleaseResource` and looks them up with `LookupResource(ExternHandle)`. The extern table is fully encapsulated inside `*host.ComponentInstance`; there are no package-level helpers for it.

Witgen-emitted `*<R>Handle` types use a small state machine (`handleState` interface) to manage their lifecycle across component boundaries. Three states exist: `unregisteredHandleState` (impl held in Go memory, not yet in any canon or extern table), `boundHandleState` (registered in a specific instance's canon table, with a captured `dropFn`), and `invalidatedHandleState` (transferred out, further bind attempts fail). Two constructors serve different call sites: `NewXXXHandle(impl)` for same-package construction (the definer is resolved to the call's owner at first bind), and `NewXXXHandleIn(definer, impl)` for cross-package construction (the definer is pre-decided so the handle can be registered on the correct instance even when the caller's `cc` owns a different component's canon table). Both are package-level free functions. Bind-site trampolines call `h.bind(cc, owner)` to lazily issue the canon own entry on first use; on own-transfer out of Go the trampoline calls `h.Invalidate()` so the handle cannot be double-transferred. `Drop()` returns error and is idempotent (no-op on unregistered or invalidated handles).

### Cross-Component Adapters

`canon.Host.BuildAdapter` (`internal/canon/build_adapter.go`) constructs a wasm stub module whose export bridges a cross-component call. Internally it compiles a `transferPlan`, builds an `adapterFunc` Go host function, and wraps it in a tiny wasm stub (import + re-export) because wazero's `ImportResolver` only works with real wasm module instances. The core's `execLowerWithGoAdapter` is a thin wrapper that resolves caller/callee `CallSide`+`Callee` descriptors from instantiation state and calls the factory; the returned `*canon.CallAdapter` is tracked in `ComponentInstance.auxiliaryAdapters` for teardown on `Close`.

### Import Resolution

Use wazero's `experimental.WithImportResolver` for forwarding imports between core instances. Blank import names still require the empty-name rewriting workaround since ImportResolver doesn't handle those.

## Coding Guidelines

- Minimal public APIs per package. Each public method/struct adds to the mental overhead of understanding the package.
- Focused public docstrings. Doc strings on public items describe only what an external user of the method would need to know. NO IMPLEMENTATION DETAILS. Implementation details on a func can go in comments in the func body.
- Minimize mutations and consider concurrency. If we're lazily initializing something, or mutating state, especially in another struct, as a side effect of what we're doing it's probably wrong. Obviously, intentional state mutations are OK (i.e. if we're in an IncrementCounter method, of course we're going to mutate state), but side effect mutations are bad (i.e. we're in an InvokeMethod call, and we go and rebuild metadata as part of that)
- Follow all Go standard naming conventions.
