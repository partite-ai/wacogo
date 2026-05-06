# Host Layer

The `host/` package lets Go code stand up a component-model component
whose exports are backed by Go functions rather than wasm bytes. The
result is a `*host.ComponentInstance` that, once unwrapped via
`Core()`, can be handed to `wacogo.WithInstanceImport` exactly like
an instance loaded from a `.wasm` binary. A wasm consumer cannot tell
the two apart at the import boundary — the type handle, the canon
resource table, the `Close` lifecycle, and the `CheckInstantiation`
subtype check are all the same.

This document covers the layering, the per-instance state model, the
builder/template/instance split, host-defined resources and the
extern table, and host-side imports. For the canonical-ABI handle
rules the host respects, see the "Resource Handles" section of
`CLAUDE.md` (top-level) and the "Resource handles across the
canon/core boundary" section of `docs/canonical_abi.md`.

## Why `host/` is its own package

Two layering choices fall out of the rule that `internal/core` must
hold no host-specific concept:

- **`internal/core` is the wasm-runtime layer.** It owns
  `*core.ComponentInstance`, the canon bridge, `*core.CallContext`,
  the wazero plumbing — everything that a wasm-loaded instance and a
  host-built instance share. The wasm-loaded path
  (`engine_instantiate.go`) and the host path both produce the same
  `*core.ComponentInstance` type via `core.NewInstance(*InstanceSpec)`
  (`internal/core/instance.go`). `InstanceSpec` carries the optional
  `ExternTable` slot the host wrapper supplies; user state, cleanups,
  and lifecycle hooks live only on the host wrapper.
- **`host/` is the Go-implementation layer.** Everything a host user
  sees — `host.Builder`, `host.Component`, `host.ComponentInstance` —
  lives here, on top of `core.*` types. Witgen-emitted bindings live
  outside the wacogo module and import `host/` only.

`*host.ComponentInstance` is the canonical wrapper. It owns the
extern table, the user-state slot, the cleanup list, and the
pre-close check list. The underlying `*core.ComponentInstance` is
reachable via `Core()` for the one place it's needed — handing the
instance to `wacogo.WithInstanceImport`. Per-instance resource-type
identities live on `core.ComponentInstance.types` (populated through
`InstanceSpec.BuildExports`); host-defined `*core.TypeResource`
values reach there via the same path.

The reason the wrapper is required (and not just a `core` field
group): user state and extern-table identity are concepts the core
runtime has no business knowing about, and the witgen `Factory` needs
a place to attach per-instance Go state without a parallel
`sync.Map[*core.ComponentInstance]*state` registry. A real wrapper
struct gives both `host` and witgen-emitted code one stable identity
to thread through.

## Three lifecycle objects

`host/` mirrors `wacogo`'s own load/instantiate split, scaled down
one level:

| Object | What it is | Lifetime |
|---|---|---|
| `*host.Builder` | Mutable accumulator for declarations | One per declaration session |
| `*host.Component` | Reusable, immutable template | Long-lived; closed when no more instances will be minted |
| `*host.ComponentInstance` | Live, stateful instance | One per consumer wiring |

`Builder.Build` (`host/builder.go`) finalises declarations into a
`*host.Component`. `Component.Instantiate` (`host/component.go`)
mints fresh instances on demand. The builder is rejected after Build;
the template is rejected after Close; instances close independently
of the template they came from.

This split exists for the same reason the root engine has it:
arena/validator and stub-bytecode work happens once at Build time;
per-instance memory, per-instance resource identity, per-instance
extern table, and per-instance type-resource pointer identity happen
each `Instantiate`. The component model defines a
`(component, instantiation)` pair as a distinct type universe — two
instances of the same `*host.Component` therefore produce
non-interchangeable `*core.TypeResource` values.

## Builder API

`Builder` (`host/builder.go`) accepts these kinds of declaration:

- **`AddFunction(name, ty, fn)`** — register a function export. `fn`
  is a `host.Func` (`host/func.go`): a flat-boundary callback that
  receives `(ctx, *core.CallContext, *host.ComponentInstance, []uint64)`.
  No automatic lift/lower runs at this layer — the user reads and
  writes the canonical-ABI flat stack directly. Higher-level codegen
  (witgen) wraps this in typed Go signatures.
- **`AddType(name, expr)`** — register a value type. Returns a
  `*TypeRef` usable wherever a `TypeExpr` is accepted. `name=""`
  registers a structural type purely for sharing across signatures.
- **`AddResource(name, dtor)`** — register a host-defined resource
  type. Returns a `*ResourceType` that constructs `Own()` / `Borrow()`
  `TypeExpr`s for use in signatures.
