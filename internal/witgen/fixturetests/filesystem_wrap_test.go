package fixturetests_test

import (
	"context"
	"testing"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/filesystem"
	streams "github.com/partite-ai/wacogo/internal/witgen/testdata/genfixtures/example/demo/streams"
)

// TestFilesystem_WrapInstance_OpenReturnsStreamsHandle verifies that a
// filesystem wrapper's Open() method correctly lifts a cross-package
// own<handle> result from wasm into a *streams.HandleHandle.
//
// The impl returns an unregistered *streams.HandleHandle whose definer
// is set to the streams instance via NewHandleHandleIn; the bind.go
// trampoline calls streams.HandleValueFor on the way out, which drives
// the unregisteredHandleState bind path: registers the impl on the
// streams instance's extTable, issues an own handle on cc.Instance()
// (the filesystem instance), and Invalidate clears the impl-side
// reference. The wrap.go return reader then re-issues the canon
// handle on the caller side and constructs a remote *HandleHandle.
func TestFilesystem_WrapInstance_OpenReturnsStreamsHandle(t *testing.T) {
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
	defer fsInst.Close(ctx)
	fs.fsInst = fsInst

	fsWrap := filesystem.WrapInstance(fsInst, fsInst.Core())
	h, err := fsWrap.Open(ctx)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if h == nil {
		t.Fatal("Open returned nil handle")
	}
	got, err := h.Read(ctx)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got != 42 {
		t.Errorf("Read: got %d, want 42", got)
	}
	if err := h.Drop(ctx); err != nil {
		t.Errorf("Drop: %v", err)
	}
}

// myStreams is the host implementation of the streams interface used in the test.
type myStreams struct{}

func (myStreams) NewHandle(ctx context.Context) (*streams.HandleHandle, error) {
	return streams.NewHandleHandle(&myHandle{}), nil
}

// myHandle is a test handle implementation that always returns 42 from Read.
type myHandle struct{}

func (*myHandle) Read(ctx context.Context) (uint32, error) { return 42, nil }

// myFilesystem is the host implementation of the filesystem interface used in
// the test. Open() returns a *streams.HandleHandle constructed via
// NewHandleHandleIn so the impl is registered on the streams instance's
// extTable when the trampoline drives the bind state machine on the way out.
type myFilesystem struct {
	streamsInst *host.ComponentInstance
	fsInst      *host.ComponentInstance
}

func (f *myFilesystem) Open(ctx context.Context) (*streams.HandleHandle, error) {
	return streams.NewHandleHandleIn(f.streamsInst, &myHandle{}), nil
}
