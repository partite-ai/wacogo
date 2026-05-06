# Host imports example

Demonstrates wacogo's host-imports pattern: a Go-implemented `filesystem`
component that imports a Go-implemented `streams` component, with both
exposed as `*wacogo.ComponentInstance` values that can plug into other
wasm components.

`streams` exports a `data-stream` resource with a constructor, `read`, and
`write`. `filesystem` imports `streams` and exposes a single `open(seed)`
function that returns a `data-stream` owned handle.

## Files

- `host-imports.wit` — WIT input
- `gen/` — wacogo-witgen output (committed; regenerate via `go generate`)
- `main.go` — runnable demo
- `main_test.go` — smoke test

## Run

```sh
go run github.com/partite-ai/wacogo/examples/host-imports
```

Expected output:

```
opened seed: 42
after write: 100
```

## Regenerate bindings

```sh
go generate ./examples/host-imports/...
```

## Topology

```
streams (Go: myStreams / myDataStream)
   ↑ wired via NewInstance(streamsInst)
filesystem (Go: myFilesystem)
   ↓
WrapInstance → Filesystem → Open(seed) → DataStream → Read() / Write(v)
```
