# wacogo

A WebAssembly Component Model (MVP) runtime for Go, built on [wazero].

Wacogo loads component-model binaries, validates them, and instantiates
them with full cross-component linking. Components implemented in Go
plug into wasm consumers exactly as if they were loaded from a `.wasm`
file.

## Status

Experimental. The component-model MVP type surface works end-to-end —
all primitives, `string`, `list`, `tuple`, `option`, `result`, `record`,
`variant`, `enum`, `flags`, and `resource` (with constructor, methods,
statics, and optional `Drop`) — plus cross-component adapter linking and
`CheckInstantiation` subtype rigor.

Not yet supported: `async`, `stream<T>`, `future<T>`, `error-context`,
`include` of worlds, and worlds with non-interface imports.

## Install

```sh
go get github.com/partite-ai/wacogo
```

Requires Go 1.25 or later.

---

## Use case 1: run a `.wasm` component

Use this path when you have a pre-built component (compiled from Rust,
Go, Python, etc.) and want to call its exports from Go.

```go
import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/partite-ai/wacogo"
    "github.com/partite-ai/wacogo/wasi"
)

func main() {
    ctx := context.Background()
    e := wacogo.NewEngine(ctx)
    defer e.Close(ctx)

    // Pre-built wasi:cli + wasi:http host imports plumbed to this process.
    w, err := wasi.NewWorld(ctx, e, &wasi.Config{
        Args:   os.Args[1:],
        Stdin:  os.Stdin,
        Stdout: os.Stdout,
        Stderr: os.Stderr,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer w.Close(ctx)

    f, err := os.Open("component.wasm")
    if err != nil {
        log.Fatal(err)
    }
    defer f.Close()

    comp, err := e.LoadComponent(ctx, f)
    if err != nil {
        log.Fatal(err)
    }

    inst, err := comp.Instantiate(ctx, w.Imports()...)
    if err != nil {
        log.Fatal(err)
    }
    defer inst.Close(ctx)

    out, err := inst.ExportedFunc("greet").Call(ctx, wacogo.ValString("world"))
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(string(out[0].(wacogo.ValString)))
}
```

A component with no imports can be instantiated as `comp.Instantiate(ctx)`.
For exports nested inside an interface, use
`inst.ExportedInstance("ns:pkg/iface").ExportedFunc("name")`. To wire
individual imports manually, use `wacogo.WithInstanceImport`,
`wacogo.WithFuncImport`, `wacogo.WithModuleImport`,
`wacogo.WithComponentImport`, or `wacogo.WithTypeImport`.

`comp.CheckInstantiation(opts...)` runs the same non-executing import
validation that `Instantiate` uses: every declared import must be present and
have the expected runtime kind, and providers carrying parser type metadata
must satisfy component-model subtyping. It does not check engine or provider
liveness and does not execute the instantiation plan or core start functions,
so a nil result is not a guarantee that `Instantiate` will succeed.

Runnable demo: [`examples/wasi-greet/`](examples/wasi-greet/).

---

## Use case 2: implement a component in Go (`wacogo-witgen`)

Use this path when you want to provide a WIT-described interface in Go
and have wasm components import it.

`wacogo-witgen` reads a `.wit` file and emits Go bindings: a Go
interface for you to implement, and a `Factory` that produces a
`*host.ComponentInstance` ready to plug in as an instance import.

### Install the generator

```sh
go install github.com/partite-ai/wacogo/cmd/wacogo-witgen@latest
```

### Generate bindings

For a WIT file like:

```wit
package example:demo;

interface calc {
    add: func(a: u32, b: u32) -> u32;
}

world arith {
    export calc;
}
```

run:

```sh
wacogo-witgen generate \
    -w example:demo/arith \
    -o ./gen \
    -p github.com/x/y/gen \
    ./calculator.wit
```

This is also a good fit for `go:generate`:

```go
//go:generate go run github.com/partite-ai/wacogo/cmd/wacogo-witgen generate -w example:demo/arith -o ./gen -p github.com/x/y/gen ./calculator.wit
```

### Implement and wire it in

```go
import (
    "context"

    "github.com/partite-ai/wacogo"
    "github.com/x/y/gen/example/demo/calc"
)

type myCalc struct{}

func (myCalc) Add(ctx context.Context, a, b uint32) (uint32, error)      { return a + b, nil }
func (myCalc) Subtract(ctx context.Context, a, b uint32) (uint32, error) { return a - b, nil }
func (myCalc) Multiply(ctx context.Context, a, b uint32) (uint32, error) { return a * b, nil }

ctx := context.Background()
e := wacogo.NewEngine(ctx)
defer e.Close(ctx)

fac, err := calc.NewFactory(ctx, e)
// ... handle err ...
defer fac.Close(ctx)

hostInst, err := fac.NewInstance(ctx, myCalc{}, nil)
// ... handle err ...
defer hostInst.Close(ctx)

// Wire hostInst into a wasm consumer as an instance import:
//   comp.Instantiate(ctx, wacogo.WithInstanceImport("example:demo/calc", hostInst.Core()))
```

See [`cmd/wacogo-witgen/README.md`](cmd/wacogo-witgen/README.md) for the
full CLI reference, the WIT-to-Go type mapping, resources, cross-package
dependencies, and the `<R>` / `<R>Impl` patterns.

---

## Examples

| Directory                                          | Description                                                                       |
| -------------------------------------------------- | --------------------------------------------------------------------------------- |
| [`examples/wasi-greet/`](examples/wasi-greet/)     | Run a `.wasm` component with the full WASI 0.2 world.                             |
| [`examples/jsinterp/`](examples/jsinterp/)         | Embed a JavaScript interpreter component.                                         |
| [`examples/calculator/`](examples/calculator/)     | Implement a WIT interface in Go and call it from a wasm consumer.                 |
| [`examples/host-imports/`](examples/host-imports/) | Two Go-implemented components wired together with cross-package resource sharing. |

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for package layout, the
build / test / regenerate-fixtures workflow, and coding guidelines.

[wazero]: https://github.com/tetratelabs/wazero
