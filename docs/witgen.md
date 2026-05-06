# Witgen

`internal/witgen/` is the WIT-to-Go bindings generator. It turns a WIT
file plus a chosen world into a self-contained Go package per imported
interface — one each for the user-implementer surface, the host-side
trampolines / factory, and the consumer-side `WrapInstance` wrapper.
The CLI in `cmd/wacogo-witgen/` is a thin wrapper around the
`witgen.Generate(opts)` library entry point.

The generator targets the public `host` and root `wacogo` packages
exclusively — it never reaches into `internal/canon/` or `internal/core/`.
Generated code is meant to look like code a careful user could have
written by hand against `host.Builder` / `host.ComponentInstance` /
`host.ExportedFunc.CallRaw`, just regenerated mechanically.

## Pipeline

```
WIT file
   │
   ▼  Load (go.bytecodealliance.org/wit)        load.go
   ▼  LowerWorld → IR                          lower.go, ir.go
   ▼  Validate                                 validate.go
   ▼  per Interface:
   │     emitIfaceFile  → <iface>.go           emit_iface.go
   │     emitBindFile   → <iface>.bind.go      emit_factory.go
   │     emitWrapperFile→ <iface>.wrap.go      emit_wrapper.go
   ▼  formatGoSource (AST prune + go/format)   emit.go
   ▼  Write to disk (or return as map)         witgen.go
```

Every emitted file is rendered through `text/template` skeletons
(`internal/witgen/templates/*.tmpl`, `go:embed`-loaded) and then run
through `formatGoSource` (`emit.go`): an AST-level pass over
`golang.org/x/tools/go/ast/astutil` that drops unused imports the
templates may have included speculatively, followed by `go/format`
canonical reformatting. Avoiding `goimports`' full module-resolver
walk keeps generation fast in tests. Statement-level emission is
plain Go in the `emit_*.go` files, not templates — recursion through
the type tree happens where it can be debugged.

## Why three files per interface

The split mirrors the three audiences:

- `<iface>.go` — "clean" file. Type declarations, the implementer
  interface (`<Iface>Impl`), the consumer interface (`<Iface>`),
  resource handle types, and the handle state-machine helpers. This
  is the file users read; it has no marshalling noise.
- `<iface>.bind.go` — host-side. The `Factory`, builder calls, the
  `wrap*` host.Func trampolines, and per-type `toGo*` / `fromGo*`
  marshalers. Users implementing a host component never read this
  file by hand.
- `<iface>.wrap.go` — consumer-side. The `WrapInstance` constructor,
  the `<iface>Wrapper` struct, per-method bodies that drive
  `host.ExportedFunc.CallRaw`, the `*<R>Remote` dispatch types, and
  per-type `liftFlat*` / `lowerFlat*` helpers performing
  cross-component transfer.

A single Go package hosts all three so that an interface's types,
implementer interface, and wrapper share their identity. Cross-package
`use` of resources is supported by importing the defining interface's
package.

## IR

`ir.go` defines a sealed `Type` interface with private `isType()`
markers. Three families:

- **Primitives** — `Prim` (constants `PrimBool` … `PrimChar`).
- **Structural compounds** — `TypeString`, `*TypeList`, `*TypeTuple`,
  `*TypeOption`, `*TypeResult`. Anonymous; deduped per package by
  recursive `TypeName(t)` (`name.go`).
- **Nominals** — `*TypeRecord`, `*TypeVariant`, `*TypeEnum`,
  `*TypeFlags`, `*TypeResource`. Live on `Interface.Records` etc.
- **Resource handles** — `*TypeOwn` and `*TypeBorrow` wrap a
  `*TypeResource`. The Go method signature is the same regardless
  (just `*<R>Handle`); the wrapper distinguishes only because the
  wire-level lifecycle differs (own consumes, borrow lends).

