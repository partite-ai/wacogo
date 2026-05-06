# Canonical ABI

The `internal/canon/` package implements lifting, lowering, and
component-to-component transfer for the WebAssembly Component Model
canonical ABI ([spec][CanonicalABI.md]). It has one entry shape — a
`Type` tree consumed by visitors that emit pre-bound step closures —
and one execution shape — a small runner that drives those closures
against per-call context state.

The design optimises for two things that are in tension under a naive
implementation: **closed-form per-call cost** (no map lookups, no type
walks, no Go-heap materialisation of intermediate `Val`s on the
component-to-component path) and **shared layout logic** between four
distinct call directions. The visitor split below is what reconciles
them.

[CanonicalABI.md]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md

## Why visitors and closures

The canonical ABI is parameterised over four orthogonal axes:

1. **Direction.** Component→component (transfer), or Go↔component
   (gocall in either direction).
2. **Mode.** Flat (values in core registers) or mem (values in linear
   memory at a base + offset). Selected per-side per-call by the
   `maxFlatParams` / `maxFlatResults` caps in `plan.go`.
3. **Composite shape.** Records, tuples, variants, options, results,
   enums, lists; each composes children differently.
4. **Per-leaf canonicalisation.** Bool→0/1, char range check, sign
   extension, flags label-count cap, etc.

A naive lift/lower pair multiplied across (a) and (b) yields six
direction-mode pairs; multiplied again across all leaf and composite
shapes that's the ~60 step structs the prior design carried in
`steps.go`, plus a `Source`/`Dest` abstraction that every step had to
type-assert through to find its real backing store. The visitor
redesign collapses all of this:

- **One `Type` interface** (`internal/canon/visitor.go`) with a single
  `Accept(TypeVisitor)` method per type. The bridge in
  `internal/core/canon_bridge.go` wraps a core `Type` to satisfy this
  interface — no parallel hierarchy.
- **One `TypeVisitor` interface** with one method per ABI shape
  (`VisitU8`, `VisitString`, `VisitList(elem Type)`, `VisitRecord`,
  `VisitVariant`, `VisitOwn`, `VisitBorrow`, plus first-class
  `VisitOption` / `VisitResult` / `VisitTuple` / `VisitEnum` for
  sugar shapes whose layout matches a variant/record but whose Go
  `Val` does not).
- **Six concrete visitors** (one per direction-mode pair) each
  implementing `TypeVisitor` and emitting closures into an `out`
  slice as they walk.

Sugar types stay distinct from their layout-equivalent composites so
gocall visitors can read and write the parent package's natural `Val`
shapes (`*ValOption`, `*ValResult`, `*ValEnum`, positional `*ValRecord`
for tuples) without a shim layer that boxes them as
`*ValVariant` and unboxes them downstream. Transfer visitors route
sugar shapes to a shared internal variant/record helper because the
byte layout is identical.

## The six visitors

| Visitor                | Source → Destination     | Step type            | File                                      |
|------------------------|--------------------------|----------------------|-------------------------------------------|
| `flatTransferVisitor`  | registers → registers    | `transferPlanStep`   | `internal/canon/visitor_transfer_flat.go` |
| `memTransferVisitor`   | memory → memory          | `transferPlanStep`   | `internal/canon/visitor_transfer_mem.go`  |
| `valToFlatVisitor`     | `Val` → registers        | `gocallLowerStep`    | `internal/canon/visitor_val_to_flat.go`   |
| `valToMemVisitor`      | `Val` → memory           | `gocallLowerStep`    | `internal/canon/visitor_val_to_mem.go`    |
| `flatToValVisitor`     | registers → `Val`        | `gocallLiftStep`     | `internal/canon/visitor_flat_to_val.go`   |
| `memToValVisitor`      | memory → `Val`           | `gocallLiftStep`     | `internal/canon/visitor_mem_to_val.go`    |

Each visitor is **stateful**: it tracks its own layout cursor (an
absolute slot index for flat visitors, a byte offset and running
`maxAlign` for mem visitors) and accumulates emitted closures into
`out`. There is no external `LayoutWalker` — bookkeeping lives next
to the emit. Every call to `Accept(visitor)` produces exactly one
top-level step appended to `out`; composites compile their children
into sub-step slices captured inside one outer closure.

### Visitor switching at composite boundaries