- **`AddResourceAlias(name, rt)`** — re-export an existing
  `*ResourceType` under an additional name. Both exports share one
  resource identity at runtime.
- **`AddResourceRef(name)`** — declare a *placeholder* for a resource
  type that will be supplied at `Instantiate` time by another
  component instance via `host.WithResourceFrom`. This is the
  cross-component re-export hook (e.g., a `filesystem` component that
  uses `streams.stream`).
- **`AddCoreModule(name, wasmBytes)`** — register a precompiled core
  wasm module exported under `name`. The module is compiled
  immediately and shared across all `Instantiate` calls.
- **`AddNestedInstance(name)`** — declare a nested-instance export.
  Returns an `*InstanceBuilder` (`host/instance_builder.go`) that
  accepts the same declarations recursively, scoped to the nested
  instance.

### Resource methods are plain `AddFunction` exports

There is no separate API for declaring constructors, methods, or
static functions on a resource type. Spec-level resource methods
are registered with `AddFunction` using the canonical bracketed
WIT names:

| WIT shape | `AddFunction` name |
|-----------|-------------------|
| `[constructor]<R>` | `[constructor]<R>` |
| `[method]<R>.<f>` | `[method]<R>.<f>` |
| `[static]<R>.<f>` | `[static]<R>.<f>` |

The bracketed prefixes are component-model conventions encoded in
the export name; they're not separate Builder kinds. Witgen-emitted
factories register these names verbatim.

### Scope-tree model

`Builder` holds a `*scope` tree (`host/scope.go`). Each
`AddNestedInstance` adds a child `*scope`; declarations are
appended to the *current* scope, and `Build` walks the tree
producing a parallel `*scopeRuntime` on the `*Component`. Names
must be unique within one scope but may collide across nested
scopes. Resource-type IDs are allocated from one builder-wide
counter (not per-scope) so each `*ResourceType` has a stable
instance-wide slot in `trSlice`. `AddResourceAlias` shares the same
`*core.TypeResource` identity (and the same arena `ResourceID`)
between the original export and the alias.

### TypeExpr — host's own type vocabulary

The host package owns its declaration vocabulary so that `core.Type`
remains a sealed interface. `TypeExpr` (`host/types.go`) is a
private-marker union with implementations: `prim` (the package-level
`Bool` / `U32` / `String` / ... vars), `*TypeRef`, the structural
compounds `List` / `Tuple` / `Option` / `Result`, and the nominal
compounds `Record` / `Variant` / `Flags` / `Enum`.

**Inline-vs-registered rules** (enforced at Build time):

- Primitives and own/borrow handles are always inline.
- Structural compounds may be inline or registered via `AddType`.
- Nominal compounds *must* be registered via `AddType`; inline use
  is rejected by `host/translate.go`. The reason: a `core.TypeRecord`
  carries field names by reference and represents a nominal type —
  repeated inline use would mint distinct records that nonetheless
  need to subtype-match.

### Build-time work

`Build` (`host/builder.go`) walks the scope tree and:

1. Validates names (no collisions per scope) and assigns each
   `*ResourceType` / `*ResourceTypeRef` a `componentResourceTypeID`
   from one shared counter on the Builder.
2. Compiles the stub bytecode covering every declared func + dtor
   across the whole nested-scope tree (see "Stub module" below).
   One `wazero.CompiledModule` shared across instances.
3. Computes each function's canonical-ABI flat signature
   (`flattenFuncForStub` in `host/translate.go`). Resource-identity-
   independent (own/borrow both flatten to i32), so it runs at
   Build time directly off the `TypeExpr` tree.

Per-instance work — translating `TypeExpr` and `FuncType` into
`wasmparser` arena entries, synthesising the `*wasmparser.InstanceType`
that `wacogo.WithInstanceImport` advertises to consuming components,
and minting per-instance `*core.TypeResource` values — happens
inside `Component.Instantiate` (`buildPerInstanceWpType` in
`host/component.go`). One `*InstanceType` per `Instantiate`, with
fresh `ResourceID`s reflecting any `WithResourceFrom` bindings, so
that the subtype checker treats two instances of the same template
as having distinct resource identities (per the canonical ABI's
`(component, instantiation)` rule).

## Stub module + per-instance host module

This is the load-bearing structural choice in the host layer.

### Why two wasm modules per instance

A wasm consumer that imports a host instance reaches its functions
through wazero's `experimental.WithImportResolver`. That resolver
will only accept *real wasm modules* as import providers — wazero's
host-module type fails the internal `*wasm.ModuleInstance` assertion
inside `store.resolveImports`. So we cannot present a wazero host
module directly as the importable module. Instead, every host
instance carries:

- A **stub module** (`host/stub.go`) — real wasm bytecode, exporting
  `memory`, `realloc`, and one wrapper function per declared export
  (and one per host-side resource dtor). Each wrapper imports the
  corresponding Go function from a fixed module name and forwards
  calls through.
- A **host module** (`host/host_module.go`) — a wazero host module
  whose Go functions close over the specific
  `*host.ComponentInstance` they belong to. Per-instance.

At `Instantiate` time, the stub is instantiated against the host
module via `experimental.WithImportResolver`, rewriting the stub's
fixed host-module name to the per-instance host module's name
(`host/stub.go`).

### Why per-instance host modules

Each `Instantiate` builds a fresh host module whose Go closures
capture the wrapper directly (`host/host_module.go`). Every host
trampoline's first line is a direct field access on the
`*host.ComponentInstance` already in scope, and per-resource dtor
closures bind their wrapper at construction (`host/component.go`).
No side map, no type assertion at dispatch time, no shared registry.

The cost is one extra wazero host module per instance.

### Stub `realloc`: bump-reset per call

The stub's `realloc` (`host/stub.go`) is a wasm-implemented bump
allocator that resets at the top of every exported wrapper. One
i32 global; each wrapper prologue stores `stubBumpInitial` (16 —
non-zero, 8-aligned) to that global before forwarding.

The reason this works: host functions don't need a real allocator.
They need scratch space for the duration of a single canonical-ABI
call. Resetting at the top of every top-level call gives that for
free, with no fragmentation and no growth. The reserved first 16
bytes of stub memory mean realloc never returns null (the canonical
ABI traps on null pointers).

## Per-instance state model

`*host.ComponentInstance` (`host/component_instance.go`) carries:

| Field | Source | Purpose |
|---|---|---|
| `core *core.ComponentInstance` | `core.NewInstance` | Wasm-runtime backing |
| `extTable *externTable` | Allocated at construction | Extern handle → Go `any` |
| `userState any` | `WithUserState` | Witgen-emitted impl state |
| `cleanups []func()` | Internal | Run after `core.Close` |
| `preCloseChecks []func() error` | Internal | Run before `core.Close` |

Per-instance `*core.TypeResource` values do not live on the wrapper
— they are pushed into `core.ComponentInstance.types` via
`InstanceSpec.BuildExports` so the canon bridge can treat them
identically to wasm-defined ones.

`UserState` is set once at Instantiate (via `WithUserState` —
`host/options.go`) and is immutable thereafter. There's no setter,
because per-instance state churn during a call would cross the line
between "configuration at construction" and "mutable behaviour" the
wrapper isn't trying to model.

### One TypeResource per (template, instance)

Each `AddResource` mints a fresh `*core.TypeResource` per
`Instantiate` call (`host/component.go`). Two instances of the same
template hold distinct TR pointer identities. The canon ABI's
resource-type identity is `(component, instantiation)`, not
`component` — a wasm consumer that imports two host instances of
the same template must see two distinct nominal types. The dtor
closure is also per-instance (it captures the per-instance wrapper),
so the per-instance TR is the natural place to hold it.

### Per-instance dtor closures

Each `AddResource` produces one dtor closure per `Instantiate`
(`host/component.go`). The closure captures the wrapper directly:

```
dtor := func(ctx, rep) error {
    obj, _ := h.releaseResource(ExternHandle(rep))
    // obj may be nil if the rep wasn't minted via RegisterResource
    // (e.g., a guest using canon resource.new with a chosen rep).
    if userDtor != nil {
        return userDtor(ctx, h, obj)
    }
    return nil
}
tr := core.NewTypeResource(dtor)
```

The closure runs whenever canon's terminal-drop orchestration fires
for any handle this TR identifies — from any component, including
cross-component drops. canon resolves the defining instance from the
TR pointer and runs the dtor under the defining instance's
reentrance gate. Since the closure captures the defining wrapper,
extern-table cleanup always lands on the right table, regardless of
which component triggered the drop.

`ResourceDtor` (`host/resource.go`) takes
`(ctx, *ComponentInstance, obj)` — no `*CallContext`. User
destructors do cleanup against `obj`, never canon transfer ops, so
a CallContext would only invite confusion. Cleanup errors propagate
back up the call path that triggered the drop.

## Resources and the extern table

The host layer separates two ideas the canon ABI's resource table
deliberately conflates:

