package preopens_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/wasi"
	"github.com/partite-ai/wacogo/wasi/filesystem/preopens"
	"github.com/partite-ai/wacogo/wasi/filesystem/types"
)

func TestFSPreopens(t *testing.T) {
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	t.Cleanup(func() {
		if err := e.Close(ctx); err != nil {
			t.Errorf("engine.Close: %v", err)
		}
	})

	fsys := fstest.MapFS{
		"hello.txt":         {Data: []byte("hello world")},
		"sub/inner.txt":     {Data: []byte("nested")},
		"sub/more/deep.txt": {Data: []byte("deeper")},
	}

	cfg := wasi.Config{
		Preopens: preopens.NewFSPreopens(fsys),
	}
	w, err := wasi.NewWorld(ctx, e, &cfg)
	if err != nil {
		t.Fatalf("NewWorld: %v", err)
	}
	t.Cleanup(func() { _ = w.Close(ctx) })

	// Get the preopened directory descriptor by calling the impl directly.
	// (End-to-end through wasm would require a real component to call into.)
	impl := preopens.NewFSPreopens(fsys)(preopens.Deps{
		Types:     w.FilesystemTypes,
		Streams:   w.Streams,
		Error:     w.Error,
		Poll:      w.Poll,
		WallClock: w.WallClock,
	})
	dirs, err := impl.GetDirectories(ctx)
	if err != nil {
		t.Fatalf("GetDirectories: %v", err)
	}
	if len(dirs) != 1 {
		t.Fatalf("got %d dirs, want 1", len(dirs))
	}
	if dirs[0].F1 != "/" {
		t.Errorf("dir name = %q, want /", dirs[0].F1)
	}

	root := dirs[0].F0

	gotType, err := root.GetType(ctx)
	if err != nil {
		t.Fatalf("GetType: %v", err)
	}
	if got, ok := gotType.(types.ResultDescriptorTypeErrorCodeOk); !ok || got.Value != types.DescriptorTypeDirectory {
		t.Errorf("root GetType = %v, want directory", gotType)
	}

	// Read the file via positional Read.
	openRes, err := root.OpenAt(ctx, 0, "hello.txt", 0, types.DescriptorFlagsRead)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	openOk, ok := openRes.(types.ResultDescriptorErrorCodeOk)
	if !ok {
		t.Fatalf("OpenAt result = %v, want Ok", openRes)
	}
	file := openOk.Value
	t.Cleanup(func() { _ = file.Drop(ctx) })

	readRes, err := file.Read(ctx, 100, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	readOk, ok := readRes.(types.ResultTupleListU8BoolErrorCodeOk)
	if !ok {
		t.Fatalf("Read result = %v, want Ok", readRes)
	}
	if string(readOk.Value.F0) != "hello world" {
		t.Errorf("Read content = %q, want %q", readOk.Value.F0, "hello world")
	}
	if !readOk.Value.F1 {
		t.Errorf("Read EOF flag = false, want true")
	}

	// OpenAt rejects writes.
	_, err = root.OpenAt(ctx, 0, "new.txt", types.OpenFlagsCreate, types.DescriptorFlagsRead)
	if err != nil {
		t.Fatalf("OpenAt with create: %v", err)
	}

	writeRes, err := root.Write(ctx, []byte("x"), 0)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got, ok := writeRes.(types.ResultU64ErrorCodeErr); !ok || got.Value != types.ErrorCodeReadOnly {
		t.Errorf("Write = %v, want ReadOnly err", writeRes)
	}

	// OpenAt rejects path escapes.
	escapeRes, err := root.OpenAt(ctx, 0, "../etc/passwd", 0, types.DescriptorFlagsRead)
	if err != nil {
		t.Fatalf("OpenAt escape: %v", err)
	}
	if got, ok := escapeRes.(types.ResultDescriptorErrorCodeErr); !ok || got.Value != types.ErrorCodeNotPermitted {
		t.Errorf("OpenAt escape = %v, want NotPermitted err", escapeRes)
	}

	// Read directory entries.
	dirRes, err := root.ReadDirectory(ctx)
	if err != nil {
		t.Fatalf("ReadDirectory: %v", err)
	}
	dirOk, ok := dirRes.(types.ResultDirectoryEntryStreamErrorCodeOk)
	if !ok {
		t.Fatalf("ReadDirectory = %v, want Ok", dirRes)
	}
	stream := dirOk.Value
	t.Cleanup(func() { _ = stream.Drop(ctx) })

	names := map[string]bool{}
	for {
		entryRes, err := stream.ReadDirectoryEntry(ctx)
		if err != nil {
			t.Fatalf("ReadDirectoryEntry: %v", err)
		}
		entryOk, ok := entryRes.(types.ResultOptionDirectoryEntryErrorCodeOk)
		if !ok {
			t.Fatalf("ReadDirectoryEntry = %v, want Ok", entryRes)
		}
		if !entryOk.Value.IsSome {
			break
		}
		names[entryOk.Value.Value.Name] = true
	}
	if !names["hello.txt"] || !names["sub"] {
		t.Errorf("dir entries = %v, missing hello.txt or sub", names)
	}
}