A composite's outer mode does not always propagate to its children.
The canonical case is lists: list elements are always mem-resident
regardless of where the list header (`ptr, len`) lives. So
`flatTransferVisitor.VisitList` emits the header step against its own
flat slots, but instantiates a fresh `memTransferVisitor` for the
element type, drives `elem.Accept(memVisitor)`, and captures
`memVisitor.out` inside the list outer closure. The same pattern
applies on the gocall side (flat visitors construct a sibling mem
visitor for list elements). The six-visitor family is closed under
this switching: every cross-domain transition lands on one of the
other five visitors, and a plan never mixes the transfer and gocall
families.

Variant payloads in mem mode stay in mem (same visitor family,
re-seated at the payload origin). Variant payloads in flat mode stay
in flat — sub-step slot indices are baked at compile time as
`payloadStart + localSlot`, so the sub-plan writes straight into the
outer register array. Records and tuples share their parent's mode
unconditionally.

## Plans

A `transferPlan` (`internal/canon/plan.go`) is a compiled recipe for
a component→component call:

```go
type transferPlan struct {
    paramMemSize   uint32 // 0 in flat params mode
    paramMaxAlign  uint32 // 0 in flat params mode
    returnMem      bool   // callee returns a single i32 result-block ptr
    resultMaxAlign uint32 // 0 when !returnMem
    paramSteps     []transferPlanStep
    resultSteps    []transferPlanStep
}
```

A `gocallPlan` is the recipe for a Go→component call:

```go
type gocallPlan struct {
    paramMemSize  uint32
    paramMaxAlign uint32
    nParamRegs    int // flat slot count, or 1 for mem params
    nResultRegs   int // flat slot count, or 1 for mem results
    paramSteps    []gocallLowerStep
    resultSteps   []gocallLiftStep
}
```

`compileTransferPlan` and `compileGocallPlan` (both in `plan.go`) are
the only entry points. They:

1. Call `pickFlat(types, maxFlatParams|maxFlatResults)` to choose mode
   per side. `pickFlat` runs a `flatCountVisitor` (a tiny
   side-effect-free counter) and compares the total against the cap.
2. Instantiate the correct visitor of the chosen mode-direction pair.
3. Walk each top-level type via `Accept`. The visitor advances its
   layout cursor and appends one step.
4. For mem mode, freeze `paramMemSize = alignUp(byteOff, maxAlign)`
   and `paramMaxAlign = maxAlign` from the visitor.
5. Repeat for results, then return the assembled plan.

Param and result modes are selected **independently**. A function with
16 flat params and mem-mode results has caller core signature
`(i32 × 16, retptr) → ()`; the return pointer does not feed back into
param-mode selection.

There is no separate sub-plan struct. List element bodies, variant
case bodies, and option/result Some/Ok/Err arms are plain `[]step`
slices captured in the parent's outer closure.

### Why three step closure types

`plan.go` defines three signatures rather than one:

```go
type transferPlanStep func(ctx context.Context, tc *transferContext, srcBase, dstBase uint32)
type gocallLowerStep  func(ctx context.Context, gcc *gocallContext, v Val, base uint32) error
type gocallLiftStep   func(ctx context.Context, gcc *gocallContext, base uint32) (Val, error)
```

The split falls out of two facts:

- **Transfer has two memories**, one source and one destination, and
  needs a base for each. Gocall has only one (Go itself has no linear
  memory), so a single `base` suffices — interpreted as `memDst` for
  lower steps, `memSrc` for lift steps.
- **Gocall steps return `error`; transfer steps panic `*Trap`.**
  Transfer plans run inside wazero host callbacks where `*Trap`
  panics surface as module traps; threading an `error` back through
  every closure would just immediately wrap and re-panic at the top.
  Gocall plans run on the Go side where `error` is the natural
  signal. Shared validation helpers in `internal/canon/validate.go`
  return `error`; transfer call sites use `mustTransfer(err)` which
  wraps the message in `panic(&Trap{...})`.

Lift and lower also have asymmetric data flow. A lower step takes its
input `Val` directly as a parameter so the runner can pair `args[i]`
with `paramSteps[i]` without parking input through context state. A
lift step returns the lifted `Val`, so the runner collects each
return into the output `[]Val`. Neither shape needs a `Val` field on
`gocallContext`.

