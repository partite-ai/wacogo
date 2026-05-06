# wacogo-witgen

Generates Go bindings for WIT interfaces, targeting the [wacogo `host`
package](../../host/). Implementing your WIT in Go and wiring it into a
wasm component instance becomes a matter of implementing a generated
Go interface and calling `Factory.NewInstance(impl)`.

## Install

```sh
go install github.com/partite-ai/wacogo/cmd/wacogo-witgen@latest
```

## Usage

```sh
wacogo-witgen generate \
    -w <world-id> \
    -o <out-dir> \
    -p <go-package-root> \
    <wit-file>
```

### Required flags

| Short | Long             | Description                                                               |
| ----- | ---------------- | ------------------------------------------------------------------------- |
| `-w`  | `--world`        | Fully-qualified world id (e.g. `wasi:io/streams`)                         |
| `-o`  | `--out`          | Output directory for generated files                                      |
| `-p`  | `--package-root` | Go module path prefix for generated package declarations and import paths |

### Options

| Flag        | Description                                       |
| ----------- | ------------------------------------------------- |
| `--dry-run` | Print files that would be written; no disk writes |

## Output layout

For input `bindings.wit` containing world `foo:bar/my-world` whose
imports include interface `foo:bar/calc`, output is:

```
<out>/foo/bar/calc/calc.go         # Clean: types + interfaces
<out>/foo/bar/calc/calc.bind.go    # Generated factory + marshalling
```

## go:generate recipe

Add to a `.go` file in your project:

```go
//go:generate go run github.com/partite-ai/wacogo/cmd/wacogo-witgen generate -w foo:bar/my-world -o ./bindings -p github.com/x/y/bindings ./bindings.wit
package bindings
```

Then run `go generate ./...` from your project root.

## What you get

For a WIT like:

```wit
package example:demo;

interface calc {
    add: func(a: u32, b: u32) -> u32;
}

world arith {
    import calc;
}
```

You get an interface to implement:

```go
package calc

type Calc interface {
    Add(a uint32, b uint32) uint32
}

func NewFactory(e *wacogo.Engine) (*Factory, error)
func (f *Factory) NewInstance(impl Calc) (*wacogo.ComponentInstance, error)
```

User-side wiring:

```go
type myCalc struct{}
func (myCalc) Add(a, b uint32) uint32 { return a + b }

fac, _ := calc.NewFactory(engine)
inst, _ := fac.NewInstance(myCalc{})
// inst is a *wacogo.ComponentInstance — pass to wacogo.WithInstanceImport
```

## Supported WIT features

Every value-level WIT type is supported:

- Primitives: `bool, u8…u64, s8…s64, f32, f64, char`
- `string`, `list<T>`, `tuple<T...>`
- `option<T>`, `result<T, E>` (top-level returns render as `(T, error)`)
- `record`, `variant` (sealed Go interface + per-case structs), `enum`, `flags`
- `resource` with constructor, methods, static funcs, and optional `Drop()`

### Type mapping reference

| WIT                   | Go (in field / nested)                                           | Go (in top-level result) |
| --------------------- | ---------------------------------------------------------------- | ------------------------ |
| `bool`                | `bool`                                                           | same                     |
| `u8…u64`              | `uint8…uint64`                                                   | same                     |
| `s8…s64`              | `int8…int64`                                                     | same                     |
| `f32 / f64`           | `float32 / float64`                                              | same                     |
| `char`                | `rune`                                                           | same                     |
| `string`              | `string`                                                         | same                     |
| `list<T>`             | `[]T`                                                            | same                     |
| `tuple<u32, string>`  | `TupleU32String{F0 uint32; F1 string}`                           | same                     |
| `option<T>`           | `OptionT{IsSome bool; Value T}` + constructors `SomeT()/NoneT()` | same                     |
| `result`              | `Result__` struct                                                | `error`                  |
| `result<T>`           | `ResultT_` struct                                                | `(T, error)`             |
| `result<_, string>`   | `Result_String` struct                                           | `error`                  |
| `result<T, string>`   | `ResultTString` struct                                           | `(T, error)`             |
| `enum {a, b}`         | `type X uint8` + `Xa, Xb` constants + `String()`                 | same                     |
| `flags {a, b}`        | `type X uint32` + `Xa = 1<<0` constants                          | same                     |
| `record {x: u32}`     | `type X struct { X uint32 }`                                     | same                     |
| `variant {a, b(u32)}` | sealed `type X interface{ isX() }` + `Xa{}` + `Xb{Value uint32}` | same                     |
| `resource R`          | `type R interface { ...methods... }` (Drop optional)             | same                     |
| `own<R> / borrow<R>`  | `R` (the Go interface)                                           | same                     |