- **Canon table** (`*canon.ResourceTable` inside
  `*canon.ComponentInstance`) — owns `(handle, kind, rt, rep, numLends)`
  per the spec. Stores integer reps. Lives one-per-instance regardless
  of host vs wasm origin. Reached through
  `*core.CallContext.IssueOwn` / `Rep` / `Drop` etc.
- **Extern table** (`host/extern_table.go`) — owns the Go `any` value
  that the rep refers to. Lives only on host instances. The canon
  rep stored by host-defined resources *is* an `ExternHandle` cast
  to `uint32`; the host wrapper hides this cast inside its three
  resource methods.

### `ExternHandle` as a distinct type

`host/component_instance.go` defines `ExternHandle uint32`. The cast
to `uint32` is permitted only at the canon boundary (e.g.,
`uint32(eh)` to feed `cc.IssueOwnHandle`). The reason for a distinct
type: host-package code reads as type-safe — you cannot accidentally
pass a canon handle where an extern handle is required, even though
both are `uint32` underneath.

The extern table itself (`externTable`, `host/extern_table.go`) is
unexported. There is no `host.LookupRep` or `host.externTableOf`
package helper. All access goes through methods on the wrapper:

| Method | Purpose |
|---|---|
| `RegisterResource(obj) ExternHandle` | Stash `obj`; allocate fresh handle |
| `LookupResource(eh) (any, bool)` | Read `obj`; do not free |
| `releaseResource(eh) (any, bool)` (unexported) | Free entry; return `obj` |

`RegisterResource` is called by user code (or witgen-emitted
trampolines) when minting a host-defined resource. `LookupResource`
is the read-side path on the same-component shortcut for borrow
methods. `releaseResource` is unexported and is invoked only from
the host module's dtor trampoline (`host/host_module.go`) — never
directly by user code, and never by the per-`*core.TypeResource`
dtor closure (which calls it via `h.releaseResource` in the same
package).

