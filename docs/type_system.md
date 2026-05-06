# Runtime Type System

The runtime type system is wacogo's per-component, per-instance representation
of every component-model type the canonical ABI consumes. It lives in
`internal/core` and is fed by the wasmparser-validated payload stream — never
by the validator's private type arena. The host package layers a small
indexed-lookup view on top so generated code can reach a per-instance
`*TypeResource` from a Builder-time slot ID.

## Why a runtime type system at all

The canonical ABI (lift / lower / cross-component transfer) needs concrete
type metadata to move values across boundaries. Borrowing that metadata from
the wasmparser validator was rejected for three reasons:

1. It leaked validator-private state into the runtime.
2. Resource types are **nominal** — unique per component instance — and the
   validator's structural view cannot express per-instance identity.
3. It conflicted with the load/instantiate split: validator type IDs are
   load-time artifacts; the runtime needs a fresh, per-instance type table.

The system therefore owns its own type universe (`internal/core/types.go`)
and builds it from parser output.

## Two-level representation

```
Component                       (load time, immutable, shared)
  typeResolvers []typeResolver  // 1:1 with the spec's component type index space

ComponentInstance               (instantiation time, per call to Instantiate)
  types []Type                  // parallel to typeResolvers, populated per-instance
  instances []*ComponentInstance// component instance index space
  parent *ComponentInstance     // outer-alias chain
```

Resolvers (`internal/core/type_resolvers.go`) are the bridge. Each one encodes
*how* a type comes into existence (declaration, alias, import). Running a
resolver against an `*resolverCtx` produces the concrete `Type` for its slot.

Both levels are needed because:

- Primitives are invariant, but treating them uniformly with everything else
  keeps the index space contiguous and the runner trivial.
- Structural compounds (record, list, variant, ...) can transitively contain
  nominal types via handles, so they must be re-materialised per instance.
- Handle types (`TypeOwn`, `TypeBorrow`) carry `*TypeResource`, whose identity
  is per-instance.
- Resource declarations mint a fresh `*TypeResource` on each instantiation —
  the only intentionally non-deterministic resolver.

## The Type interface and its variants

`Type` (`types.go`) is a tagged-union interface using a private marker method.
Concrete variants:

- 13 primitive descriptors (`TypeBool`, `TypeS8`..`TypeF64`, `TypeChar`,
  `TypeString`) — all empty structs, addressed by value.
- Compounds: `TypeList`, `TypeRecord`, `TypeTuple`, `TypeVariant`, `TypeEnum`,
  `TypeOption`, `TypeResult`, `TypeFlags`. Children are `Type` values, not
  type indices — composition is by direct embedding.
- `TypeOwn{ResourceType *TypeResource}` and `TypeBorrow{ResourceType *TypeResource}`.
  The handle is one i32 in flat representation regardless of which resource it
  points to; `flatten` and `alignment` ignore the pointer.
- `*TypeResource` itself implements `Type`, so a resource declaration's TypeID
  slot carries the descriptor directly.
- `*FuncType` is a `Type` (so function-type slots fit the same table).
- `*InstanceType` and `*ComponentType` are placeholder structs. They exist so
  every spec-allocated TypeID has a non-nil runtime value; the ABI does not
  consume them today and fields can be added later without disturbing the
  scaffolding.

All struct fields are exported (project convention).

## TypeResource — nominal identity by pointer equality

```go
type TypeResource struct {
    instance *ComponentInstance
    dtor     func(ctx context.Context, rep uint32) error
}
```

Identity is pointer equality on `*TypeResource`. Two handles refer to the
same resource type iff their `ResourceType` fields are the same pointer. This
is the only source of per-instance nominal identity in the runtime:
`resourceResolver.resolve` mints a fresh `*TypeResource` on every call, so two
instantiations of the same component yield two distinct, unequal resource
types.

`instance` carries the defining instance — consumers reach the origin
`*Component` via `instance.component`. canon's cross-component machinery uses
it as an opaque pointer-equality key (see `internal/core/canon_bridge.go`'s
`resourceType.DefiningInstance`) for the same-component shortcut.

`dtor` is a single Go closure invoked by canon's `ResourceTable` when the
last own handle for the resource type is terminally dropped. wasm-defined
resources wrap their `api.Function` into this shape at resolve time;
host-defined resources supply the closure directly at registration time.
`nil` means no destructor.

`dtor` is resolved at the point in the plan where the resource is declared.
The defining core module is guaranteed by declaration order to already exist
at that point — an unresolvable dtor is a load error, not deferred state.

## Resolvers

`typeResolver` (`type_resolvers.go`) is one method:

```go
type typeResolver interface {
    resolve(rc *resolverCtx) Type
}

type resolverCtx struct {
    inst    *ComponentInstance
    imports map[string]any        // satisfies importResolver lookups
    state   *instantiationState   // resourceResolver dtor lookup
}
```

