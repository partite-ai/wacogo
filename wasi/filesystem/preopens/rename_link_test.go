package preopens_test

import (
	"context"
	"errors"
	"io/fs"
	"syscall"
	"testing"
	"time"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/wasi"
	"github.com/partite-ai/wacogo/wasi/filesystem/preopens"
	"github.com/partite-ai/wacogo/wasi/filesystem/types"
)

// capFS is a single-directory filesystem whose root implements the
// RenameAter/LinkAter/RmdirAter capabilities, returning canned errors and
// recording its calls.
type capFS struct{ root *capDir }

func (f capFS) Open(name string) (fs.File, error) {
	if name != "." {
		return nil, fs.ErrNotExist
	}
	return f.root, nil
}

type capDir struct {
	err   error // returned by every capability
	calls []string
}

type capDirInfo struct{}

func (capDirInfo) Name() string       { return "." }
func (capDirInfo) Size() int64        { return 0 }
func (capDirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o755 }
func (capDirInfo) ModTime() time.Time { return time.Time{} }
func (capDirInfo) IsDir() bool        { return true }
func (capDirInfo) Sys() any           { return nil }

func (d *capDir) Stat() (fs.FileInfo, error) { return capDirInfo{}, nil }
func (d *capDir) Read([]byte) (int, error)   { return 0, errors.New("is a directory") }
func (d *capDir) Close() error               { return nil }

func (d *capDir) RenameAt(oldName string, newDir fs.File, newName string) error {
	if _, ok := preopens.As[*capDir](newDir); !ok {
		return syscall.EXDEV
	}
	d.calls = append(d.calls, "rename "+oldName+" "+newName)
	return d.err
}

func (d *capDir) LinkAt(oldName string, newDir fs.File, newName string) error {
	d.calls = append(d.calls, "link "+oldName+" "+newName)
	return d.err
}

func (d *capDir) RmdirAt(name string) error {
	d.calls = append(d.calls, "rmdir "+name)
	return d.err
}

// rootDescriptor returns the preopened root descriptor for fsys.
func rootDescriptor(t *testing.T, fsys fs.FS) *types.DescriptorHandle {
	t.Helper()
	ctx := context.Background()
	e := wacogo.NewEngine(ctx)
	t.Cleanup(func() { _ = e.Close(ctx) })
	w, err := wasi.NewWorld(ctx, e, &wasi.Config{Preopens: preopens.NewFSPreopens(fsys)})
	if err != nil {
		t.Fatalf("NewWorld: %v", err)
	}
	t.Cleanup(func() { _ = w.Close(ctx) })
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
	t.Cleanup(func() { _ = dirs[0].F0.Drop(ctx) })
	return dirs[0].F0
}

func errorCode(t *testing.T, res types.Result_ErrorCode) (types.ErrorCode, bool) {
	t.Helper()
	switch r := res.(type) {
	case types.Result_ErrorCodeOk:
		return 0, false
	case types.Result_ErrorCodeErr:
		return r.Value, true
	}
	t.Fatalf("unexpected result %T", res)
	return 0, false
}

func TestRenameAtCapability(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		err      error
		wantCode types.ErrorCode
		wantErr  bool
	}{
		{name: "ok"},
		{name: "exists", err: fs.ErrExist, wantCode: types.ErrorCodeExist, wantErr: true},
		{name: "not empty", err: syscall.ENOTEMPTY, wantCode: types.ErrorCodeNotEmpty, wantErr: true},
		// Falls back to renameat(2), which needs Fd(): unsupported here.
		{name: "unsupported", err: errors.ErrUnsupported, wantCode: types.ErrorCodeUnsupported, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := &capDir{err: tc.err}
			dir := rootDescriptor(t, capFS{root})
			res, err := dir.RenameAt(ctx, "a", dir, "b")
			if err != nil {
				t.Fatalf("RenameAt: %v", err)
			}
			code, isErr := errorCode(t, res)
			if isErr != tc.wantErr || (isErr && code != tc.wantCode) {
				t.Fatalf("RenameAt = %v (err=%v), want code %v (err=%v)", code, isErr, tc.wantCode, tc.wantErr)
			}
			if len(root.calls) != 1 || root.calls[0] != "rename a b" {
				t.Errorf("calls = %v", root.calls)
			}
		})
	}
}

func TestRenameAtCrossDevice(t *testing.T) {
	ctx := context.Background()
	src := rootDescriptor(t, capFS{&capDir{}})
	other := rootDescriptor(t, preopens.ImmutableFS{FS: capFS{&capDir{}}})
	// The destination is wrapped by ImmutableFS, which As sees through, so
	// it still counts as a capDir.
	res, err := src.RenameAt(ctx, "a", other, "b")
	if err != nil {
		t.Fatalf("RenameAt: %v", err)
	}
	if _, isErr := errorCode(t, res); isErr {
		t.Fatalf("RenameAt into wrapped capDir failed: %v", res)
	}

	// A directory of another kind of filesystem is cross-device.
	foreign := rootDescriptor(t, emptyDirFS{})
	res, err = src.RenameAt(ctx, "a", foreign, "b")
	if err != nil {
		t.Fatalf("RenameAt: %v", err)
	}
	if code, isErr := errorCode(t, res); !isErr || code != types.ErrorCodeCrossDevice {
		t.Fatalf("RenameAt across filesystems = %v, want cross-device", res)
	}
}

func TestLinkAtCapability(t *testing.T) {
	ctx := context.Background()
	root := &capDir{}
	dir := rootDescriptor(t, capFS{root})
	res, err := dir.LinkAt(ctx, 0, "a", dir, "b")
	if err != nil {
		t.Fatalf("LinkAt: %v", err)
	}
	if _, isErr := errorCode(t, res); isErr {
		t.Fatalf("LinkAt = %v, want ok", res)
	}
	if len(root.calls) != 1 || root.calls[0] != "link a b" {
		t.Errorf("calls = %v", root.calls)
	}
}

func TestErrnoMapping(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		err  error
		want types.ErrorCode
	}{
		{syscall.ENOTEMPTY, types.ErrorCodeNotEmpty},
		{syscall.ENOTDIR, types.ErrorCodeNotDirectory},
		{syscall.EISDIR, types.ErrorCodeIsDirectory},
		{syscall.EXDEV, types.ErrorCodeCrossDevice},
		{&fs.PathError{Op: "rmdir", Path: "x", Err: syscall.ENOTEMPTY}, types.ErrorCodeNotEmpty},
		{fs.ErrExist, types.ErrorCodeExist},
	} {
		dir := rootDescriptor(t, capFS{&capDir{err: tc.err}})
		res, err := dir.RemoveDirectoryAt(ctx, "x")
		if err != nil {
			t.Fatalf("RemoveDirectoryAt: %v", err)
		}
		if code, isErr := errorCode(t, res); !isErr || code != tc.want {
			t.Errorf("%v: got %v, want %v", tc.err, res, tc.want)
		}
	}
}

// emptyDirFS is a filesystem with an empty root and no capabilities.
type emptyDirFS struct{}

func (emptyDirFS) Open(name string) (fs.File, error) {
	if name != "." {
		return nil, fs.ErrNotExist
	}
	return emptyDir{}, nil
}

type emptyDir struct{}

func (emptyDir) Stat() (fs.FileInfo, error) { return capDirInfo{}, nil }
func (emptyDir) Read([]byte) (int, error)   { return 0, errors.New("is a directory") }
func (emptyDir) Close() error               { return nil }
