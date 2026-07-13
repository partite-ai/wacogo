# Instantiation Import Checking

How `(*core.Component).CheckInstantiation` and `Instantiate` validate
host-supplied imports against a component's declarations before any plan step
executes.

Spec reference:
[Subtyping.md](https://github.com/WebAssembly/component-model/blob/main/design/mvp/Subtyping.md)
and the `(instantiate ...)` rules in
[Explainer.md](https://github.com/WebAssembly/component-model/blob/main/design/mvp/Explainer.md).

## Why this exists

Without import checking, mismatched imports surface late: a wrong-arity func
import would not fail until lift; an unsatisfied `(eq $r)` resource
constraint would not fail at all (the resource handles would silently flow
through with mismatched identities and the bug would manifest as a trap or
data corruption inside a callee). The wasm-tools spec suite has several
`assert_unlinkable` cases that exercise exactly these failures (e.g.
`instance.wast` lines 287/294/301/308/315/322 — six `mismatched resource
types`). Import checking folds all of them into one authoritative helper that
runs before any imports are wired.

The helper composes three distinct checks:

1. every declared import has a non-nil provider;
2. `validateImportKind` confirms that the provider's Go type matches the
   declared component-model sort;
3. providers carrying wasmparser type handles are checked for structural and
   resource-identity compatibility.

The public `CheckInstantiation` method and top-level `Instantiate` call this
same helper, so their import validation cannot drift.

## Where it runs

There are three relevant points in the lifecycle:

- **Load** (`internal/core/engine_load.go`). Validating parser walks the
  component binary; the validator (`*wasmparser.Validator`, owned by the
  Engine — see `internal/core/engine.go`) materializes a
  `*wasmparser.ComponentType` for the outer component and stashes it on
  `*Component.wpType`. Inline core modules and component
  imports/exports come out of the parser as
  `*wasmparser.ValidatedModuleSectionPayload` /
  `Validated{Component,Import}SectionPayload` items, each carrying an
  opaque type handle (`*wasmparser.FuncType`, `*ModuleType`,
  `*ComponentType`, `*InstanceType`). Those handles ride along on
  `ExportDesc.ParserFunctionType`, `CompiledModule.wpModuleType`,
  `*Component.wpType`, and (after Instantiate)
  `*ComponentInstance.wpInstance`. No subtype check fires at load
  time.

- **Check imports** (`internal/core/engine_instantiate.go`,
  `(*Component).checkInstantiationImports`). The helper checks required
  presence and Go runtime kind, then type-switches providers with parser
  metadata into a `map[string]any` for
  `(*wasmparser.ComponentType).CheckInstantiation`.

- **Public check or execution** (`internal/core/component.go` and
  `internal/core/engine_instantiate.go`). `Component.CheckInstantiation`
  applies its options and returns the helper's result without executing the
  component. Top-level `Instantiate` first rejects a closed Engine, applies
  the same options and helper, and only then starts the instantiation plan.

The split is deliberate. Load produces type handles but never compares
them; one component can be loaded and instantiated against many different
provider sets. Import options provide the concrete argument map needed for
subtype checking, independently of whether the caller then executes the
component.

## What `CheckInstantiation` does not guarantee

`CheckInstantiation` is deliberately an import-metadata check, not a dry run
of the complete instantiation process:

- A loaded `Component` has no independent `Close` operation. The check reads
  its immutable metadata and does not inspect whether its owning Engine is
  still open. It likewise does not inspect whether supplied Components or
  ComponentInstances are closed, poisoned, or otherwise usable.
- A provider without a wasmparser type handle receives presence and runtime-
  kind checks only. Host and synthetic providers may fall into this category.
- Type and value imports have no parser-side structural handles today, so only
  their presence and runtime kind are checked.
- The check does not execute the instantiation plan, allocate runtime state,
  resolve external resources, or run a core start function. Any of those steps
  can still make `Instantiate` fail.

Consequently, a nil result means only that the checks above passed; it is not a
promise that a subsequent `Instantiate` call will succeed.

## Subtype rules per import kind

The wasmparser-side subtype dispatch lives in
`(*ComponentType).CheckInstantiation` in `wasmparser/public_instantiate.go`.
For each declared import (in `compType.ImportOrder`) the checker type-switches
supplied parser handles. The wasmparser API itself silently skips missing
arguments because it is also useful to lower-level callers; the core helper
enforces required-import presence before invoking it.

### Func imports — `*wasmparser.FuncType`

Routes through `SubtypeChecker.isFuncTypeSubtype`. Structural by
parameter and result types: arity must match exactly; each param/result
type pair is checked recursively (records, variants, lists, options,
results all decompose). Resource types appearing inside func signatures
are matched by the checker's a-side/b-side `ResourceID` mapping (see
"Resource identity" below).

A func arg with the wrong structural type fails with
`import %q: expected function found function` plus an inner explanation
from the recursive checker. A `*ExportedFunc` carrying no handle (e.g.
host-supplied) is silently skipped at the engine layer
(`engine_instantiate.go`); the Go-level kind check still runs.

### Module imports — `*wasmparser.ModuleType`

Routes through `isModuleSubtype`. Core modules use the wasm 1.0 import/
export structural subtype: every import the *callee* declares must have a
compatible matching declaration in the supplied module's imports, and
every export the consumer expects must appear in the supplied module's
exports with a compatible type. Function signatures are compared by core
wasm value-type lists; memory/table limits use the wasm "is-a"
limits-subtype rule (provided min ≥ required min, provided max ≤
required max where present).

Host-built modules (adapters, bridges, blank-import stubs) carry no
handle and silently skip — they are never reached as imports anyway, so
this is a non-issue in practice.

### Component imports — `*wasmparser.ComponentType`

Routes through `isComponentSubtype`. A component is a subtype of another
if its import set is a *supertype* (contravariance) and its export set is
a *subtype* (covariance) of the target's. Resources cross both sides;
the checker remaps `ResourceID`s along the way so a single concrete
resource can satisfy multiple structurally-distinct mentions.

### Instance imports — `*wasmparser.InstanceType`

This is the load-bearing case for the real-world spec failures. Routes
through `isInstanceSubtypeAgainstExports`, which compares the *source
component's* exports against the *import's* expected exports — no
synthetic instance type is materialized.

For each export the import declares:

- Missing on the source → `export `%s` was not found`, *unless* the
  declared kind is `EntityType` (purely structural type re-exports may be
  omitted by the host since their identity is fully determined by the
  consumer).
- Present → recursive `isSubtype`. For type exports of resource kind,
  the source's `ResourceID` is recorded as "providing" the import's
  abstract `ResourceID`.

After the export loop, every `DefinedResources` entry on the import side
must have been pinned by at least one provided export referring to it.
If not, the missing export is reported by name (the same
`export `%s` was not found` phrasing the spec suite checks for).

### Value and type imports

Out of scope for parser-level structural checking. The shared core helper
still verifies presence and Go-level kind via `validateImportKind`, but no
structural subtype check runs. A future change could add handles for these once
a concrete spec case demands it.

## Resource identity and `(eq $r)` constraints

This is the rule that makes the cross-arena subtype check work and that
the spec's `mismatched resource types` cases hinge on.

`SubtypeChecker` in `wasmparser/validator_subtype.go` carries a
`resourceMapping` keyed by the source-side `ResourceID` and scoped per
`*InstanceType` pointer. Two consumer-side resource references are
required to match identity (e.g. via an `(eq $r)` `(sub resource)`
chain) iff they map to the same source-side `ResourceID` under the same
scope.

This means:

- Supplying *the same* `*InstanceType` for two instance imports that
  share an `(eq $r)` constraint resolves both to the same `ResourceID`.
  Check passes.
- Supplying *two distinct* `*InstanceType` values (each minted by a
  separate `NewInstance` call on the same `*ComponentType`) gives each
  side an independent identity scope. The `(eq $r)` mapping cannot
  unify them, and the check fails with `mismatched resource types`.

Pointer identity, not arena equality, controls scoping. An
`*InstanceType` is a Go pointer wrapper around its source `*ComponentType`
(see `wasmparser/public_instantiate.go`), and `(*ComponentType).NewInstance`
always returns a fresh allocation. Each top-level `Instantiate` mints
exactly one new `*InstanceType` for the resulting `*ComponentInstance`
(stashed on `wpInstance`), which is what gets passed back in when the
instance is later supplied as an arg.

## Error model

Parser errors flow up through wasmparser's `CheckInstantiation`, wrapped with
the import name (`import %q: <inner>`). The core helper retains the existing
`wacogo: instantiate:` prefix for those errors. Missing-provider and runtime-
kind errors originate in the core helper. Phrasing for the spec-bearing
failure modes is fixed:

- `mismatched resource types` — a resource-equality constraint can't be
  satisfied (emitted from `checkResourceMatch`).
- `export `%s` was not found` — instance-export gap, including the
  resource-pinning case above.
- `expected %s found %s` — kind mismatch at the top level (e.g. a
  `*FuncType` supplied where the import declares an instance).

These strings are matched verbatim by the wasm-tools spec test harness;
do not paraphrase without checking the corresponding `assert_unlinkable`
fixture.

## Runtime defense in depth

`validateImportKind` requires a `*ExportedFunc` for `SortFunc`, a
`*CompiledModule` for `SortCoreModule`, and so on. The shared helper runs it
before parser subtyping, which catches misuse such as supplying
`WithFuncImport(name, fn)` for a declared instance import even when no parser
handle is available.

`instantiateWithParentCfg` retains its own runtime-kind guard for internal
nested-component instantiation paths. Anything that cannot be decided from
metadata still surfaces later as a plan-step or start-function error.