`resolverCtx` is stack-allocated for one `planResolveType.execute` call and
discarded at return. There is no long-lived resolver state.

The concrete kinds:

| Resolver | Returns | Notes |
|----------|---------|-------|
| `primitiveResolver` | a stored `Type` | singleton; field read, no alloc |
| `indexResolver` | `rc.inst.types[idx]` | child reference within one component's type space |
| `listResolver`, `recordResolver`, `tupleResolver`, `variantResolver`, `enumResolver`, `optionResolver`, `resultResolver`, `flagsResolver` | the matching compound `Type` | composes child resolvers |
| `funcResolver` | `*FuncType` | returns nil if any child is unresolved (see below) |
| `ownResolver`, `borrowResolver` | `TypeOwn` / `TypeBorrow` over a `*TypeResource` | reads the resource slot from `rc.inst.types` |
| `resourceResolver` | fresh `&TypeResource{...}` | the only intentionally non-pure resolver |
| `instanceImportResolver` | `rc.inst.instances[idx].exportedType(name)` | uniformly covers imported and locally-instantiated sub-instances since both share the instance index space |
| `aliasResolver` | walks `rc.inst.parent` `outerDepth` times, returns `target.types[idx]` | outer aliases never cross siblings |
| `importResolver` | `rc.imports[importName].(Type)` | direct component-level type imports |
| `instanceTypeResolver`, `componentTypeResolver` | `&InstanceType{}` / `&ComponentType{}` | placeholders |

`funcResolver` returns `nil` rather than panicking when a child resolver
returns nil. That's the load-time fall-back path for function signatures whose
type imports are not plumbed through the `WithTypeImport` carrier — the caller
treats the absent `*FuncType` like the pre-resolver behaviour.

### Invariants

- All resolvers except `resourceResolver` are pure with respect to
  `resolverCtx`. Same context in, same `Type` out.
- `resourceResolver` is deliberately side-effecting. Per-instantiation
  identity is the whole point.
- Forward references within one scope are impossible: the loader emits one
  `planResolveType` per allocated TypeID in declaration order, so every
  dependency is resolved before its dependents run.

## Load-time construction

`engine_load.go` walks parser payloads and allocates one TypeID per
type-allocating site, calling `componentLoader.allocType(resolver)` which
appends to `typeResolvers` and emits a corresponding `planResolveType` step.
Sites that allocate a TypeID:

- `ComponentTypeSectionPayload` entries — dispatched in
  `engine_load_types.go::buildComponentTypeResolver`.
- `AliasInstanceExport` for `SortType` → `instanceImportResolver`.
- `AliasOuter` for `SortType` → `aliasResolver`.
- Direct type imports → `importResolver`.

The loader stores no flattened "expected required exports" or other
validator-derived metadata. Imports are carried by their existing plan steps;
type-import names live on the `importResolver`. The validator's correctness
guarantees let the loader skip defensive shape checks while building
resolvers.

The loader tracks the component instance index space via `loadInstanceInfo`
on `loadScope` so alias lookups and instance-export aliases can resolve
without counting through imports. The same tracking is what lets
`instanceImportResolver` work uniformly across imports and sub-component
instantiations.

## Instantiate-time flow

```go
type planResolveType struct{ typeID uint32 }
```

`planResolveType.execute` (`engine_instantiate.go`) reads the resolver at
`component.typeResolvers[typeID]`, runs it against the instance, and stores
the result in `instance.types[typeID]`.

Plan ordering is deliberate. Earlier plan steps populate `inst.instances`,
`inst.types[0..N-1]`, and the import args before any later step runs. By the
time a `planResolveType{N}` executes, every state it could need is already
in place. There is no two-pass resolution, no cycle detection, no deferral.

Resource dtor wiring inside `resourceResolver.resolve` follows the same
discipline: the resolver looks up the dtor core function index against
`instantiationState.resolveDtor` (the live core function index space) and
wraps the resulting `api.Function` in the closure stored on the new
`*TypeResource`. Wasm-defined dtors and host-defined dtors flow through the
same closure shape so canon's `ResourceTable.Drop` has one calling
convention.

## canon bridge

`internal/core/canon_bridge.go` adapts core's `Type` universe to canon's
visitor interfaces without exposing canon-specific methods on public types:

- `canonType{t: Type}.Accept(canon.TypeVisitor)` is a centralised type switch
  that dispatches each `Type*` variant into the matching `Visit*` call.
- `resourceType{rt: *TypeResource}` satisfies `canon.ResourceType`. Two
  wrappers around the same underlying pointer compare equal as Go interface
  values, which is what canon's `ResourceTable` lookups depend on.
