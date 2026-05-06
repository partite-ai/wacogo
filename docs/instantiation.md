# Instantiation Type Checking

How `(*core.Component).Instantiate` validates host-supplied imports against
the component's declared import types before any plan step executes.

Spec reference:
[Subtyping.md](https://github.com/WebAssembly/component-model/blob/main/design/mvp/Subtyping.md)
and the `(instantiate ...)` rules in
[Explainer.md](https://github.com/WebAssembly/component-model/blob/main/design/mvp/Explainer.md).

## Why this exists

Without the pre-check, mismatched imports surface late: a wrong-arity func
import would not fail until lift; an unsatisfied `(eq $r)` resource
constraint would not fail at all (the resource handles would silently flow
through with mismatched identities and the bug would manifest as a trap or
data corruption inside a callee). The wasm-tools spec suite has several
`assert_unlinkable` cases that exercise exactly these failures (e.g.
`instance.wast` lines 287/294/301/308/315/322 — six `mismatched resource
types`). The pre-check folds all of them into one authoritative entry
point that runs before any imports are wired.

The check also keeps the runtime kind check in `validateImportKind`
(`internal/core/engine_instantiate.go`) honest: that function only knows
the Go type of a supplied arg, not its structural type. The pre-check
enforces the structural rules; the runtime kind check stays as a
last-ditch guard for host-supplied args that have no wasmparser handle.

## Where it runs

Two layers, with a hard split:

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

- **Instantiate** (`internal/core/engine_instantiate.go`,
  `(*Engine).instantiate`). Right after option parsing and before the
  plan loop, `cfg.imports` is type-switched into a
  `map[string]any` of handle values and handed to
  `(*wasmparser.ComponentType).CheckInstantiation`. On error the
  Instantiate call returns with a `wacogo: instantiate:` prefix — no
  plan step runs, no core module is instantiated.

The split is deliberate. Load produces type handles but never compares
them; one component can be loaded and instantiated against many different
provider sets. Instantiate is the only place a complete arg map exists,
and it is the only place subtype checking is meaningful.

## Subtype rules per import kind

The wasmparser-side dispatch lives in
`(*ComponentType).CheckInstantiation` in `wasmparser/public_instantiate.go`.
For each declared import (in `compType.ImportOrder`) the checker either
accepts a missing arg (silent skip — instance imports without a handle
are picked up by the runtime path) or type-switches the arg.

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

Out of scope for `CheckInstantiation`. The runtime path
(`validateImportKind`) verifies the Go-level kind for type imports, but
no structural check runs. A future change could add handles for these
once a concrete spec case demands it.

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

Errors flow up through `CheckInstantiation` wrapped with the import name
(`import %q: <inner>`). The engine wrapper adds `wacogo: instantiate:`
on top. Phrasing for the spec-bearing failure modes is fixed:

- `mismatched resource types` — a resource-equality constraint can't be
  satisfied (emitted from `checkResourceMatch`).
- `export `%s` was not found` — instance-export gap, including the
  resource-pinning case above.
- `expected %s found %s` — kind mismatch at the top level (e.g. a
  `*FuncType` supplied where the import declares an instance).

These strings are matched verbatim by the wasm-tools spec test harness;
do not paraphrase without checking the corresponding `assert_unlinkable`
fixture.

## What the runtime path still does

`validateImportKind` (in `engine_instantiate.go`) runs unchanged for
every import after the pre-check. Its job is the Go-level kind check:
the supplied `any` must be a `*ExportedFunc` for `SortFunc`, a
`*CompiledModule` for `SortCoreModule`, etc. This catches the case where
the host calls `WithFuncImport(name, fn)` for an import declared as an
instance — a bug `CheckInstantiation` cannot see, because that arg never
makes it into the `map[string]any` it receives (the type switch in
`(*Engine).instantiate` only forwards args whose Go type is recognized).

Together: the pre-check catches structural mismatches between
component-sourced args and the consumer's declared types; the runtime
kind check catches misuse of the `WithXImport` options. Anything that
slips past both surfaces later as a plan-step error from
`planImport*.execute`, which formats the same `expected X found Y`
messages.
