// Host-imports example: two Go-implemented components wired together.
// streams provides data-stream resources; filesystem imports streams and
// uses them to implement Open(seed).
//
// Demonstrates the cross-package resource minting pattern: when a host
// component (filesystem) needs to mint a resource defined by an imported
// component (streams), it uses the lender package's
// New<R>HandleIn(definer, impl) free function. The constructor
// registers the impl in definer's resource table and returns an unbound
// handle that the trampoline binds to the caller's canon resource table.
//
// Run with: go run github.com/partite-ai/wacogo/examples/host-imports
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/partite-ai/wacogo"
	filesystem "github.com/partite-ai/wacogo/examples/host-imports/gen/example/host-imports/filesystem"
	streams "github.com/partite-ai/wacogo/examples/host-imports/gen/example/host-imports/streams"
	"github.com/partite-ai/wacogo/host"
)

var _ = host.WithUserState // keep host import in scope for godoc cross-references

//go:generate go run github.com/partite-ai/wacogo/cmd/wacogo-witgen generate -w example:host-imports/host-imports -o ./gen -p github.com/partite-ai/wacogo/examples/host-imports/gen ./host-imports.wit

// myStreams is the Go implementation of the streams interface.
type myStreams struct{}

func (myStreams) NewDataStream(ctx context.Context, seed uint32) (*streams.DataStreamHandle, error) {
	return streams.NewDataStreamHandle(&myDataStream{value: seed}), nil
}

// myDataStream is a single in-memory data-stream resource.
type myDataStream struct{ value uint32 }

func (s *myDataStream) Read(ctx context.Context) (uint32, error)   { return s.value, nil }
func (s *myDataStream) Write(ctx context.Context, v uint32) error { s.value = v; return nil }

// myFilesystem is the Go implementation of the filesystem interface.
// Open creates a new data-stream seeded with the given value. In the
// trampolined path (driven by a wasm consumer) the bind.go trampoline
// calls streams.NewDataStreamHandleIn(streamsInst, impl) to register
// the resource in streamsInst's table and bind the handle to the caller's
// canon resource table. The direct-call path here returns an unbound handle
// as a convenience stub.
type myFilesystem struct {
	streamsInst *host.ComponentInstance //nolint:unused // referenced by trampoline path
}

func (f *myFilesystem) Open(ctx context.Context, seed uint32) (*streams.DataStreamHandle, error) {
	// Direct-call stub: returns an unbound handle (no canon binding).
	// The trampolined export in gen/.../filesystem/filesystem.bind.go
	// uses streams.NewDataStreamHandleIn(streamsInst, impl) to mint
	// a properly bound cross-package handle.
	return streams.NewDataStreamHandle(&myDataStream{value: seed}), nil
}

func main() {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	// Build and instantiate the streams host component. streams defines
	// the data-stream resource.
	streamsFac, err := streams.NewFactory(ctx, e)
	if err != nil {
		log.Fatal(err)
	}
	defer streamsFac.Close(ctx)

	streamsInst, err := streamsFac.NewInstance(ctx, myStreams{}, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer streamsInst.Close(ctx)

	// Build and instantiate the filesystem host component, injecting
	// streamsInst as the lender for the imported data-stream resource.
	fsFac, err := filesystem.NewFactory(ctx, e)
	if err != nil {
		log.Fatal(err)
	}
	defer fsFac.Close(ctx)

	fsImpl := &myFilesystem{streamsInst: streamsInst}
	fsInst, err := fsFac.NewInstance(ctx, fsImpl, &filesystem.Deps{Streams: streamsInst.Core()})
	if err != nil {
		log.Fatal(err)
	}
	defer fsInst.Close(ctx)
	_ = wacogo.WithInstanceImport // keep import alive for examples below

	// Drive the filesystem impl directly. (A wasm consumer of this
	// component would call into the trampolined Open export; here the
	// direct path is a stub that returns an unbound handle — the
	// cross-package mint requires a live wasm trampoline call.)
	ds, err := fsImpl.Open(ctx, 42)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("minted unbound data-stream handle")

	// Drop the handle. With an unbound handle this is a no-op.
	if err := ds.Drop(ctx); err != nil {
		log.Printf("drop error: %v", err)
	}
	fmt.Println("dropped handle")
}
