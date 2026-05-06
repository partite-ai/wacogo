# witgen: WIT-to-Go bindings generator

Internal package implementing [`wacogo-witgen`](../../cmd/wacogo-witgen/).
The CLI is a thin wrapper around `Generate(opts)`; the library is also
usable directly for embedding in other tools.

## Pipeline

```
WIT file
   │
   ▼  Load (go.bytecodealliance.org/wit)
   ▼  LowerWorld → IR  (lower.go, ir.go)
   ▼  Validate         (validate.go)
   ▼  For each interface:
   │     emitIfaceFile     → <iface>.go      (clean: types + interfaces)
   │     emitBindFile      → <iface>.bind.go  (factory + marshalling)
   │     emitWrapperFile   → <iface>.wrap.go  (WrapInstance + remoteX)
   ▼  goimports.Process formats every emitted file
   ▼  Write to disk (or return as map for --dry-run)
```

## Files

| File | Responsibility |
|------|---------------|
| `witgen.go` | Top-level `Generate(opts)` API + orchestration |
| `load.go` | WIT loader (wraps `go.bytecodealliance.org/wit`) |
| `lower.go` | wit AST → IR (multi-pass with shared typeDefIR) |
| `ir.go` | Go-side IR types |
| `name.go` | `GoName`, `GoPackageName`, `TypeName`, `GoTypeOf` |
| `abi.go` | Canonical-ABI metadata: `Size`, `Align`, `FlatSlots`, `FieldOffset` |
| `typededup.go` | Per-interface type collection + filters |
| `validate.go` | Pre-emit validation pass |
| `emit.go` | Template + goimports.Process pipeline |
| `emit_iface.go` | Emits `<iface>.go` (types + interfaces) |
| `emit_factory.go` | Emits `<iface>.bind.go` (factory + helpers + wrap_*) |
| `emit_wrapper.go` | Drives `wrapper.tmpl` emission |
| `templates/` | `text/template` files embedded via `go:embed` |
| `templates/wrapper.tmpl` | Emits `<iface>.wrap.go` (consumer-side `WrapInstance` + `remoteX` types) |
| `testdata/wit/*.wit` | WIT input fixtures |
| `testdata/genfixtures/**/*.go` | Generated golden output — disposable; re-run `TestGolden -update` to regenerate |
| `runtime/**/*_test.go` | Hand-written tests that exercise generated bindings via the wasm boundary |

## IR

`ir.go` defines the Go-side IR. `Type` is a sealed interface implemented
by:

- **Primitives** — `Prim` (one type per WIT primitive: `PrimBool`, `PrimU32`, …)
- **Structural compounds** — `TypeString`, `*TypeList`, `*TypeTuple`,
  `*TypeOption`, `*TypeResult`
- **Nominal types** — `*TypeRecord`, `*TypeVariant`, `*TypeEnum`,
  `*TypeFlags`, `*TypeResource` (each carrying `OwnerInterface` and
  living in `iface.X` slices)
- **Resource handles** — `*TypeOwn`, `*TypeBorrow` (thin wrappers around
  `*TypeResource` distinguishing wire-level handle semantics)

Anonymous compounds are deduped per package via `CollectTypes`. Nominal
types live in their owning interface's package (each `Interface` has
`Records`, `Variants`, `Enums`, `Flags`, `Resources` slices).

## Lowering

`lower.go` walks the wit AST in three passes:

1. **Allocate Interface stubs** for every imported interface in the world.
2. **Walk typedefs** in each interface — populates `iface.X` slices and a
   shared `typeDefIR` map (`map[*wit.TypeDef]Type`).
3. **Walk functions** in each interface, classifying via `wfn.Kind`
   (`*wit.Freestanding | *wit.Constructor | *wit.Method | *wit.Static`)
   and attaching to the right interface or resource.

The shared `typeDefIR` ensures that cross-references between types
resolve to the SAME IR pointer (load-bearing for pointer-equality
checks in tests + emission).

## Naming

`name.go` provides:

- `GoName(s, exported bool)` — kebab → Pascal/camel with initialism table
  (`id` → `ID`, `url` → `URL`, etc.).
- `GoPackageName(s)` — kebab → flat lowercase.
- `TypeName(t Type)` — recursive name builder for anonymous types.
  Produces e.g. `OptionListString`, `TupleU32String`, `ResultU32String`.
- `GoTypeOf(t, currentIface)` — Go type expression. Currently always
  unqualified (cross-package qualification deferred to a future plan).

## ABI metadata

`abi.go` provides:

- `Size(t Type) uint32` — bytes occupied in caller memory
- `Align(t Type) uint32` — alignment requirement
- `FlatSlots(t Type) int` — wasm flat-slot count
- `FieldOffset(types []Type, idx int) uint32` — offset of field `idx`
  in a tuple/record's memory layout (C-style packing with per-field
  alignment padding)

## Emission

