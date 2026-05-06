# Code Overview

A navigation map of the wacogo source tree. Use this together with
[`CONTRIBUTING.md`](../CONTRIBUTING.md) (top-level package layout) and
the per-subsystem docs in this directory.

The root `wacogo` package is intentionally tiny — five files of
trampolines and Go type aliases over `internal/core`. All implementation
lives below.

```
wacogo/
├── engine.go       Engine{ core *core.Engine }; LoadComponent, NewHostBuilder
├── types.go        type-aliases for core.* (Component, ComponentInstance,
│                   Type*, ExportedFunc, Sort, FuncType, ...)
├── val.go          type-aliases for core.Val* + constructor trampolines
├── options.go      WithFuncImport / WithInstanceImport / ...
└── val_test.go
```

## internal/core

The runtime layer: load a binary, build a `*Component`, instantiate it,
call its exports. Carries no host-specific state.

| File | What it owns |
|------|-------------|
| `engine.go` | `Engine` (wazero runtime + canon `Host` + `wasmparser.Validator`). Package-level `WazeroRuntime(e)` / `Validator(e)` accessors. |
| `engine_load.go` | Load pipeline: parser walk → plan-step emission → `*Component`. |
| `engine_load_types.go` | Resolver dispatch: parser type payloads → `typeResolver` values. |
| `engine_instantiate.go` | `(*Engine).instantiate` plus every `planXxx.execute` body. |
| `component.go` | `Component`, `CompiledModule`, `ExportDesc` / `ImportDesc`, `InstantiateOption`, all `WithXImport` constructors. |
| `component_instance.go` | `ComponentInstance` (live), `Enter` / `CanLeave` / `SuspendLeave`, `ExportedFunc` / `ExportedInstance` / `ExportedType` / etc. |
| `instance.go` | `NewInstance(*InstanceSpec)` — the shared constructor used by both the load path and the host package's `Instantiate`. |
| `func.go` | `Func` (Component-level callable; just a `*FuncType` plus a `*canon.CallBinding`), `ExportedFunc`. |
| `plan.go` | All `planXxx` step structs and their interface, plus `Sort` enum and `canonicalOptions`. |
| `types.go` | The `Type` interface and every concrete variant (`TypeBool` … `TypeResource`, `*FuncType`, placeholder `*InstanceType` / `*ComponentType`). |
| `type_resolvers.go` | `typeResolver` interface, `resolverCtx`, every concrete resolver. See [`type_system.md`](type_system.md). |
| `val.go` | Re-exports `canon.Val*` so external callers see them as `core.Val*`. |
| `canon_bridge.go` | `canonType.Accept(canon.TypeVisitor)` dispatch, `resourceType` adapter, `FuncType{Params,Results}AsCanon`, `canonInstanceView`. |
| `adapter.go` | `execLowerWithGoAdapter` plus `planResource{New,Drop,Rep}.execute` — the four call-sites that build canon adapters. |
| `call_context.go` | `CallContext` (canon-level per-call accessor: instance, task, memory, realloc) plus `IssueOwnHandle`/`LookupOwn`/`TransferTarget` helpers. |
| `resource_table.go` | `ResourceTable` (1-indexed, kind-discriminated, free-list reuse), `TransferTarget` interface. |
| `resource_handle.go` | `ResourceHandle` (live entry view) plus `LendTo` / `TransferOwn` and the `liveResourceHandle` lifecycle. |
| `extern_table.go` | `ExternTable` interface (the host wrapper supplies the implementation). |

The split between core's `ResourceTable` and canon's `Instance.ResourceTable()`
abstraction is deliberate: canon never sees `*ComponentInstance`, only an
opaque `Instance` whose `ResourceTable()` it can drive. The canon bridge
exposes `canonInstanceView` to satisfy `canon.Instance` without leaking
core types.

## internal/canon — Canonical ABI

The visitor-driven lift / lower / transfer pipeline. Read
[`canonical_abi.md`](canonical_abi.md) for the design rationale; the
files below are the surface a reader navigates.

