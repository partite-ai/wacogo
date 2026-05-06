# Calculator example

A minimal wacogo-witgen demo: implements a WIT interface in Go and
wires it into a wasm component.

## Files

- `calculator.wit` — WIT input
- `gen/` — wacogo-witgen output (committed; regenerate via `go generate`)
- `main.go` — runnable demo
- `main_test.go` — runnable test

## Run

```sh
go run github.com/partite-ai/wacogo/examples/calculator
```

Expected: `7 + 35 = 42`.

## Regenerate bindings

```sh
go generate ./examples/calculator/...
```

(Also possible: `cd examples/calculator && wacogo-witgen generate -w example:demo/arith -o ./gen -p github.com/partite-ai/wacogo/examples/calculator/gen ./calculator.wit`)