Each `Type` exposes a `GoType()` (Go type expression) and
`HostTypeExpr()` (a Go expression evaluating to a `host.TypeExpr`,
used to materialize `host.FuncType` declarations in generated bind
code). Both are co-located with the IR type so adding a new type
keeps the related logic in one place.

`Interface` carries `WrapName` (consumer-side type, e.g. `Counterhost`),
`ImplName` (implementer-side, `CounterhostImpl`), and `GoName` as
an alias for `WrapName` kept for template back-compat. A user
implementing the host side implements `<Iface>Impl`; a user holding a
wrapped wasm exporter holds a `<Iface>` value.

`Imports []ImportRef` records cross-interface imports detected during
lowering. Each `ImportRef` carries the source interface's WIT identity,
its computed Go package path (derived from `Options.PackageRoot`), the
package alias for emitted code, the field name on `Deps` (e.g.,
`StreamsInst`), and a list of `ImportedResource`s. Imported
resource entries get rewritten in a post-pass (`rewriteImports`
in `lower.go`) to point at the importing interface's copy with
`ImportRefField`, `GoPackageQualifier`, and `LenderInstField`
populated — this keeps emission decisions local to a single
`*TypeResource`.

## Lowering

`lower.go` walks the wit AST in three passes inside `LowerWorld`:

1. **Allocate Interface stubs** for every imported interface in the
   chosen world.
2. **Walk typedefs** in each interface — populates `iface.Records /
   Variants / Enums / Flags / Resources` plus a shared `typeDefIR`
   `map[*wit.TypeDef]Type` so cross-references resolve to the same
   pointer (load-bearing for pointer-equality dedup).
3. **Walk functions**, classifying via `wfn.Kind`
   (`*wit.Freestanding | *wit.Constructor | *wit.Method | *wit.Static`)
   and attaching to the right interface or resource.

Multi-pass is necessary because resource methods and constructors
reference the resource type itself, and signatures across interfaces
reference each other's typedefs.

## Naming

`name.go`:

- `GoName(s, exported)` — kebab → Pascal/camel with an initialism
  table (`id` → `ID`, `url` → `URL`, `http` → `HTTP`, `json` → `JSON`,
  `uuid` → `UUID`). The first segment of an unexported identifier is
  always lowercased even if the lookup would have produced an
  initialism — exporting trumps the initialism table.
- `GoPackageName(s)` — kebab → flat lowercase. `terminal-input` →
  `terminalinput`. WIT interface names become Go directory names.
- `TypeName(t)` — recursive, deterministic name builder for anonymous
  compounds. `tuple<list<u32>, option<string>>` →
  `TupleListU32OptionString`. The same string is used both as a
  generated struct/interface name and as the suffix on per-type
  helpers, which makes call sites readable (`liftFlatListString`,
  `toGoMemTupleU32String`).
- `GoTypeOf(t, currentIface)` — Go type expression at a use site.
  For nominals it qualifies with the owning interface's package alias
  when `OwnerInterface != currentIface`; for anonymous compounds it
  calls into `TypeName`.

Per-package dedup means two `tuple<u32, string>` instances anywhere
in an interface produce one struct + one set of helpers. Across
packages the same anonymous compound is intentionally duplicated —
no cross-package import for an anonymous type.

## ABI metadata

`abi.go` provides the canonical-ABI numbers the emitters need:

- `Size(t)` — bytes occupied in linear memory.
- `Align(t)` — alignment requirement.
- `FlatSlots(t)` — wasm flat-slot count.
- `FieldOffset(types, idx)` — offset of field `idx` in a tuple/record's
  memory layout, with C-style per-field alignment padding.

These functions are pure and total over the IR `Type` universe;
`Size`/`Align`/`FlatSlots` for resource handle types are 4/4/1 (one
i32 wire handle).

## Type collection and dedup

`typededup.go` walks an interface's signatures collecting the unique
`Type` values referenced anywhere. Filters
(`HelperTypes`, `RecordTypes`, `VariantTypes`, …) split the result for
specific emission decisions. The dedup key is structural-pointer
identity for nominals (since lowering shares pointers via `typeDefIR`)
and recursive deep-equal-by-name for anonymous compounds.