| File | What it owns |
|------|-------------|
| `host.go` | `Host`, `NewHost(rt)` — the engine's single canon entry point. |
| `build_adapter.go` | `(*Host).BuildAdapter` — produces a `*CallAdapter` for a cross-component call. |
| `build_resource.go` | `(*Host).BuildResource{New,Drop,Rep}` — wasm core funcs for canon resource intrinsics. |
| `call_binding.go` | `CallBinding` — Go→component call binding (`NewCallBinding`, `Call`, `CallRaw`, `Callee`). |
| `call_adapter.go` | `CallAdapter{Module, Name, ...}` and idempotent `Close`. |
| `call_side.go` | `CallSide` and `Callee` plain-value descriptors. |
| `instance.go` | `Instance` interface (`Enter` / `CanLeave` / `SuspendLeave` / `ResourceTable`). |
| `resource_iface.go` | `ResourceTable` interface and `ResourceType` interface. |
| `resource.go` / `resource_handle_type_test.go` | Internal resource-handle plumbing used by visitors. |
| `visitor.go` | `Type` and `TypeVisitor` interfaces. |
| `visitor_transfer_flat.go`, `visitor_transfer_mem.go` | Component→component step emission. |
| `visitor_val_to_flat.go`, `visitor_val_to_mem.go` | Go→wasm lowering step emission. |
| `visitor_flat_to_val.go`, `visitor_mem_to_val.go` | Wasm→Go lifting step emission. |
| `plan.go` | `transferPlan`, `gocallPlan`, `compileTransferPlan`, `compileGocallPlan`, `pickFlat`, the three step closure types. |
| `context.go` | `transferSide`, `transferContext`, `gocallContext`. |
| `run_transfer.go` | `runTransferPlan` (host-callback runner for cross-component calls). |
| `run_gocall.go` | `runGocallPlan` (Go→component runner). |
| `flatten.go`, `flat_count.go` | Type-level flat-slot counts. |
| `transfer_content.go` | Helpers shared between flat and mem transfer visitors. |
| `strings.go`, `validate.go`, `helpers.go` | Encoding helpers, validation traps, alignment math. |
| `stub.go` | Tiny wasm-stub bytecode that the adapter / resource builders import-and-re-export through. |
| `val.go` | The concrete `Val*` types (`ValBool` … `ValOwnHandle`). Re-exported by core and root. |
| `types.go` | Canon-internal type structs used by visitors and tests. |
| `task_test.go` (and `Task` in `validate.go`) | Per-call accounting record (borrow tracking + post-call release closures). |

## host

Layered on top of `internal/core`. See [`host_layer.md`](host_layer.md)
for the full design.

| File | Role |
|------|------|
| `builder.go` | `Builder` (mutable accumulator), `NewBuilder`, `Build`. |
| `component.go` | `Component` (template), `Instantiate`. |
| `component_instance.go` | `*ComponentInstance` wrapper (extern table, type-resource slots, user state, lifecycle hooks). |
| `instance_builder.go` | Builds the underlying `*core.ComponentInstance` from declarations + per-instance state. |
| `scope.go` | Build-time scope tracking. |
| `extern_table.go` | `ExternHandle` (distinct uint32) + private `externTable`. |
| `host_module.go` | Per-instance wazero host module wrapping declared `host.Func`s. |
| `stub.go` | Real-wasm stub module: exported wrappers + bump-reset `realloc` + `memory`. |
| `translate.go` | `TypeExpr` → wasmparser arena entry + `core.Type`. |
| `types.go` | `TypeExpr` union + primitives + `TypeRef`. Re-exports `host.CallContext = core.CallContext`. |
| `resource.go` / `resource_ref.go` | `ResourceType`, `ResourceDtor`, `ResourceTypeRef`. |
| `func.go` | `host.Func` callback signature. |
| `exported_func.go` | `*ExportedFunc` consumer-side wrapper around `*core.ExportedFunc`. |
| `options.go` | `WithUserState`, `WithResourceFrom`. |
| `doc.go` | Package doc. |

## internal/witgen

WIT → Go bindings generator. See [`witgen.md`](witgen.md).

