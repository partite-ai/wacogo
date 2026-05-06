# Contributing to wacogo

## Build and test

```sh
go build ./...
go test ./...
```

To run a single test or a specific subtest:

```sh
go test ./wasmparser/ -run TestName -count=1
go test ./wasmparser/ -run TestComponentModelWast/empty -v
```

Integration tests under `host/` and the runtime tests under
`internal/witgen/fixturetests/**/*_test.go` exercise the full wasm
boundary. They use the [`wasmtools/`](wasmtools/) package, which runs
the wasip1 build of `wasm-tools` inside wazero — no external
`wasm-tools` binary is required.

### Regenerating wasmparser fixtures

`wasmparser/` test fixtures are pre-compiled from `.wast` source via
`wasm-tools json-from-wast` (driven by the embedded wasmtools package
through wazero). Source `.wast` files in `wasmparser/testdata/wast/`
are committed; generated JSON + binary fixtures in
`wasmparser/testdata/generated/` are gitignored.

```sh
go generate ./wasmparser/...
```

### Bumping the embedded wasm-tools version

The wasip1 wasm-tools binary is committed to
`wasmtools/wasm-tools.wasm.gz`. To upgrade, edit the version constant
in `wasmtools/fetch.go` and regenerate:

```sh
go generate ./wasmtools/...
```

## Package layout

```
wacogo/                    # Public API: Engine + type aliases + option trampolines
├── engine.go              #   NewEngine, LoadComponent, NewHostBuilder
├── types.go, val.go, options.go
│
├── host/                  # Host-component declaration layer
│
├── wasi/                  # Pre-built wasi:cli + wasi:http host components
│
├── wasmtools/             # Embedded wasip1 wasm-tools, runnable via wazero
│
├── cmd/wacogo-witgen/     # WIT-to-Go bindings generator (CLI)
│
├── internal/
│   ├── core/              # Engine + instance internals + canon bridge
│   ├── canon/             # Canonical-ABI lowering / lifting / transfer
│   ├── wasm/              # wasm value-type constants
│   └── witgen/            # WIT bindings generator (used by cmd/wacogo-witgen)
│
├── wasmparser/            # Streaming component-model binary parser + validator
│
├── docs/                  # Design discussion
│
└── examples/              # Runnable demos
```

The root `wacogo` package is a thin public API layer. Public types are
Go aliases of `internal/core` types; public functions are trampolines.
All implementation lives in `internal/core` and `host/`. The `host`
package imports `internal/core` directly (not through the root package),
so no host-side state leaks onto the root `Engine` wrapper.

## Architecture

Wacogo follows wazero's compile / instantiate split:

- **Load** is expensive: parses, validates, and compiles a binary into
  a reusable `*Component`.
- **Instantiate** is cheap: wires up imports and yields a live
  `*ComponentInstance` with its own resource state.

The canonical ABI lives in `internal/canon` and uses a visitor-driven
closure-emission architecture. Six concrete `TypeVisitor`
implementations cover every direction (caller→callee transfer, Go→wasm
lowering, wasm→Go lifting) across both storage modes (flat registers
vs. linear memory). Plans compile a tree of pre-bound step closures;
runners execute them against a per-call context.

`internal/core/canon_bridge.go` adapts core's `Type` universe to canon's
interfaces. `wasmparser/` is independent of the runtime — it streams
component binaries and validates them, with its own internal index-space
machinery.

For deeper reading:

- [`docs/code_overview.md`](docs/code_overview.md) — navigation map
  across the source tree
- [`docs/canonical_abi.md`](docs/canonical_abi.md) — canon machinery
- [`docs/host_layer.md`](docs/host_layer.md) — `host` package internals
- [`docs/instantiation.md`](docs/instantiation.md) — load/instantiate
  pipeline
- [`docs/type_system.md`](docs/type_system.md) — `Type` and `Val`
  conventions
- [`docs/witgen.md`](docs/witgen.md) — WIT bindings generator design

For the `*<R>Handle` state machine see the "Resource Handles"
section of `CLAUDE.md`. For the canon-side handle lifecycle
(ResourceTable, live/detached handles, `Task`-centric borrow
tracking, gocall `*ValOwnHandle` semantics), see the "Resource
handles across the canon/core boundary" section of
`docs/canonical_abi.md`.

## Coding guidelines

- **Minimal public APIs.** Each public method or struct adds to the
  mental overhead of using the package. Prefer composing existing
  primitives over adding new ones.
- **Focused public docstrings.** Doc strings on public items describe
  only what an external user would need to know. No implementation
  details — those go in body comments.
- **Type-system conventions:**
  - Unions → interface with a private marker method + concrete types
    (use a type switch).
  - Optional fields → `Optional[T]` generic — never pointers.
  - All struct fields exported.
  - Types decode themselves via `BinaryUnmarshaler`.
- **Minimize side-effect mutations.** Lazy initialization or incidental
  state changes inside otherwise non-mutating methods are usually wrong.
  Intentional mutation in obviously-mutating methods (e.g. an
  `IncrementCounter` method) is fine.
- **Concurrency.** `Engine` Load and Instantiate are safe for concurrent
  use; the component-model spec forbids concurrent or reentrant calls
  on a single `*ComponentInstance`.
- **Import wiring.** Use wazero's `experimental.WithImportResolver` for
  forwarding imports between core instances. Do not use host modules
  for import wiring.
- **Standard Go naming** throughout.