The collection result drives:

- Anonymous-compound struct declarations in `<iface>.go`.
- Per-type helper emission in `<iface>.bind.go` and `<iface>.wrap.go`.
- `b.AddType` and `b.AddResource` calls inside `NewFactory`.

## Emission

### Per-type helpers — two families

For every non-primitive type T in `HelperTypes(types)` the emitter
unconditionally produces eight functions, four per family. The split
keeps the two call sites' lifecycle assumptions separate without a
per-type predicate at the call site.

**`to/fromGo` family — trampoline-side, no canon transfer.**
Emitted into `<iface>.bind.go`. The trampoline is invoked *after*
canon's transfer plan has placed handles in cc's representation
already, so these helpers are pure marshalers between cc's wire form
and Go values. They never call `IssueOwn` / `ReleaseOwn` — that
already happened upstream.

```go
func toGoFlat<X> (ctx, cc, h, stack []uint64) (X, error)
func toGoMem<X>  (ctx, cc, h, ptr uint32)    (X, error)
func fromGoFlat<X>(ctx, cc, h, stack []uint64, v X) error
func fromGoMem<X> (ctx, cc, h, ptr uint32,    v X) error
```

**`lift/lower` family — wrap-side, performs the transfer.**
Emitted into `<iface>.wrap.go`. The wrapper *is* the cross-component
transfer: helpers carry both `caller` and `callee` `*host.CallContext`
pointers and cross-issue handles between the two tables themselves
(`callee.ReleaseOwn(tr, oh) → rep`; `caller.IssueOwn(tr, rep) →
callerH`).

```go
func liftFlat<X> (ctx, caller, callee, h, stack []uint64) (X, error)
func liftMem<X>  (ctx, caller, callee, h, ptr uint32)    (X, error)
func lowerFlat<X>(ctx, caller, callee, h, stack []uint64, v X) error
func lowerMem<X> (ctx, caller, callee, h, ptr uint32,    v X) error
```

The emitter never inspects T to decide which subset to emit. The two
families are always emitted in lockstep; for T whose tree contains no
resource the bodies are nearly identical pure-data marshalers and the
`caller` parameter is unused (named, not blank, so the signature stays
uniform). The 2× helper-volume cost buys a one-line emit rule, no
`containsResource` predicate, and a fixed call-site rule:

