package preopens

import (
	"context"
	"errors"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"syscall"
	"time"

	wallclock "github.com/partite-ai/wacogo/wasi/clocks/wallclock"
	types "github.com/partite-ai/wacogo/wasi/filesystem/types"
	streams "github.com/partite-ai/wacogo/wasi/io/streams"
)

// Optional capability interfaces. fsDescriptor inspects its underlying
// fs.File for each interface and returns ErrorCodeUnsupported when a
// requested operation has no implementation. The file-level shapes mirror
// methods on *os.File (using io.ReaderAt, io.WriterAt, io.Seeker,
// io.Writer, fs.ReadDirFile, plus Syncer and Truncater); the *Ater
// interfaces cover path-relative ops with no *os.File equivalent.

type Syncer interface {
	Sync() error
}

type Truncater interface {
	Truncate(size int64) error
}

type Chtimeser interface {
	Chtimes(atime, mtime time.Time) error
}

type OpenAter interface {
	OpenAt(name string, flag int, perm fs.FileMode) (fs.File, error)
}

type StatAter interface {
	StatAt(name string) (fs.FileInfo, error)
}

type ReadlinkAter interface {
	ReadlinkAt(name string) (string, error)
}

type ChtimesAter interface {
	ChtimesAt(name string, atime, mtime time.Time) error
}

type MkdirAter interface {
	MkdirAt(name string, perm fs.FileMode) error
}

type RmdirAter interface {
	RmdirAt(name string) error
}

type UnlinkAter interface {
	UnlinkAt(name string) error
}

type SymlinkAter interface {
	SymlinkAt(target, linkName string) error
}

type Unwrapper[T any] interface {
	Unwrap() T
}

type Aser interface {
	As(target any) bool
}

func as[T, U any](x U) (T, bool) {
	for {
		if v, ok := any(x).(T); ok {
			return v, true
		}
		if u, ok := any(x).(Unwrapper[U]); ok {
			x = u.Unwrap()
			continue
		}
		if a, ok := any(x).(Aser); ok {
			var target T
			if a.As(&target) {
				return target, true
			}
		}
		var zero T
		return zero, false
	}
}

// errEscape signals an attempt by an ImmutableFS-wrapped path to
// navigate above its root. fsErr maps it to ErrorCodeNotPermitted.
var errEscape = errors.New("preopens: path escapes root")

// errUnsupported signals that a fallback path could not satisfy a
// capability. fsErr maps it to ErrorCodeUnsupported.
var errUnsupported = errors.New("preopens: capability not supported")

// NewFSPreopens returns a Config.Preopens callback that exposes the root
// of fsys as a single preopened directory at "/". Files supplied through
// fs.FS are read-only; richer fs.File implementations can opt into
// additional operations through the optional capability interfaces in
// this package.
func NewFSPreopens(fsys fs.FS) func(Deps) Preopens {
	return func(deps Deps) Preopens {
		entries := []*PreopenEntry{{Path: "/", Root: ".", FS: fsys}}
		return &fsPreopens{entries: entries, deps: deps}
	}
}

func NewMultiFSPreopens(entries []*PreopenEntry) func(Deps) Preopens {
	return func(deps Deps) Preopens {
		return &fsPreopens{entries: entries, deps: deps}
	}
}

type PreopenEntry struct {
	Path string
	Root string
	FS   fs.FS
}

type fsPreopens struct {
	entries []*PreopenEntry
	deps    Deps
}

func (p *fsPreopens) GetDirectories(_ context.Context) ([]TupleDescriptorString, error) {
	result := make([]TupleDescriptorString, len(p.entries))
	for i, e := range p.entries {
		root, err := e.FS.Open(e.Root)
		if err != nil {
			return nil, err
		}
		desc := &fsDescriptor{file: root, deps: p.deps}
		h := types.NewDescriptorHandleIn(p.deps.Types, desc)
		result[i] = TupleDescriptorString{F0: h, F1: e.Path}
	}
	return result, nil
}

// fsDescriptor adapts an fs.File to wasi:filesystem/types.Descriptor.
type fsDescriptor struct {
	file fs.File
	deps Deps
}

