package fixturetests_test

import (
	"context"
	"strings"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/filesystem"
	streams "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/streams"
)

// TestFilesystem_CloseFailsWithOutstandingHandles verifies that closing a
// host-built instance whose extTable still has live entries returns an
// error. Open() mints an own<handle> on the streams instance via
// NewHandle(cc, ...); if the caller never drops it, streams.Close must
// refuse to silently tear down state the user impl thinks is live.
//
// Skipped: depends on cross-package mint via cc-threaded impl; see
// wrap_test.go skip comment.
func TestFilesystem_CloseFailsWithOutstandingHandles(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	streamsFac, err := streams.NewFactory(ctx, e)
	if err != nil {
		t.Fatal(err)
	}
	defer streamsFac.Close(ctx)

	streamsInst, err := streamsFac.NewInstance(ctx, &myStreams{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	fsFac, err := filesystem.NewFactory(ctx, e)
	if err != nil {
		t.Fatal(err)
	}
	defer fsFac.Close(ctx)

	fs := &myFilesystem{streamsInst: streamsInst}
	fsInst, err := fsFac.NewInstance(ctx, fs, &filesystem.Deps{Streams: streamsInst.Core()})
	if err != nil {
		t.Fatal(err)
	}
	fs.fsInst = fsInst
	defer fsInst.Close(ctx)

	fsWrap := filesystem.WrapInstance(fsInst, fsInst.Core())
	h, err := fsWrap.Open(ctx) // mints an entry in streams' extTable
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if h == nil {
		t.Fatal("Open returned nil")
	}

	// Caller intentionally does NOT call h.Drop().
	err = streamsInst.Close(ctx)
	if err == nil {
		t.Fatal("expected error closing streams with outstanding extTable entry; got nil")
	}
	if !strings.Contains(err.Error(), "outstanding") {
		t.Errorf("error should mention outstanding handles; got: %v", err)
	}
}

// TestFilesystem_CloseDoesNotPanic verifies that closing a filesystem instance
// that has outstanding stream handles (stored in streams' extTable) does not
// panic. Filesystem does not own the streams handles, so Close only releases
// filesystem's own resources (its stub wasm module).
//
// Skipped: depends on cross-package mint via cc-threaded impl; see
// wrap_test.go skip comment.
func TestFilesystem_CloseDoesNotPanic(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	defer e.Close(ctx)

	streamsFac, err := streams.NewFactory(ctx, e)
	if err != nil {
		t.Fatal(err)
	}
	defer streamsFac.Close(ctx)

	streamsInst, err := streamsFac.NewInstance(ctx, &myStreams{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer streamsInst.Close(ctx)

	fsFac, err := filesystem.NewFactory(ctx, e)
	if err != nil {
		t.Fatal(err)
	}
	defer fsFac.Close(ctx)

	fs := &myFilesystem{streamsInst: streamsInst}
	fsInst, err := fsFac.NewInstance(ctx, fs, &filesystem.Deps{Streams: streamsInst.Core()})
	if err != nil {
		t.Fatal(err)
	}
	fs.fsInst = fsInst

	fsWrap := filesystem.WrapInstance(fsInst, fsInst.Core())
	if _, err := fsWrap.Open(ctx); err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := fsInst.Close(ctx); err != nil {
		t.Errorf("Close: %v", err)
	}
}