- Trampoline emission (`emit_factory.go`'s `wrap*` bodies) calls
  `toGoFlat<X>` / `fromGoFlat<X>`.
- Wrap emission (`emit_wrapper.go`'s wrapper method bodies and
  `*<R>Remote` method bodies) calls `liftFlat<X>` / `lowerFlat<X>`.

Helper functions are package-internal (lowercase), so unused emissions
in a given package do not produce build warnings.

Primitives, enums, and flags are inlined at the call site — their
lift/lower bodies are one expression each, and indirecting through a
helper would only add noise.

### Flat vs mem

Both modes are emitted for every helper-eligible type because the
canonical ABI cap (`maxFlatParams = 16`, `maxFlatResults = 1`) means
the same type may appear in either position depending on the
surrounding signature: a `string` returned at top level lowers via a
single i32 result-pointer (mem mode), but the same `string` inside a
tuple appears at flat slots. Each helper reads/writes through
`cc.Memory()` and `cc.Realloc()` reached via the `host.CallContext`
parameter; memory and realloc are not separate parameters.

### Trampolines (`emit_factory.go`)

For each WIT function the factory emits a `wrap<Name>(f *Factory)
host.Func` closure. The closure body:

1. Recovers per-instance state via `h.UserState().(*instanceState)`.
2. Calls `toGoFlat*` / `toGoMem*` to lift each param.
3. Invokes the user's impl method.
4. Calls `fromGoFlat*` / `fromGoMem*` to lower each result.
5. Returns `nil` or a wrapped infrastructure error.

Resource methods, constructors, and statics use canonical bracketed
WIT names (`[constructor]counter`, `[method]counter.increment`,
`[static]counter.open`) for the `b.AddFunction` registration; the
generated Go names hoist constructor → `New<R>` and static `f` →
`<R><F>` onto the parent interface, matching the WIT spec's hoisting
rule.

### Wrapper (`emit_wrapper.go`)

`WrapInstance(caller *host.ComponentInstance, callee
*wacogo.ComponentInstance) <Iface>` returns the consumer surface for a
wasm-exported interface. It pre-resolves each export to a
`*host.ExportedFunc` so per-method calls just drive `CallRaw` with two
closures: a write closure that lowers args into the callee's wire
form, and a read closure that lifts results back into the caller's
representation. The wrapper is plan-free — `CallRaw` deliberately
bypasses the canon plan, so every lift/lower op is explicit in those
closures.

Resource methods on a remote (`*<R>Remote`) follow the same shape: the
remote dispatcher carries a `*counterFns` value with one
`*host.ExportedFunc` per method.

## Resource handle types

Each WIT resource generates a `*<R>Handle` struct whose lifecycle is
managed by an embedded `handleState` interface. The state machine
covers four states — `unregisteredHandleState` (impl held in Go,
not yet in any canon table), `boundHandleState` (registered in a
specific instance's canon table), `invalidatedHandleState`
(transferred out, further bind attempts fail), and a
shortcut-borrow state — and the constructor naming convention is
described in `CLAUDE.md` ("Resource Handles"). The design rationale
is that handles need to be constructible *before* an instance is
available (unregistered), bound lazily on first transfer (bound),
guarded against double-use (invalidated), and able to round-trip a
borrow without minting a canon entry (shortcut-borrow).

Witgen emission is the consumer of that design:

- `<iface>.go` declares `<R>Handle`, the `handleState` impls, and
  the local-dispatch / remote-dispatch dispatcher structs.
- `NewXxxHandle(impl)` and `NewXxxHandleIn(definer, impl)` are
  emitted as package-level free functions — same-package vs
  cross-package construction.
- `NewXxxHandleFrom(rh wacogo.ResourceHandle, impl)` is a third
  constructor used by the wrapper's read closures when a call
  returns an own that the consumer needs to wrap as a `*<R>Handle`
  bound to an existing canon handle. `impl` is either the local
  Go implementation (when reachable) or a forwarder dispatcher
  built by the importing package.
- `HandleValueFor(cc, owner, h)` is the cross-package companion to
  `bind`, used when a different package needs to drive an unbound
  handle through registration in this resource's defining instance.

The lift/lower side of resources splits four ways across the two
helper families and the own/borrow distinction; the canonical
description lives in `CLAUDE.md`'s "Resource Handles" section and
the "Resource handles across the canon/core boundary" section of
`docs/canonical_abi.md`. The witgen emitters mechanically realize
those tables. The headline points the emitter relies on:

- `to/fromGo` decode of `borrow<R>` for local R reads the rep
  directly from the wire (canon's same-component shortcut).
- `to/fromGo` decode of `own<R>` always reads a canon handle off cc's
  table — own's contract requires a canon entry for the eventual
  `[resource-drop]`.
- `lift/lower` always performs the cross-table handoff
  (`ReleaseOwn`/`IssueOwn` for own; `Rep`+`Lend` / `IssueBorrow` for
  borrow).
- Borrow cannot appear as a result per WIT, so the borrow encode
  paths in both families return an error if invoked.

## Error and context propagation

Generated code is panic-clean: no `panic(` in any file under
`internal/witgen/testdata/genfixtures/**/*.go`, no `context.Background()`
literals in `internal/witgen/**`. The mechanism:

- **`host.Func` returns `error`.** The wazero adapter in `host/`
  surfaces a non-nil callback error as a wasm trap on the calling
  instance. Trampoline bodies always end with `return nil` or a
  wrapped error.
- **Every method takes `ctx context.Context` first and returns
  `error` last.** The outer `error` is reserved for *infrastructure
  failures* (CallRaw, realloc, memory access, handle-table errors,
  ctx cancellation). The *business outcome* declared by a WIT
  `result<T, E>` is a separate inner `Result*` value emitted in the
  package alongside variants and records.
- **`Result*` is a sealed interface plus `<R>Ok` / `<R>Err` case
  structs**, named via the same `TypeName` convention used for other
  anonymous compounds (`ResultU32String`, …). Two methods sharing the
  same WIT result shape share the same generated trio.
- **`NewFactory(ctx, e)` and `Factory.NewInstance(ctx, impl, opts...)`
  take a `ctx`** which threads into `b.Build(ctx)` and
  `f.comp.Instantiate(ctx, ...)`.
- **`*<R>Handle.Drop(ctx) error`** captures the drop-time ctx, not
  the call-time ctx (the closure captured at bind time stores
  `dropFn func(ctx) error` and `Drop(ctx)` passes the user's ctx
  through).
- **Memory operations propagate errors.** `cc.Memory().ReadX` /
  `Write` / `cc.Realloc` errors flow up through the helper return
  with context: interface name, helper name, and op
  (`fmt.Errorf("wacogo/witgen: %s.%s: %s: %w", iface, helper, op,
  err)`).
- **Variant discriminant out of range** returns a typed error rather
  than panicking.

The `primMemReadStmt` / `elemMemReadStmt` emitters in
`emit_factory.go` are statement emitters parameterized by an
`errSink` describing the surrounding helper's failure-return shape
(`return X{}, err` for the lift family, `return err` for the lower
family). The same emitter serves both families.

A verification test walks `testdata/genfixtures/**/*.go` and asserts
both invariants (no `panic(`, no `context.Background()`); CI keeps
the goldens honest.

## Validation

`validate.go` runs after lowering, before emission. Returns
aggregated errors via `errors.Join` for:

- Go-name collisions across freestanding funcs and resource
  ctor/method/statics.
- Reserved Go keywords used as parameter names.
- Unsupported `result<_, E>` shapes (E must be string or void).

Validation errors carry the offending interface and function for
clear diagnostics.

## Cross-package emission

When an interface's signatures reference types defined in another
interface — resources or nominal compounds (records, variants,
enums, flags) — lowering captures this via `Interface.Imports
[]ImportRef`. The `rewriteImports` post-pass (`lower.go`) produces
per-iface shallow copies of the imported `*TypeRecord` / `*TypeVariant`
/ `*TypeEnum` / `*TypeFlags` / `*TypeResource` with `ImportRefField`,
`GoPackageQualifier`, and (for resources) `LenderInstField`
populated. The originals on the defining interface must stay
unmutated; only the qualifier-tagged copies feed emission.

The walker recurses into structural compounds (`TypeOption.Elem`,
`TypeList.Elem`, `TypeTuple.Elems`, `TypeResult.Ok`/`.Err`,
`TypeRecord.Fields`, `TypeVariant.Cases[i].Payload`) so transitive
imports surface even when the user never names the foreign type
directly. `TypeName` disambiguates anonymous compounds whose
parameters cross package boundaries (e.g. `OptionStreamsFoo` vs
`OptionFoo`) so two interfaces can declare the same shape without
collisions.

Emission rules:

- **Reference, don't re-emit.** `B`'s signatures use `A.Foo` directly
  — there is no "import nominal types into B" step. Aliases preserve
  cross-package identity.
- **Re-register `host.TypeExpr` locally.** `B`'s `NewFactory` calls
  `b.AddType(...)` for imported records/variants/etc. The transfer
  is wire-level field-by-field, so identity across components is
  not required for these (only for resources, which `AddResourceRef`
  handles).
- Explicit `import "<pkg>"` statements get written into each
  generated file (goimports cannot resolve testdata fixture import
  paths reliably).
- Cross-package resource references are qualified as
  `<otherPkg>.<R>Handle` / `<otherPkg>.<R>Impl`.
- `b.AddResourceRef("<resource>")` is emitted in `NewFactory` and
  `host.WithResourceFrom(...)` in `NewInstance` per imported
  resource type.

`HandleValueFor` and `New<R>HandleFrom` are also exported from
each defining package so importing packages can drive cross-package
construction without reaching into private state.

### Forwarder dispatch for cross-package resource methods

When a wrapper-side `*<R>Remote` for a foreign-defined resource
needs to invoke a method, the call must reach *this* package's
imported instance — not the defining package's local impl. The
generated `Factory.NewInstance` precomputes one
`*host.ExportedFunc` per remote method on a per-importer
`<R>FwdFns` struct, and each `*<R>Remote` carries a pointer to
that struct. Method dispatch is `fns.<methodName>.CallRaw(...)` —
a struct-field deref, not a per-call map lookup.

Same-package lift uses `(*core.ComponentInstance).LookupExtern`
to recover the local Go impl when a returned own refers to a
locally-defined resource. Cross-package lift constructs a
`<otherPkg>.New<R>HandleFrom(rh, fwd)` carrying the importer's
forwarder dispatcher, so subsequent method calls round-trip back
through this package's ExportedFunc slots.

## Public factory shape: `NewFactory` + `Deps`

Every witgen-emitted package (defining or importing) emits a
`Deps` struct and a `NewInstance(ctx, impl, deps *Deps, opts...)`
factory. The shape is uniform regardless of import count:

```go
type Deps struct {
    StreamsInst   *wacogo.ComponentInstance // one field per imported iface
    FilesystemInst *wacogo.ComponentInstance
}

func (f *Factory) NewInstance(
    ctx context.Context,
    impl <Iface>Impl,
    deps *Deps,
    opts ...host.InstantiateOption,
) (*host.ComponentInstance, error)
```

Rules:

- **Imports always typed `*wacogo.ComponentInstance`.** Never
  `*host.ComponentInstance` — the host wrapper is an implementation
  detail of host-defined components, and consumers may legitimately
  satisfy an import with a wasm-loaded instance.
- **`Deps` struct, not positional params.** A struct instead of
  positional `*wacogo.ComponentInstance` parameters lets package
  authors add new imports without breaking existing call sites,
  and keeps imports legible at the call site (`&Deps{StreamsInst: s}`).
- **Validation.** `nil` `deps` with non-empty imports panics; nil
  fields panic listing every missing dep. Empty `Deps{}` for an
  interface with no imports still requires either `nil` or a
  non-nil empty struct — generated code defines both shapes.

## Test fixtures

- `testdata/wit/*.wit` — WIT inputs.
- `testdata/genfixtures/<ns>/<pkg>/<iface>/<iface>.{go,bind.go,wrap.go}`
  — committed generated outputs, verified by `TestGolden`
  (`golden_test.go`). Re-run with `-update` to regenerate after
  intentional changes. Placing the tree under `testdata/` hides it
  from `go test ./...` auto-discovery.
- `fixturetests/*_test.go` — hand-written tests in the
  `fixturetests_test` package that import the generated bindings and
  exercise them through small WAT consumers. These cover end-to-end
  correctness; the genfixtures themselves are pure artifacts.

## See also

- `CLAUDE.md` — "Resource Handles" section: the `*<R>Handle` state
  machine spec.
- `docs/canonical_abi.md` — the canon/core boundary, ResourceTable
  and ResourceHandle interfaces, gocall `*ValOwnHandle` semantics
  the consumer-side `lift/lower` family interacts with.
- `docs/host_layer.md` — host wrapper, extern table, the
  CallContext shape witgen-emitted trampolines receive.
- `internal/witgen/README.md` — short pointer-style sibling to this
  doc, oriented at code spelunking.