Two emitters per interface:

- `emit_iface.go` — emits `<iface>.go` with type declarations and the
  user-facing implementer interface. Variant types emit a sealed
  marker interface plus per-case structs. Enum/flags emit typed uint
  with constants. Records emit Go structs with named fields.
- `emit_factory.go` — emits `<iface>.bind.go` with the Factory struct,
  `b.AddType / b.AddResource` calls, `b.AddFunction` calls (using
  canonical bracketed names like `[constructor]counter`,
  `[method]counter.increment`), per-method `wrap_*` trampoline closures,
  and per-type lift/lower helpers.

Per-type helpers come in four flavors: `liftFlat_X`, `liftMem_X`,
`lowerFlat_X`, `lowerMem_X`. Primitives, enums, and flags are inlined
(no helper); everything else gets standalone helper functions.

`emit.go` provides the template + `imports.Process` pipeline, with
per-emit FuncMap closures over the current interface (so qualified type
names work and interface-specific signature builders have context).

## Validation

`validate.go` runs after lowering, before emission. Returns aggregated
errors via `errors.Join` for:

- Go-name collisions (across freestanding funcs + resource ctor/static)
- Reserved Go keywords as parameter names
- `result<_, E>` where E is non-string non-nil (B-3 restriction —
  arbitrary error types deferred to future plan)

## Test fixtures

`testdata/wit/*.wit` are the WIT inputs. Corresponding goldens live at
`testdata/genfixtures/<ns>/<pkg>/<iface>/<iface>.{go,bind.go,wrap.go}` and are
verified by `TestGolden` in `golden_test.go` (re-run with `-update` to
regenerate after intentional changes).

`testdata/genfixtures/` is pure codegen output — it is safe to delete the entire
tree and regenerate with `TestGolden -update`. Do not put hand-written
files there. Placing it under `testdata/` hides it from `go test ./...`
auto-discovery (Go skips `testdata/` directories), keeping the package
list clean.

Hand-written tests live in `fixturetests/<iface>/`. Each package is an external
test package (`package <iface>_test`) and imports the generated bindings from
`testdata/genfixtures/<ns>/<pkg>/<iface>`. The test files wire the generated
bindings into a wasm component via a small WAT importer and exercise
the round-trip end-to-end.

## Adding a new WIT feature

1. **IR** — add types to `ir.go`. Implement `isType()` marker. If
   nominal, add `OwnerInterface` field and slice on `Interface`.
2. **Lowering** — extend `lowerType` / `lowerInterfaceTypeDefs` /
   `lowerInterfaceFunctions` in `lower.go`.
3. **Naming + ABI** — add cases to `TypeName`, `GoTypeOf`, `Size`,
   `Align`, `FlatSlots`, and `walkType`.
4. **Filters** — add `XTypes(types) []*TypeX` filter in `typededup.go`
   if helpful.
5. **Emission** — add cases to all four `emit{Lift,Lower}{Flat,Mem}`
   functions in `emit_factory.go`, and a declaration block in
   `templates/iface.tmpl`.
6. **Validation** — if there are restrictions or known gotchas, add
   checks in `validate.go`.
7. **Test fixture** — add `testdata/wit/<feature>.wit`, regenerate
   goldens, write a runtime test exercising the feature through the
   wasm boundary, and add the case to `TestGolden`'s table.

For long-form design context, see `docs/witgen.md`.

## Cross-package emission

When an interface's signatures reference resource types defined in
another interface, lowering captures this in `Interface.Imports
[]ImportRef`. Each `ImportRef` records the source interface's WIT
identity, its Go package path (computed from `Options.PackageRoot`),
and the resources pulled in.

Emission consumers (`emit_iface.go`, `emit_factory.go`,
`emit_wrapper.go`) use `Imports` to:

- Emit explicit `import "<pkg>"` statements (goimports cannot resolve
  testdata fixtures' external paths reliably)
- Qualify cross-package resource references as `<otherPkg>.<R>` /
  `<otherPkg>.<R>Impl`
- Generate the per-package `Deps` struct with one exported field per
  imported source interface (`Streams *wacogo.ComponentInstance`)
- Emit `b.AddResourceRef("<resource>")` per imported resource at Build
  time, then `host.WithResourceFrom(...)` per resource at Instantiate
  time

Per-interface IR fields supporting this:
- `ImportRef.GoFieldName` — exported PascalCase field name on the
  importer's `Deps` struct (e.g., `"Streams"`)
- `ImportRef.StateField` — unexported field name on `instanceState`
  (e.g., `"streams"`)
- `TypeResource.ImportRefField` — set on per-iface copies of imported
  resources by the `rewriteImportedResources` post-pass
- `TypeResource.GoPackageQualifier` — package alias used in emitted
  Go code
- `TypeResource.LenderInstField` — instanceState field name for the
  bound dep (e.g., `"streams"`), derived from `ImportRef.StateField`