### Resource types: `<R>` vs `<R>Impl`

Each WIT resource produces TWO Go interfaces in its owning package:

| Type      | Role                         | Who satisfies                                                                                    |
| --------- | ---------------------------- | ------------------------------------------------------------------------------------------------ |
| `<R>`     | Sealed cross-package handle. | Generated `*localx<R>` (wraps a local impl) and `*remotex<R>` (consumes a wasm-resident handle). |
| `<R>Impl` | Implementer interface.       | User code when implementing the resource locally.                                                |

The implementer-side parent interface (`<Iface>Impl`) returns `<R>Impl`
from constructors. The consumer-side wrapper (`<Iface>`, returned by
`WrapInstance`) returns `<R>`. Cross-package uses always reference
the sealed `<R>` form.

### Consuming a generated package

Every package now exports `WrapInstance(inst *host.ComponentInstance) <Iface>`,
which returns a value satisfying the consumer-side `<Iface>`. All method
calls dispatch via `CallRaw` on the underlying instance's exports. The
returned wrapper holds no Go-side state — it's a thin proxy over the
wasm-resident component.

```go
// Same package as `streams`:
streamsInst, _ := streams.NewFactory(e).NewInstance(myStreams{})

// Anywhere:
streamsView := streams.WrapInstance(streamsInst)
handle := streamsView.NewStream(42)
fmt.Println(handle.Read())
```

### Composing components

When an interface imports types from another (`use other.{type}`), the
generated `Factory.NewInstance` takes one positional `*wacogo.ComponentInstance`
per imported source interface, in alphabetical order:

```go
fsInst, _ := filesystem.NewFactory(e).NewInstance(
    &myFilesystem{streams: streams.WrapInstance(streamsInst)},
    streamsInst,  // positional dep: streams instance
)
```

Behind the scenes the factory uses `host.AddResourceRef` (Build time)
and `host.WithResourceFrom` (Instantiate time) to bind the imported
resource type to the lender's `*host.ResourceType`.

See [`../../examples/host-imports/`](../../examples/host-imports/) for a runnable demo.

### Resource hoisting

Constructor and static functions on a resource are hoisted to the
parent interface's implementer:

| WIT inside `resource counter`    | Go on the parent `Counters` interface |
| -------------------------------- | ------------------------------------- |
| `constructor(initial: u32)`      | `NewCounter(initial uint32) Counter`  |
| `static open: func() -> counter` | `CounterOpen() Counter`               |
| `inc: func() -> u32`             | (on `Counter` interface, not parent)  |

### `Drop()` is optional

If your resource impl has cleanup work, add a `Drop()` method:

```go
type myCounter struct { /* ... */ }
func (c *myCounter) Drop() { /* close handles, etc. */ }
```

The host package's destructor runs `Drop()` if the type assertion
succeeds. Otherwise no cleanup callback is invoked.

## Not yet supported

- `async`, `stream<T>`, `future<T>`, `error-context`
- Worlds with non-interface imports (instances, type-import)
- Multiple results per function (wit-tools 1.227+ removed this anyway)

## Examples

See [`../../examples/`](../../examples/) for runnable demos.