- `FuncTypeParamsAsCanon` / `FuncTypeResultsAsCanon` walk a `*FuncType`'s
  parameter and result slices into `[]canon.Type` for plan compilation.

The `Val*` runtime values live in `internal/canon/val.go` and are re-exported
from `internal/core` and root `wacogo` via Go type aliases. There is no
`Val`-level conversion at any layer boundary — the types are identical.

## Cross-component transfer compilation

Transfer compilation runs at **instantiate time**, not load time.

Reason: `typeResolvers` carry TypeID references, not concrete structural
trees. Compiling a transfer plan requires resolved `Type` values from both
sides' instance tables, which only exist once both instances have run their
`planResolveType` steps. The wiring step that builds the cross-component
adapter therefore resolves both sides' `*FuncType` from their respective
`ComponentInstance.types` and feeds them to `canon.Host.BuildAdapter`.

The trade-off is a heavier instantiate. If that ever shows up as a problem
the structural layout could be compiled once at load time and re-bound to
resource pointers at instantiate.

## Per-instance TypeResource indexing (host package)

The host package mints `*core.TypeResource` values per Instantiate and
threads them through `core.InstanceSpec.BuildExports` so that downstream
consumers can find them via the same `*core.ComponentInstance.ExportedType`
path used for wasm-loaded components.

### Builder-time IDs

`*host.ResourceType` (declared via `Builder.AddResource`) and
`*host.ResourceTypeRef` (declared via `Builder.AddResourceRef`) each carry
a private `componentResourceTypeID` (`host/resource.go`,
`host/resource_ref.go`). Both draw from one counter on the `Builder`, so
every value is unique within one Builder regardless of kind. The ID is
used at Instantiate to locate each declaration's per-instance
`*core.TypeResource` in a dense slice (`trSlice` in `host/component.go`);
that slice in turn drives both `buildCoreFuncType` (to translate
own/borrow `TypeExpr`s into `core.TypeOwn` / `core.TypeBorrow` over the
right `*core.TypeResource`) and the per-instance `*wasmparser.InstanceType`.

### Per-instance slot space

The dense `trSlice` in `Component.Instantiate` is the source of truth for
host-side resource identity within one Instantiate call. Each
`*ResourceType`'s slot holds a freshly minted `*core.TypeResource`; each
`*ResourceTypeRef`'s slot holds the lender's `*core.TypeResource`,
resolved by `Component.resolveResourceRefs` from the supplied
`WithResourceFrom` options.

The same `*core.TypeResource` values are published as type slots on the
underlying `*core.ComponentInstance` (via `InstanceSpec.BuildExports`'
`typeSlots` return), so external consumers — including witgen-emitted
remote wrappers — read them through `(*core.ComponentInstance).ExportedType(name)`.
There is no `host`-side `LookupTypeResource` accessor; the spec-level
type-export lookup is the single path.

### Why this layout was chosen

- One identity per (template, Instantiate) — `*core.TypeResource` pointer
  equality is the canon-spec rule for resource identity.
- One source of truth: spec-level `ExportedType(name)` works the same way
  for host-built and wasm-loaded instances. Witgen-emitted cross-component
  wrappers can rely on a single API.
- Encapsulation: a component that imports another's resource records the
  lender's `*core.TypeResource` at its own slot (via `WithResourceFrom`);
  pointer-equality with the lender's entry is the cross-instance invariant
  that lets canon transfer plans short-circuit.
- The host extern table is independent of this — `ExternHandle`s flow
  into canon's resource table as `rep` values, while `*core.TypeResource`
  identities are looked up by name. The two index spaces never alias.

## Source-file map

| File | Role |
|------|------|
| `internal/core/types.go` | `Type` interface and every variant; `TypeResource`, `InstanceType`, `ComponentType` |
| `internal/core/type_resolvers.go` | `typeResolver` interface, `resolverCtx`, all resolver kinds |
| `internal/core/engine_load_types.go` | Parser payload → resolver mapping (`buildComponentTypeResolver`, `buildDefinedTypeResolver`) |
| `internal/core/engine_load.go` | `componentLoader.allocType`, `loadInstanceInfo` index-space tracking |
| `internal/core/plan.go` | `planResolveType` plan step |
| `internal/core/engine_instantiate.go` | `planResolveType.execute`, `instantiationState.resolveDtor` |
| `internal/core/component_instance.go` | Per-instance `types []Type`, `instances`, `parent`, `exportedType` / `ExportedType` |
| `internal/core/canon_bridge.go` | `canonType` Accept dispatch, `resourceType` / `canonInstanceView` adapters, `FuncType*AsCanon` |
| `host/resource.go`, `host/resource_ref.go` | `componentResourceTypeID`, builder-time ID allocation |
| `host/component.go` | `trSlice` build, `resolveResourceRefs`, `buildPerInstanceWpType` |
