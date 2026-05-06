package main

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	filesystem "github.com/partite-ai/wacogo/examples/host-imports/gen/example/host-imports/filesystem"
	streams "github.com/partite-ai/wacogo/examples/host-imports/gen/example/host-imports/streams"
)

// TestHostImports_CrossPackageMint smoke-tests instantiation of both
// host components. The actual cross-package mint via
// NewDataStreamHandleIn(definer, impl) requires a live wasm trampoline
// call; this test only exercises factory wiring.
func TestHostImports_CrossPackageMint(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	streamsFac, err := streams.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("streams.NewFactory: %v", err)
	}
	defer streamsFac.Close(ctx)

	streamsInst, err := streamsFac.NewInstance(ctx, myStreams{}, nil)
	if err != nil {
		t.Fatalf("streams.NewInstance: %v", err)
	}
	defer streamsInst.Close(ctx)

	fsFac, err := filesystem.NewFactory(ctx, e)
	if err != nil {
		t.Fatalf("filesystem.NewFactory: %v", err)
	}
	defer fsFac.Close(ctx)

	fsImpl := &myFilesystem{streamsInst: streamsInst}
	fsInst, err := fsFac.NewInstance(ctx, fsImpl, &filesystem.Deps{Streams: streamsInst.Core()})
	if err != nil {
		t.Fatalf("filesystem.NewInstance: %v", err)
	}
	defer fsInst.Close(ctx)

	ds, err := fsImpl.Open(ctx, 42)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if ds == nil {
		t.Fatal("Open returned nil")
	}
	if err := ds.Drop(ctx); err != nil {
		t.Fatalf("Drop: %v", err)
	}
}