func (d *fsDescriptor) Drop() {
	_ = d.file.Close()
}

func (d *fsDescriptor) GetType(_ context.Context) (types.ResultDescriptorTypeErrorCode, error) {
	info, err := d.file.Stat()
	if err != nil {
		return types.ResultDescriptorTypeErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.ResultDescriptorTypeErrorCodeOk{Value: modeToType(info.Mode())}, nil
}

func (d *fsDescriptor) GetFlags(_ context.Context) (types.ResultDescriptorFlagsErrorCode, error) {
	flags := types.DescriptorFlagsRead
	if _, ok := as[io.Writer](d.file); ok {
		flags |= types.DescriptorFlagsWrite
	} else if _, ok := as[io.WriterAt](d.file); ok {
		flags |= types.DescriptorFlagsWrite
	}
	return types.ResultDescriptorFlagsErrorCodeOk{Value: flags}, nil
}

func (d *fsDescriptor) Stat(_ context.Context) (types.ResultDescriptorStatErrorCode, error) {
	info, err := d.file.Stat()
	if err != nil {
		return types.ResultDescriptorStatErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.ResultDescriptorStatErrorCodeOk{Value: infoToStat(info)}, nil
}

func (d *fsDescriptor) StatAt(_ context.Context, _ types.PathFlags, p string) (types.ResultDescriptorStatErrorCode, error) {
	sa, ok := as[StatAter](d.file)
	if !ok {
		return types.ResultDescriptorStatErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	info, err := sa.StatAt(p)
	if err != nil {
		return types.ResultDescriptorStatErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.ResultDescriptorStatErrorCodeOk{Value: infoToStat(info)}, nil
}

func (d *fsDescriptor) OpenAt(_ context.Context, _ types.PathFlags, p string, of types.OpenFlags, df types.DescriptorFlags) (types.ResultDescriptorErrorCode, error) {
	oa, ok := as[OpenAter](d.file)
	if !ok {
		return types.ResultDescriptorErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	child, err := oa.OpenAt(p, openFlagsToOSFlag(of, df), 0o666)
	if err != nil {
		return types.ResultDescriptorErrorCodeErr{Value: fsErr(err)}, nil
	}
	if of&types.OpenFlagsDirectory != 0 {
		info, statErr := child.Stat()
		if statErr == nil && !info.IsDir() {
			_ = child.Close()
			return types.ResultDescriptorErrorCodeErr{Value: types.ErrorCodeNotDirectory}, nil
		}
	}
	desc := &fsDescriptor{file: child, deps: d.deps}
	h := types.NewDescriptorHandleIn(d.deps.Types, desc)
	return types.ResultDescriptorErrorCodeOk{Value: h}, nil
}

func openFlagsToOSFlag(of types.OpenFlags, df types.DescriptorFlags) int {
	flag := os.O_RDONLY
	if df&types.DescriptorFlagsWrite != 0 {
		if df&types.DescriptorFlagsRead != 0 {
			flag = os.O_RDWR
		} else {
			flag = os.O_WRONLY
		}
	}
	if of&types.OpenFlagsCreate != 0 {
		flag |= os.O_CREATE
	}
	if of&types.OpenFlagsExclusive != 0 {
		flag |= os.O_EXCL
	}
	if of&types.OpenFlagsTruncate != 0 {
		flag |= os.O_TRUNC
	}
	return flag
}

func (d *fsDescriptor) Read(_ context.Context, length uint64, offset uint64) (types.ResultTupleListU8BoolErrorCode, error) {
	info, err := d.file.Stat()
	if err != nil {
		return types.ResultTupleListU8BoolErrorCodeErr{Value: fsErr(err)}, nil
	}
	if info.IsDir() {
		return types.ResultTupleListU8BoolErrorCodeErr{Value: types.ErrorCodeIsDirectory}, nil
	}
	buf := make([]byte, length)
	n, err := readAt(d.file, buf, int64(offset))
	atEOF := errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
	if err != nil && !atEOF {
		return types.ResultTupleListU8BoolErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.ResultTupleListU8BoolErrorCodeOk{Value: types.TupleListU8Bool{F0: buf[:n], F1: atEOF}}, nil
}

func readAt(f fs.File, buf []byte, off int64) (int, error) {
	if ra, ok := as[io.ReaderAt](f); ok {
		return ra.ReadAt(buf, off)
	}
	if s, ok := as[io.Seeker](f); ok {
		if _, err := s.Seek(off, io.SeekStart); err != nil {
			return 0, err
		}
		return io.ReadFull(f, buf)
	}
	if off == 0 {
		return io.ReadFull(f, buf)
	}
	return 0, errUnsupported
}

func (d *fsDescriptor) ReadViaStream(_ context.Context, offset uint64) (types.ResultInputStreamErrorCode, error) {
	info, err := d.file.Stat()
	if err != nil {
		return types.ResultInputStreamErrorCodeErr{Value: fsErr(err)}, nil
	}
	if info.IsDir() {
		return types.ResultInputStreamErrorCodeErr{Value: types.ErrorCodeIsDirectory}, nil
	}
	if offset > 0 {
		s, ok := as[io.Seeker](d.file)
		if !ok {
			return types.ResultInputStreamErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
		}
		if _, err := s.Seek(int64(offset), io.SeekStart); err != nil {
			return types.ResultInputStreamErrorCodeErr{Value: fsErr(err)}, nil
		}
	}
	st := streams.NewIOReaderInputStream(d.deps.Error, d.deps.Poll, d.file)
	h := streams.NewInputStreamHandleIn(d.deps.Streams, st)
	return types.ResultInputStreamErrorCodeOk{Value: h}, nil
}

func (d *fsDescriptor) ReadDirectory(_ context.Context) (types.ResultDirectoryEntryStreamErrorCode, error) {
	rd, ok := as[fs.ReadDirFile](d.file)
	if !ok {
		return types.ResultDirectoryEntryStreamErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	entries, err := rd.ReadDir(-1)
	if err != nil {
		return types.ResultDirectoryEntryStreamErrorCodeErr{Value: fsErr(err)}, nil
	}
	stream := &fsDirEntryStream{entries: entries}
	h := types.NewDirectoryEntryStreamHandleIn(d.deps.Types, stream)
	return types.ResultDirectoryEntryStreamErrorCodeOk{Value: h}, nil
}

func (d *fsDescriptor) MetadataHash(_ context.Context) (types.ResultMetadataHashValueErrorCode, error) {
	info, err := d.file.Stat()
	if err != nil {
		return types.ResultMetadataHashValueErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.ResultMetadataHashValueErrorCodeOk{Value: hashInfo(info.Name(), info)}, nil
}

func (d *fsDescriptor) MetadataHashAt(_ context.Context, _ types.PathFlags, p string) (types.ResultMetadataHashValueErrorCode, error) {
	sa, ok := as[StatAter](d.file)
	if !ok {
		return types.ResultMetadataHashValueErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	info, err := sa.StatAt(p)
	if err != nil {
		return types.ResultMetadataHashValueErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.ResultMetadataHashValueErrorCodeOk{Value: hashInfo(p, info)}, nil
}

func (d *fsDescriptor) IsSameObject(_ context.Context, _ *types.DescriptorHandle) (bool, error) {
	return false, nil
}

func (d *fsDescriptor) ReadlinkAt(_ context.Context, p string) (types.ResultStringErrorCode, error) {
	rl, ok := as[ReadlinkAter](d.file)
	if !ok {
		return types.ResultStringErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	target, err := rl.ReadlinkAt(p)
	if err != nil {
		return types.ResultStringErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.ResultStringErrorCodeOk{Value: target}, nil
}

func (d *fsDescriptor) Sync(_ context.Context) (types.Result_ErrorCode, error) {
	s, ok := as[Syncer](d.file)
	if !ok {
		return types.Result_ErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	if err := s.Sync(); err != nil {
		return types.Result_ErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.Result_ErrorCodeOk{}, nil
}

func (d *fsDescriptor) SyncData(ctx context.Context) (types.Result_ErrorCode, error) {
	return d.Sync(ctx)
}

func (d *fsDescriptor) Advise(_ context.Context, _ uint64, _ uint64, _ types.Advice) (types.Result_ErrorCode, error) {
	return types.Result_ErrorCodeOk{}, nil
}

func (d *fsDescriptor) Write(_ context.Context, data []uint8, offset uint64) (types.ResultU64ErrorCode, error) {
	if wa, ok := as[io.WriterAt](d.file); ok {
		n, err := wa.WriteAt(data, int64(offset))
		if err != nil {
			return types.ResultU64ErrorCodeErr{Value: fsErr(err)}, nil
		}
		return types.ResultU64ErrorCodeOk{Value: uint64(n)}, nil
	}
	if s, ok := as[io.Seeker](d.file); ok {
		if w, ok := as[io.Writer](d.file); ok {
			if _, err := s.Seek(int64(offset), io.SeekStart); err != nil {
				return types.ResultU64ErrorCodeErr{Value: fsErr(err)}, nil
			}
			n, err := w.Write(data)
			if err != nil {
				return types.ResultU64ErrorCodeErr{Value: fsErr(err)}, nil
			}
			return types.ResultU64ErrorCodeOk{Value: uint64(n)}, nil
		}
	}
	return types.ResultU64ErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
}

func (d *fsDescriptor) WriteViaStream(_ context.Context, offset uint64) (types.ResultOutputStreamErrorCode, error) {
	w, ok := as[io.Writer](d.file)
	if !ok {
		return types.ResultOutputStreamErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	if offset > 0 {
		s, ok := as[io.Seeker](d.file)
		if !ok {
			return types.ResultOutputStreamErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
		}
		if _, err := s.Seek(int64(offset), io.SeekStart); err != nil {
			return types.ResultOutputStreamErrorCodeErr{Value: fsErr(err)}, nil
		}
	}
	st := streams.NewIOWriterOutputStream(d.deps.Error, d.deps.Poll, w)
	h := streams.NewOutputStreamHandleIn(d.deps.Streams, st)
	return types.ResultOutputStreamErrorCodeOk{Value: h}, nil
}

func (d *fsDescriptor) AppendViaStream(_ context.Context) (types.ResultOutputStreamErrorCode, error) {
	w, ok := as[io.Writer](d.file)
	if !ok {
		return types.ResultOutputStreamErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	if s, ok := as[io.Seeker](d.file); ok {
		if _, err := s.Seek(0, io.SeekEnd); err != nil {
			return types.ResultOutputStreamErrorCodeErr{Value: fsErr(err)}, nil
		}
	}
	st := streams.NewIOWriterOutputStream(d.deps.Error, d.deps.Poll, w)
	h := streams.NewOutputStreamHandleIn(d.deps.Streams, st)
	return types.ResultOutputStreamErrorCodeOk{Value: h}, nil
}

func (d *fsDescriptor) SetSize(_ context.Context, size uint64) (types.Result_ErrorCode, error) {
	t, ok := as[Truncater](d.file)
	if !ok {
		return types.Result_ErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	if err := t.Truncate(int64(size)); err != nil {
		return types.Result_ErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.Result_ErrorCodeOk{}, nil
}

func (d *fsDescriptor) SetTimes(_ context.Context, atim types.NewTimestamp, mtim types.NewTimestamp) (types.Result_ErrorCode, error) {
	c, ok := as[Chtimeser](d.file)
	if !ok {
		return types.Result_ErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	a, m := resolveTimestamp(atim), resolveTimestamp(mtim)
	if err := c.Chtimes(a, m); err != nil {
		return types.Result_ErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.Result_ErrorCodeOk{}, nil
}

func (d *fsDescriptor) SetTimesAt(_ context.Context, _ types.PathFlags, p string, atim types.NewTimestamp, mtim types.NewTimestamp) (types.Result_ErrorCode, error) {
	c, ok := as[ChtimesAter](d.file)
	if !ok {
		return types.Result_ErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	a, m := resolveTimestamp(atim), resolveTimestamp(mtim)
	if err := c.ChtimesAt(p, a, m); err != nil {
		return types.Result_ErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.Result_ErrorCodeOk{}, nil
}

// resolveTimestamp converts a NewTimestamp variant to a time.Time. Both
// NoChange and Now collapse onto the zero value and time.Now respectively;
// callers are expected to interpret time.Time{} as "leave unchanged" if
// their backend supports it.
func resolveTimestamp(t types.NewTimestamp) time.Time {
	switch v := t.(type) {
	case types.NewTimestampNow:
		return time.Now()
	case types.NewTimestampTimestamp:
		return time.Unix(int64(v.Value.Seconds), int64(v.Value.Nanoseconds))
	}
	return time.Time{}
}

func (d *fsDescriptor) CreateDirectoryAt(_ context.Context, p string) (types.Result_ErrorCode, error) {
	m, ok := as[MkdirAter](d.file)
	if !ok {
		return types.Result_ErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	if err := m.MkdirAt(p, 0o755); err != nil {
		return types.Result_ErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.Result_ErrorCodeOk{}, nil
}

func (d *fsDescriptor) RemoveDirectoryAt(_ context.Context, p string) (types.Result_ErrorCode, error) {
	r, ok := as[RmdirAter](d.file)
	if !ok {
		return types.Result_ErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	if err := r.RmdirAt(p); err != nil {
		return types.Result_ErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.Result_ErrorCodeOk{}, nil
}

func (d *fsDescriptor) UnlinkFileAt(_ context.Context, p string) (types.Result_ErrorCode, error) {
	u, ok := as[UnlinkAter](d.file)
	if !ok {
		return types.Result_ErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	if err := u.UnlinkAt(p); err != nil {
		return types.Result_ErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.Result_ErrorCodeOk{}, nil
}

// LinkAt and RenameAt cross descriptor boundaries. On unix we dispatch
// to linkat(2)/renameat(2) when both descriptors expose Fd() uintptr;
// otherwise (and on non-unix builds) we report Unsupported.

func (d *fsDescriptor) LinkAt(_ context.Context, _ types.PathFlags, srcPath string, newDir *types.DescriptorHandle, dstPath string) (types.Result_ErrorCode, error) {
	dst, ok := dstFile(newDir)
	if !ok {
		return types.Result_ErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	if err := linkAt(d.file, srcPath, dst, dstPath); err != nil {
		return types.Result_ErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.Result_ErrorCodeOk{}, nil
}

func (d *fsDescriptor) RenameAt(_ context.Context, srcPath string, newDir *types.DescriptorHandle, dstPath string) (types.Result_ErrorCode, error) {
	dst, ok := dstFile(newDir)
	if !ok {
		return types.Result_ErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	if err := renameAt(d.file, srcPath, dst, dstPath); err != nil {
		return types.Result_ErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.Result_ErrorCodeOk{}, nil
}

// dstFile resolves the cross-descriptor argument of LinkAt/RenameAt to a
// local fsDescriptor's underlying fs.File. Cross-component handles or
// non-fsDescriptor impls fall back to Unsupported.
func dstFile(h *types.DescriptorHandle) (fs.File, bool) {
	impl, ok := h.LocalImpl()
	if !ok {
		return nil, false
	}
	d, ok := impl.(*fsDescriptor)
	if !ok {
		return nil, false
	}
	return d.file, true
}

// fdFile is satisfied by any fs.File that exposes a unix file descriptor
// (e.g. *os.File). LinkAt/RenameAt require both endpoints to implement it.
type fdFile interface {
	Fd() uintptr
}

func (d *fsDescriptor) SymlinkAt(_ context.Context, target string, p string) (types.Result_ErrorCode, error) {
	s, ok := as[SymlinkAter](d.file)
	if !ok {
		return types.Result_ErrorCodeErr{Value: types.ErrorCodeUnsupported}, nil
	}
	if err := s.SymlinkAt(target, p); err != nil {
		return types.Result_ErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.Result_ErrorCodeOk{}, nil
}

// fsDirEntryStream serves entries from a single fs.ReadDir snapshot.
type fsDirEntryStream struct {
	entries []fs.DirEntry
	cursor  int
}

func (s *fsDirEntryStream) ReadDirectoryEntry(_ context.Context) (types.ResultOptionDirectoryEntryErrorCode, error) {
	if s.cursor >= len(s.entries) {
		return types.ResultOptionDirectoryEntryErrorCodeOk{Value: types.NoneDirectoryEntry()}, nil
	}
	e := s.entries[s.cursor]
	s.cursor++
	entry := types.DirectoryEntry{Type: dirEntryType(e), Name: e.Name()}
	return types.ResultOptionDirectoryEntryErrorCodeOk{Value: types.SomeDirectoryEntry(entry)}, nil
}

func infoToStat(info fs.FileInfo) types.DescriptorStat {
	mt := info.ModTime()
	dt := types.SomeWallclockDatetime(wallclock.Datetime{
		Seconds:     uint64(mt.Unix()),
		Nanoseconds: uint32(mt.Nanosecond()),
	})
	if mt.Unix() < 0 {
		dt = types.NoneWallclockDatetime()
	}
	return types.DescriptorStat{
		Type:                      modeToType(info.Mode()),
		LinkCount:                 1,
		Size:                      uint64(info.Size()),
		DataAccessTimestamp:       dt,
		DataModificationTimestamp: dt,
		StatusChangeTimestamp:     dt,
	}
}

func modeToType(m fs.FileMode) types.DescriptorType {
	switch {
	case m.IsDir():
		return types.DescriptorTypeDirectory
	case m&fs.ModeSymlink != 0:
		return types.DescriptorTypeSymbolicLink
	case m&fs.ModeDevice != 0:
		return types.DescriptorTypeBlockDevice
	case m&fs.ModeCharDevice != 0:
		return types.DescriptorTypeCharacterDevice
	case m&fs.ModeNamedPipe != 0:
		return types.DescriptorTypeFifo
	case m&fs.ModeSocket != 0:
		return types.DescriptorTypeSocket
	case m.IsRegular():
		return types.DescriptorTypeRegularFile
	}
	return types.DescriptorTypeUnknown
}

func dirEntryType(e fs.DirEntry) types.DescriptorType {
	if e.IsDir() {
		return types.DescriptorTypeDirectory
	}
	return modeToType(e.Type())
}

func hashInfo(p string, info fs.FileInfo) types.MetadataHashValue {
	h := fnv.New128a()
	h.Write([]byte(p))
	h.Write([]byte{0})
	var sz [8]byte
	v := uint64(info.Size())
	for i := 0; i < 8; i++ {
		sz[i] = byte(v >> (8 * i))
	}
	h.Write(sz[:])
	mt := info.ModTime().UnixNano()
	for i := 0; i < 8; i++ {
		sz[i] = byte(uint64(mt) >> (8 * i))
	}
	h.Write(sz[:])
	sum := h.Sum(nil)
	var lower, upper uint64
	for i := 0; i < 8; i++ {
		lower |= uint64(sum[i]) << (8 * i)
		upper |= uint64(sum[8+i]) << (8 * i)
	}
	return types.MetadataHashValue{Lower: lower, Upper: upper}
}

func fsErr(err error) types.ErrorCode {
	switch {
	case errors.Is(err, errEscape):
		return types.ErrorCodeNotPermitted
	case errors.Is(err, errUnsupported):
		return types.ErrorCodeUnsupported
	case errors.Is(err, fs.ErrNotExist):
		return types.ErrorCodeNoEntry
	case errors.Is(err, fs.ErrPermission):
		return types.ErrorCodeAccess
	case errors.Is(err, fs.ErrInvalid):
		return types.ErrorCodeInvalid
	case errors.Is(err, fs.ErrExist):
		return types.ErrorCodeExist
	case errors.Is(err, syscall.ENOSPC):
		return types.ErrorCodeInsufficientSpace
	case errors.Is(err, syscall.EDQUOT):
		return types.ErrorCodeInsufficientSpace
	}
	return types.ErrorCodeIo
}