| File | Role |
|------|------|
| `witgen.go` | `Generate(opts)` library entry; orchestrates load + lower + emit. |
| `load.go` | WIT-file → `wit.Resolve` loader. |
| `lower.go` | Resolve → IR; populates `Interface.Records / Variants / ... / Imports`. |
| `ir.go` | Sealed `Type` IR plus `Interface`, `Function`, `ImportRef`, `ImportedResource`. |
| `name.go` | Naming conventions: `GoName` (with initialism table), `GoPackageName`, `TypeName`, `GoTypeOf`. |
| `abi.go` | Pure `Size` / `Align` / `FlatSlots` / `FieldOffset` over the IR. |
| `typededup.go` | `HelperTypes` / `RecordTypes` / etc. type-collection filters. |
| `validate.go` | Pre-emit checks (collisions, keyword params, `result<_, E>` shape). |
| `emit.go` | `text/template` driver + `imports.Process` post-pass. |
| `emit_iface.go` | `<iface>.go` emission (declarations + handle state machine). |
| `emit_factory.go` | `<iface>.bind.go` emission (host trampolines + `to/fromGo*`). |
| `emit_wrapper.go` | `<iface>.wrap.go` emission (consumer wrapper + `lift/lower*`). |
| `emit_doc.go` | Comment / docstring emission helpers. |
| `emit_errsink.go` | Statement-level error-return shaping shared by both helper families. |
| `templates/*.tmpl` | `go:embed` template skeletons. |

## wasmparser

Streaming parser + validator for component-model binaries. Independent
of the runtime. See `wasmparser/README.md` for design notes; the runtime
consumes it via `Validator(e)` (held on `*core.Engine`) and the
`*ValidatedXSectionPayload` items it yields.

## Other top-level directories

- `wasi/` — pre-built `wasi:cli` and `wasi:http` host components.
- `wasmtools/` — wasip1 build of `wasm-tools` runnable through wazero.
- `cmd/wacogo-witgen/` — CLI wrapper around `internal/witgen.Generate`.
- `examples/` — runnable demos (`calculator`, `host-imports`, `wasi-greet`, `jsinterp`).
- `sample-components/` — committed `.wasm` fixtures used by tests.

## Cross-package call paths

### Loading a component

```
Engine.LoadComponent
└── core.(*Engine).LoadComponent (engine.go)
    └── core.componentLoader.run (engine_load.go)
        ├── ParseAll → wasmparser.ValidatedXSectionPayload
        ├── allocType(typeResolver)        ─► typeResolvers + planResolveType
        ├── canonicalOptions / lift / lower─► planLift / planLower
        └── …                                planAlias / planImport* / etc.
    => *Component { plan, typeResolvers, ... }
```

### Instantiating

```
Component.Instantiate
└── core.(*Engine).instantiate (engine_instantiate.go)
    ├── CheckInstantiation against cfg.imports     (instantiation.md)
    ├── for each step in plan: step.execute(ctx, s)
    │   ├── planInstantiateModule.execute  ─► coreInstances
    │   ├── planResolveType.execute        ─► inst.types[typeID]
    │   ├── planLift / planLower           ─► inst.funcs / coreFuncs
    │   ├── planResource{New,Drop,Rep}     ─► canon.Host.BuildResource*
    │   └── …
    └── core.NewInstance(spec)             — for sub-components and host instances
    => *ComponentInstance
```

### Calling an exported function

```
ExportedFunc.Call(ctx, args...)
└── core.Func.Call
    └── canon.CallBinding.Call
        └── canon.runGocallPlan          (run_gocall.go)
            ├── Enter callee                (canon.Instance.Enter)
            ├── SuspendLeave + lower args   (gocall lower steps)
            ├── calleeFn.Call               (wazero core invocation)
            ├── lift results                (gocall lift steps)
            ├── postReturn (optional)
            └── Task.End                    (drain release closures, surface borrow trap)
```

### Cross-component call (lowered import)

```
caller wasm → planLower.execute → adapter core func
                 │
                 ▼
       canon.runTransferPlan (run_transfer.go)
       ├── Enter callee
       ├── SuspendLeave + run paramSteps  (caller mem → callee mem)
       ├── calleeFn.Call
       ├── swap caller/callee
       ├── SuspendLeave + run resultSteps (callee mem → caller mem)
       ├── postReturn (optional)
       └── Task.End
```

## Where to read further

- [`canonical_abi.md`](canonical_abi.md) — visitor design, plan/runner shapes, base-pointer passing.
- [`type_system.md`](type_system.md) — `Type` universe, resolvers, canon bridge.
- [`instantiation.md`](instantiation.md) — pre-instantiate subtype check.
- [`host_layer.md`](host_layer.md) — `host` package internals.
- [`witgen.md`](witgen.md) — WIT bindings generator design.
- `CLAUDE.md` (project root) — top-level architectural conventions.