## Contexts

`internal/canon/context.go`:

```go
type transferContext struct {
    caller    *transferSide
    callee    *transferSide
    registers []uint64 // wazero host-callback stack
    Task      Task     // borrow accounting + per-call release closures
}

type gocallContext struct {
    callee    *transferSide
    registers []uint64 // core args/results slice
    Task      Task
}
```

`transferSide` packages everything the visitor closures need from one
end of a call: `Instance`, `Memory`, `Realloc`, `StringEncoding`, and
`ResourceTable`. Reentrance gating (`may_enter`) and the may_leave flag
live on `Instance` — runners call `Instance.Enter(ctx)` to acquire the
lock and `Instance.SuspendLeave()` to gate the lowering windows.

The contexts carry only state that is **shared across step closures
within a single call** and **must not be captured at compile time**.
Sides, register stacks, and the shared `Task` satisfy both. Input args
and lifted results do not — they are call-local, flow as runner
parameters and return values, and never enter the context.

Borrow accounting and unlend release closures live on `Task`. A
`LendTo` step registers an unlend closure via `Task.AddRelease`; the
runner drains them on `Task.End`, which also returns an error if any
callee-side borrows remain live.

## Base pointer passing

Earlier iterations carried `memSrcBase` / `memDstBase` as mutable
fields on the contexts. Composite steps that walked into a list
element or a variant payload had to save the field, overwrite it,
recurse, and restore — which leaked composite-local layout decisions
into a process-wide piece of state. A trap inside the recurse left
the field clobbered.

Today, base pointers are step-closure **arguments**:

- `transferPlanStep` takes `(srcBase, dstBase uint32)` — one byte
  address per side.
- `gocallLowerStep` and `gocallLiftStep` take `(base uint32)` — a
  single address since Go has no source/destination memory.

This matches the spec's `load(cx, ptr)` / `store(cx, v, ptr)` shape
directly. Flat-mode steps ignore their base arguments entirely; their
slot indices are absolute compile-time positions in
`tc.registers` / `gcc.registers`. Mem-mode steps add their captured
compile-time offset to the base they receive. Composite steps
(`VisitList`, `VisitVariant`, `VisitOption`, `VisitResult`) compute
the child base — `dstPtr + i*elemSize` for list elements,
`base + discOff + memPayloadOffset` for variant payloads — and pass
it directly into the sub-step loop. No save, no restore.

The runners compute the top-level bases locally. For
`runTransferPlan`: param bases come from the caller's pre-pushed
param-block pointer (`tc.registers[0]`) and the callee block freshly
allocated via `Realloc`; result bases come from the callee's returned
pointer and the caller's pre-allocated retptr slot. For
`runGocallPlan`: param `base` is the callee block (or 0 in flat
mode); result `base` is `uint32(coreResults[0])` (used only by
mem-mode result steps).

## Runners

Two functions own the full per-call pipeline.

### `runTransferPlan` (`run_transfer.go`)

1. **Enter callee.** `tc.callee.Instance.Enter(ctx)` returns an exit
   closure captured before the side swap below, so the original
   callee's reentrance lock is always released.
2. **Defer `Task.End`.** Drains caller-side unlend release closures
   and traps if any callee-side borrows are still live (via
   `trapf`). Registered after the `Exit` defer so it runs first;
   both fire on panic too, so a `*Trap` from a step still releases
   lend counts and surfaces an outstanding-borrows trap.
3. **`SuspendLeave` window for params.** The callee's `may_leave`
   flag is cleared while param steps run, so a callee-side `realloc`
   call cannot itself make outgoing calls. Allocate the callee param
   block via `tc.callee.Realloc` if mem params; compute
   `paramSrcBase` / `paramDstBase`.
4. **Run `paramSteps`**, passing the bases. Flat steps canonicalise
   `tc.registers` in place; mem steps move bytes between the
   memories.
5. **Build `calleeCoreArgs`** from `tc.registers[:nCallerFlatParams]`
   (flat) or the callee param-block pointer (mem).
6. **Invoke `calleeFn`.**
7. **Swap `tc.caller ⇄ tc.callee`** so the result-phase visitor
   closures, which hard-code "caller=src, callee=dst", run in the
   correct direction without per-direction flags. Release closures
   on `Task` captured their target table at lend time, so the swap
   doesn't affect teardown.
