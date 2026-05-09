package preopens

import (
	"io/fs"
	"os"
	"path"
	"strings"
)

// ImmutableFS wraps an fs.FS so directory fs.Files returned by Open
// satisfy OpenAter, StatAter, and ReadlinkAter by re-resolving paths
// against the underlying fs.FS.
//
// These operations are NOT FD-relative: every call does a fresh path
// lookup through the wrapped fs.FS, which means they are vulnerable to
// TOCTOU races if the underlying filesystem can mutate between calls.
//
// Use ImmutableFS only with filesystems guaranteed not to change while
// in use — embedded assets (//go:embed), fstest.MapFS, or other
// read-only snapshots. For mutable backends, supply an fs.File whose
// OpenAt is rooted at the directory's file descriptor (e.g. using
// golang.org/x/sys/unix.Openat against Fd()).
type ImmutableFS struct {
	FS fs.FS
}

// Open implements fs.FS by delegating to the wrapped fs.FS and, for
// directories, returning a wrapper that exposes path-relative ops.
func (i ImmutableFS) Open(name string) (fs.File, error) {
	return openImmutable(i.FS, name)
}

func openImmutable(fsys fs.FS, p string) (fs.File, error) {
	f, err := fsys.Open(p)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !info.IsDir() {
		return f, nil
	}
	return &immutableDir{fsys: fsys, path: p, file: f}, nil
}

// immutableDir is the directory fs.File handed out by ImmutableFS. It
// implements fs.ReadDirFile, OpenAter, StatAter, and ReadlinkAter on
// top of the wrapped fs.FS.
type immutableDir struct {
	fsys fs.FS
	path string
	file fs.File
}

func (d *immutableDir) Stat() (fs.FileInfo, error) { return d.file.Stat() }
func (d *immutableDir) Read(b []byte) (int, error) { return d.file.Read(b) }
func (d *immutableDir) Close() error               { return d.file.Close() }

func (d *immutableDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if rd, ok := d.file.(fs.ReadDirFile); ok {
		return rd.ReadDir(n)
	}
	return fs.ReadDir(d.fsys, d.path)
}

func (d *immutableDir) OpenAt(name string, flag int, _ fs.FileMode) (fs.File, error) {
	if flag != os.O_RDONLY {
		return nil, fs.ErrPermission
	}
	full, ok := joinPath(d.path, name)
	if !ok {
		return nil, errEscape
	}
	return openImmutable(d.fsys, full)
}

func (d *immutableDir) StatAt(name string) (fs.FileInfo, error) {
	full, ok := joinPath(d.path, name)
	if !ok {
		return nil, errEscape
	}
	return fs.Stat(d.fsys, full)
}

func (d *immutableDir) ReadlinkAt(name string) (string, error) {
	full, ok := joinPath(d.path, name)
	if !ok {
		return "", errEscape
	}
	return fs.ReadLink(d.fsys, full)
}

// joinPath resolves p relative to base inside an fs.FS rooted at "."
// and rejects results that escape the root or use absolute paths.
func joinPath(base, p string) (string, bool) {
	if strings.HasPrefix(p, "/") {
		return "", false
	}
	joined := path.Clean(path.Join(base, p))
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return "", false
	}
	return joined, true
}