The single lifecycle invariant the wrapper enforces:
`Close` rejects if `extTable.liveCount() > 0` (see
`host/component.go`'s `preCloseChecks`). That keeps "outstanding
owns" from silently leaking when a consumer forgets to drop.

### `core.ExternTable` — the read-only canon-side view

The host wrapper implements `core.ExternTable`
(`internal/core/extern_table.go`):

```go
type ExternTable interface {
    Lookup(rep uint32) (any, bool)
}
```

The wrapper passes its `extTable` through `core.InstanceSpec.ExternTable`
at `core.NewInstance` time. Code on the canon side that needs to
recover the Go object behind a host-defined resource calls
`(*core.ComponentInstance).LookupExtern(rep)`. This is the *only*
sanctioned cross-component handle resolution path — there is no
package-level helper that opens the table to outside readers.

The `core.ResourceHandle.Instance()` accessor returns the
`*core.ComponentInstance` whose canon table holds the entry; when
that instance has an extern table, `LookupExtern` reaches the Go
object. Together with `core.ResourceHandle.Type()` and
`(*core.TypeResource).Instance()` this forms the closed surface
witgen-emitted forwarders rely on. No additional public surface is
permitted on top.

### Cross-component re-export of resource types

`AddResourceRef(name)` (`host/builder.go`) declares that *this*
host component re-exports a resource type defined by *another*
component instance. The returned `*ResourceTypeRef`
(`host/resource_ref.go`) is a Build-time placeholder. Its
`*core.TypeResource` arrives at `Instantiate` time via
`host.WithResourceFrom(ref, lender, lenderExport)`
(`host/options.go`).

This is what lets a generated `filesystem` host component — which
imports `streams.stream` — share a single resource-type pointer
identity with the `streams` host component. Without it, two
distinct host components could not interoperate over a shared
resource type.

The `componentResourceTypeID` counter on the Builder allocates one
slot per `*ResourceType` and one per `*ResourceTypeRef`, so all
resource declarations (defined and re-exported) share a single
dense ID space within the template. At Instantiate
(`host/component.go`), each ref is resolved by
`Component.resolveResourceRefs` against the supplied
`WithResourceFrom` options to pick out the lender's
`*core.TypeResource`; the resulting identity is published as a
type export of the new instance via the
`InstanceSpec.BuildExports` callback so that downstream consumers
see one nominal type for both the lender and the importer.

### Alignment with handle management

The witgen-emitted `*<R>Handle` state machine
(`unregisteredHandleState`, `boundHandleState`,
`invalidatedHandleState` — described in `CLAUDE.md`'s "Resource
Handles" section) sits on top of this extern-table infrastructure.
`bind` is the operation that calls `hostInst.RegisterResource(impl)`
and `cc.IssueOwnHandle(tr, uint32(eh))` — the wrapper provides the
first half, the call context the second. The handle-state machine
can therefore be lazy with `cc`: as long as something eventually
reaches a bind site with a live `cc`, the extern entry and canon
entry are issued together.

## Host imports: same-shape factory wiring

A host component can also *import* an interface defined elsewhere
(host or wasm). The witgen-emitted `Factory.NewInstance` takes a
`Deps` struct of `*wacogo.ComponentInstance` fields:

```
func (f *Factory) NewInstance(
    ctx context.Context,
    impl FilesystemImpl,
    deps *Deps,
    opts ...host.InstantiateOption,
) (*host.ComponentInstance, error)
```

Imported deps are typed `*wacogo.ComponentInstance` rather than
`*host.ComponentInstance`. The host wrapper is an implementation
detail of host-defined components, and consumers may legitimately
satisfy an import with a wasm-loaded instance — using the more
general type keeps both paths uniform. `nil` `deps` (or any nil
field) panics at `NewInstance` with the missing field name.

Function-only imports (no shared resource types) don't appear in
`Deps` — those the user wires themselves via `pkg.WrapInstance(inst)`
and stashes on their impl. The factory only mediates
**resource-type binding**, because that's the binding canon needs
at the type-identity level.

Inside `NewInstance`, the factory walks the lender's resource
exports (via the witgen-emitted `XxxResourceType(*ComponentInstance)`
accessor — see `internal/witgen/`) and appends one
`host.WithResourceFrom(...)` per imported resource. The user's wired
`WrapInstance(...)` and the factory's `WithResourceFrom(...)` are
two complementary touch points: one for function-side calls, one
for resource-type binding.

The shape "where the source instance came from" — host or wasm —
does not matter to the consumer. Everything is a
`*core.ComponentInstance` at the import boundary; canon's transfer
plan handles the rest. The same `WrapInstance` factory works
whether the import is satisfied by a host instance or a wasm
instance.

## CallContext at the host boundary

`*core.CallContext` (`internal/core/call_context.go`) is the single
call-context type. `host/types.go` re-exports it as
`host.CallContext = core.CallContext` (a Go type alias) so that
witgen-emitted bindings — which live outside the wacogo module and
cannot import `internal/core` — can write `*host.CallContext` and
have it be the same pointer type.

A host trampoline receives one CallContext (`host.Func` —
`host/func.go`). The CallContext's `Memory()` and `Realloc(...)`
refer to the stub module's, because the transfer-plan machinery has
already placed lift/lower data in the stub's memory by the time
the trampoline runs. The trampoline reads/writes the flat stack and
reaches resource-table ops through the CallContext.

`host.ExportedFunc` (`host/exported_func.go`) is a slim wrapper
around `*core.ExportedFunc` for the consumer side of cross-component
host-to-host calls. Its `CallRaw` method runs two callbacks
(`write`, `read`) with caller and callee CallContexts that share a
single `*canon.Task` — borrow bookkeeping is drained at call
resolution by `Task.End`, which returns an error if any borrows
remain outstanding (see `internal/core/func.go`'s `CallRaw`). The
two-context shape exists because host-implementer trampolines on the
receive side need their own CallContext for canon ops, while
host-wrapper closures on the caller side need a separate CallContext
to perform the consumer-side lift/lower against the consumer's
canon table.

## File map

| File | Role |
|---|---|
| `host/builder.go` | `Builder` accumulator + `Build` (compiles the stub) |
| `host/instance_builder.go` | `InstanceBuilder` for nested-instance scopes |
| `host/scope.go` | Internal scope-tree shape used by Builder + Component |
| `host/component.go` | `Component` template + `Instantiate`, `resolveResourceRefs`, `buildPerInstanceWpType` |
| `host/component_instance.go` | `*ComponentInstance` wrapper, `ExternHandle`, resource methods |
| `host/extern_table.go` | unexported `externTable` storage |
| `host/options.go` | `WithUserState`, `WithResourceFrom`, `InstantiateOption` |
| `host/resource.go` | `ResourceType`, `ResourceDtor`, `Own`/`Borrow` |
| `host/resource_ref.go` | `ResourceTypeRef` placeholder |
| `host/types.go` | `TypeExpr` union + `TypeRef` + `CallContext` alias |
| `host/translate.go` | `TypeExpr` → wasmparser arena + `core.Type` |
| `host/stub.go` | Stub bytecode synthesis + per-instance instantiation |
| `host/host_module.go` | Per-instance host module construction |
| `host/func.go` | `host.Func` callback signature |
| `host/exported_func.go` | `*ExportedFunc` consumer-side wrapper |
| `host/doc.go` | Package doc |