8. **`SuspendLeave` window for results.** Compute result bases from
   the callee's returned pointer and the caller's pre-pushed retptr
   slot (`tc.registers[1]` if mem params, else
   `tc.registers[nCallerFlatParams]`). Run `resultSteps` with those
   bases.
9. **Post-return hook** if provided, also under `SuspendLeave` on the
   original callee.
10. **Exit callee.**

### `runGocallPlan` (`run_gocall.go`)

1. **Validate** `len(args) == len(plan.paramSteps)`.
2. **Enter callee** (with `Exit` deferred on all paths).
3. **Defer `Task.End`** that surfaces as the returned `err` only if
   no other error is in flight.
4. **`SuspendLeave` window for params.** Allocate callee param block
   if mem params; otherwise `coreArgs = make([]uint64, plan.nParamRegs)`
   and `gcc.registers = coreArgs`.
5. **Run `paramSteps`**, pairing `args[i]` with each step:
   `step(ctx, gcc, args[i], paramBase)`. Each step returns `error`
   directly.
6. **Invoke `calleeFn`.**
7. **Set `gcc.registers = coreResults`** and
   `resultBase = uint32(coreResults[0])`. Flat-mode result steps
   ignore `resultBase`; mem-mode steps read at `resultBase + off`.
8. **Run `resultSteps`**, collecting each returned `Val` into
   `results[i]`.
9. **Post-return hook** under `SuspendLeave`, then **Exit callee**.

`runGocallPlan` does not use `defer/recover` for the call body —
every step returns `error` and propagates through normal returns. The
only `defer`s are the always-fire `Exit` and `Task.End`. (The
`Call`-level entry on `*CallBinding` recovers any `*Trap` panic into
an error so it never leaks out.)

## Zero-allocation transfer

The component→component path through `runTransferPlan` performs **no
Go heap allocations per value**. The `transferPlanStep` closures move
bytes directly between caller and callee `api.Memory` views; strings
and lists are read as memory slices, written through `Realloc`'d
destination memory, and never round-tripped through Go `string` or
`[]Val` intermediates. The only allocations on the hot path are:

- `calleeCoreArgs := make([]uint64, nCallerFlatParams)` for flat
  params (a single small slice), and the wazero-managed
  `calleeResults` return slice.
- The `Task` borrow-accounting record on the context, plus any
  release closures appended by `LendTo` calls during the param
  phase. `Task` itself is pre-allocated; release-closure capture
  cost is one closure per outbound borrow.

Per-call closure objects do **not** allocate: each step is a
`func(...)` value already bound to its captured offsets and indices
at plan-compile time, stored in `paramSteps` / `resultSteps` slices
that live on the plan and are reused across every call through the
adapter.

The Go→component path through `runGocallPlan` materialises input
`args []Val` and output `results []Val` because the public API is
Val-based; the transfer path between two wasm components has no such
materialisation — visitor closures lift from caller memory and lower
into callee memory in one step, with no intermediate `Val`.

## Properties of the visitor design

- **One method per leaf per visitor.** Each leaf type's logic for
  each direction (transfer / lower / lift, flat / mem) appears
  exactly once, as a `Visit*` method body on one of the six
  visitors. There is no Lower×Lift×Transfer matrix to keep in sync.
- **Direct memory access.** Visitor closures call
  `tc.caller.Memory.Read*` / `gcc.callee.Memory.Write*` directly,
  with offsets baked in at plan-compile time. There is no
  `Source`/`Dest` abstraction over the backing store and no
  pre-computed `SlotOffsets` table.
- **Composites are visited natively.** Records, tuples, variants,
  options, and results are handled inside their `Visit*` methods —
  the visitor sees the full composite shape, not a flattened scalar
  stream. The bridge layer (`internal/core/canon_bridge.go`) is just
  `Accept` dispatch plus plan compilation.
- **Composite locality.** Sub-plans for variants, lists, and
  option/result arms are plain `[]step` slices captured in the
  parent closure. Variant flat mode emits sub-steps with absolute
  joined slot indices baked at compile time, so the outer closure
  runs the selected case's sub-plan and zero-fills only the trailing
  tail.
- **Base pointers as data, not state.** Composite recursion passes
  child bases as arguments rather than mutating context fields, so a
  trap mid-walk leaves nothing to restore.

## Resource handles across the canon/core boundary

Canon defines a small set of interfaces that core implements. The
canon package does not import core; it only sees:

- `Instance` — `Enter` / `CanLeave` / `SuspendLeave` / `ResourceTable`.
- `ResourceTable` — `IssueOwn` / `IssueBorrow` / `LookupOwn` /
  `LookupBorrowable` / `Owner`.
- `ResourceType` — pointer-equality identity plus `DefiningInstance`
  and `Destructor`.
- `ResourceHandle` — `HandleID` / `Rep` / `Type` / `LendTo` /
  `TransferOwn` / `Drop` (canon-side view).

Concrete state lives on the core side: `*core.ResourceTable`,
`*core.liveResourceHandle`, and `*core.detachedResourceHandle`. The
canon bridge in `internal/core/canon_bridge.go` adapts each of those
to the canon interfaces (`canonResourceTableView`,
`canonResourceHandleView`).

### Two `core.ResourceHandle` implementations

| Type | Where it comes from | Purpose |
|------|--------------------|---------|
| `*liveResourceHandle` | `ResourceTable.IssueOwn` / `IssueBorrow` / `LookupOwn` / `LookupBorrowable` | Table-backed, validity flag, `Drop` runs the dtor |
| `*detachedResourceHandle` | The same-component `LendTo` shortcut | Rep carrier with no table entry, `numLends`, no `Drop` |

`*liveResourceHandle` panics on use after `valid` flips to false (a
programmer bug, not a recoverable error).

`*detachedResourceHandle` is the shortcut return when `LendTo`
detects that `target.Owner() == rt.DefiningInstance()`. It carries
just `(rt, rep)` — no table allocation, no `numLends` increment on
the callee side. Visitors call `HandleID()` to write `rep` into the
wasm slot. `LendTo` / `TransferOwn` / `Drop` all panic on this
shape; it's a transient artifact of the shortcut and never reaches
user code.

### `Task`-centric borrow lifecycle

All borrow accounting lives on `*Task`:

- `Task.BorrowIssued` / `Task.BorrowDropped` adjust the per-call
  borrow counter.
- `Task.AddRelease(fn)` registers an unlend closure invoked at call
  end (`LendTo` registers one per outbound borrow).
- `Task.End` drains the release closures and returns an error if
  any callee-side borrows remain live.

Both `runTransferPlan` and `runGocallPlan` rely on a single
`defer task.End()` — there is no separate lender slice on the
context, no per-side outstanding-borrow counter elsewhere, and no
duplicated trap path.

### Gocall side: `*ValOwnHandle`

`*ValOwnHandle` (`internal/canon/val.go`) is the gocall-side
participant in the same handle protocol. It implements both
`TransferTarget` (so a lifting visitor can `target.IssueOwn(rt, rep)`
into a fresh `*ValOwnHandle` shell) and `ResourceHandle` (so the
caller can later pass the same value back into `LendTo` /
`TransferOwn` against another target).

Invariants:

- **Gocall is one-way Go→wasm.** Borrow Vals never appear as
  results, and the gocall path never mints a fresh resource — Go
  obtains an own only by lifting one out of a returned `Val`.
- **State machine.** A `*ValOwnHandle` walks `empty → valid →
  {transferred, dropped}`. `Rep` / `Type` panic on `transferred`;
  `Drop` is idempotent on `dropped` and a no-op on `empty`. Methods
  describe the offending state in their error message.
- **Same-component shortcut.** `LendTo` returns a `*repCarrier`
  (the gocall analogue of `*detachedResourceHandle`) when
  `target.Owner() == rt.DefiningInstance()`. Source-side
  `numLends` is incremented unconditionally; the source-side
  release closure registered with `task.AddRelease` always fires.
- **Leak warning, not finalizer-driven dtor.** `IssueOwn` registers
  a `runtime.AddCleanup` that logs a leak warning if the handle is
  GC'd while still valid. The dtor is *not* invoked from a
  finalizer — running it would require entering the defining
  instance from a goroutine the runtime did not control.

There is no separate `*ValBorrowHandle` type; borrow Vals don't
exist on the gocall side, only as an intermediate visitor step.

[CanonicalABI.md]: https://github.com/WebAssembly/component-model/blob/main/design/mvp/CanonicalABI.md
