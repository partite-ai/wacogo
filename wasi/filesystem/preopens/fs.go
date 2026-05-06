package preopens

import (
	"context"
	"errors"
	"hash/fnv"
	"io"
	"io/fs"
	"path"
	"strings"

	streams "github.com/partite-ai/wacogo/wasi/io/streams"
	wallclock "github.com/partite-ai/wacogo/wasi/clocks/wallclock"
	types "github.com/partite-ai/wacogo/wasi/filesystem/types"
)

// NewFSPreopens returns a Config.Preopens callback that exposes the
// root of fsys as a single preopened directory at "/". Descriptors are
// read-only.
func NewFSPreopens(fsys fs.FS) func(Deps) Preopens {
	return func(deps Deps) Preopens {
		return &fsPreopens{fsys: fsys, deps: deps}
	}
}

type fsPreopens struct {
	fsys fs.FS
	deps Deps
}

func (p *fsPreopens) GetDirectories(_ context.Context) ([]TupleDescriptorString, error) {
	desc := &fsDescriptor{fsys: p.fsys, path: ".", isDir: true, deps: p.deps}
	h := types.NewDescriptorHandleIn(p.deps.Types, desc)
	return []TupleDescriptorString{{F0: h, F1: "/"}}, nil
}

// fsDescriptor adapts a path inside an io/fs.FS to wasi:filesystem/types.Descriptor.
// All write operations return ErrorCodeReadOnly.
type fsDescriptor struct {
	fsys  fs.FS
	path  string // path inside fsys; "." for the root
	isDir bool
	deps  Deps
}

func (d *fsDescriptor) GetType(_ context.Context) (types.ResultDescriptorTypeErrorCode, error) {
	if d.isDir {
		return types.ResultDescriptorTypeErrorCodeOk{Value: types.DescriptorTypeDirectory}, nil
	}
	return types.ResultDescriptorTypeErrorCodeOk{Value: types.DescriptorTypeRegularFile}, nil
}

func (d *fsDescriptor) GetFlags(_ context.Context) (types.ResultDescriptorFlagsErrorCode, error) {
	return types.ResultDescriptorFlagsErrorCodeOk{Value: types.DescriptorFlagsRead}, nil
}

func (d *fsDescriptor) Stat(_ context.Context) (types.ResultDescriptorStatErrorCode, error) {
	info, err := fs.Stat(d.fsys, d.path)
	if err != nil {
		return types.ResultDescriptorStatErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.ResultDescriptorStatErrorCodeOk{Value: infoToStat(info)}, nil
}

func (d *fsDescriptor) StatAt(_ context.Context, _ types.PathFlags, p string) (types.ResultDescriptorStatErrorCode, error) {
	full, ok := joinPath(d.path, p)
	if !ok {
		return types.ResultDescriptorStatErrorCodeErr{Value: types.ErrorCodeNotPermitted}, nil
	}
	info, err := fs.Stat(d.fsys, full)
	if err != nil {
		return types.ResultDescriptorStatErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.ResultDescriptorStatErrorCodeOk{Value: infoToStat(info)}, nil
}

func (d *fsDescriptor) OpenAt(_ context.Context, _ types.PathFlags, p string, of types.OpenFlags, df types.DescriptorFlags) (types.ResultDescriptorErrorCode, error) {
	if of&(types.OpenFlagsCreate|types.OpenFlagsExclusive|types.OpenFlagsTruncate) != 0 {
		return types.ResultDescriptorErrorCodeErr{Value: types.ErrorCodeReadOnly}, nil
	}
	if df&types.DescriptorFlagsWrite != 0 {
		return types.ResultDescriptorErrorCodeErr{Value: types.ErrorCodeReadOnly}, nil
	}
	full, ok := joinPath(d.path, p)
	if !ok {
		return types.ResultDescriptorErrorCodeErr{Value: types.ErrorCodeNotPermitted}, nil
	}
	info, err := fs.Stat(d.fsys, full)
	if err != nil {
		return types.ResultDescriptorErrorCodeErr{Value: fsErr(err)}, nil
	}
	if of&types.OpenFlagsDirectory != 0 && !info.IsDir() {
		return types.ResultDescriptorErrorCodeErr{Value: types.ErrorCodeNotDirectory}, nil
	}
	child := &fsDescriptor{fsys: d.fsys, path: full, isDir: info.IsDir(), deps: d.deps}
	h := types.NewDescriptorHandleIn(d.deps.Types, child)
	return types.ResultDescriptorErrorCodeOk{Value: h}, nil
}

func (d *fsDescriptor) Read(_ context.Context, length uint64, offset uint64) (types.ResultTupleListU8BoolErrorCode, error) {
	if d.isDir {
		return types.ResultTupleListU8BoolErrorCodeErr{Value: types.ErrorCodeIsDirectory}, nil
	}
	f, err := d.fsys.Open(d.path)
	if err != nil {
		return types.ResultTupleListU8BoolErrorCodeErr{Value: fsErr(err)}, nil
	}
	defer f.Close()
	if offset > 0 {
		if err := skipBytes(f, int64(offset)); err != nil {
			if errors.Is(err, io.EOF) {
				return types.ResultTupleListU8BoolErrorCodeOk{Value: types.TupleListU8Bool{F0: nil, F1: true}}, nil
			}
			return types.ResultTupleListU8BoolErrorCodeErr{Value: fsErr(err)}, nil
		}
	}
	buf := make([]byte, length)
	n, err := io.ReadFull(f, buf)
	atEOF := errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
	if err != nil && !atEOF {
		return types.ResultTupleListU8BoolErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.ResultTupleListU8BoolErrorCodeOk{Value: types.TupleListU8Bool{F0: buf[:n], F1: atEOF}}, nil
}

func (d *fsDescriptor) ReadViaStream(_ context.Context, offset uint64) (types.ResultInputStreamErrorCode, error) {
	if d.isDir {
		return types.ResultInputStreamErrorCodeErr{Value: types.ErrorCodeIsDirectory}, nil
	}
	f, err := d.fsys.Open(d.path)
	if err != nil {
		return types.ResultInputStreamErrorCodeErr{Value: fsErr(err)}, nil
	}
	if offset > 0 {
		if err := skipBytes(f, int64(offset)); err != nil && !errors.Is(err, io.EOF) {
			f.Close()
			return types.ResultInputStreamErrorCodeErr{Value: fsErr(err)}, nil
		}
	}
	s := streams.NewIOReaderInputStream(d.deps.Error, d.deps.Poll, f)
	h := streams.NewInputStreamHandleIn(d.deps.Streams, s)
	return types.ResultInputStreamErrorCodeOk{Value: h}, nil
}

func (d *fsDescriptor) ReadDirectory(_ context.Context) (types.ResultDirectoryEntryStreamErrorCode, error) {
	if !d.isDir {
		return types.ResultDirectoryEntryStreamErrorCodeErr{Value: types.ErrorCodeNotDirectory}, nil
	}
	entries, err := fs.ReadDir(d.fsys, d.path)
	if err != nil {
		return types.ResultDirectoryEntryStreamErrorCodeErr{Value: fsErr(err)}, nil
	}
	stream := &fsDirEntryStream{entries: entries}
	h := types.NewDirectoryEntryStreamHandleIn(d.deps.Types, stream)
	return types.ResultDirectoryEntryStreamErrorCodeOk{Value: h}, nil
}

func (d *fsDescriptor) MetadataHash(ctx context.Context) (types.ResultMetadataHashValueErrorCode, error) {
	info, err := fs.Stat(d.fsys, d.path)
	if err != nil {
		return types.ResultMetadataHashValueErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.ResultMetadataHashValueErrorCodeOk{Value: hashInfo(d.path, info)}, nil
}

func (d *fsDescriptor) MetadataHashAt(_ context.Context, _ types.PathFlags, p string) (types.ResultMetadataHashValueErrorCode, error) {
	full, ok := joinPath(d.path, p)
	if !ok {
		return types.ResultMetadataHashValueErrorCodeErr{Value: types.ErrorCodeNotPermitted}, nil
	}
	info, err := fs.Stat(d.fsys, full)
	if err != nil {
		return types.ResultMetadataHashValueErrorCodeErr{Value: fsErr(err)}, nil
	}
	return types.ResultMetadataHashValueErrorCodeOk{Value: hashInfo(full, info)}, nil
}

func (d *fsDescriptor) IsSameObject(_ context.Context, _ *types.DescriptorHandle) (bool, error) {
	// io/fs has no inode/identity primitive and the generated handle
	// wrappers hide the underlying impl, so we can't reliably compare.
	return false, nil
}

func (d *fsDescriptor) ReadlinkAt(_ context.Context, _ string) (types.ResultStringErrorCode, error) {
	return types.ResultStringErrorCodeErr{Value: types.ErrorCodeNotPermitted}, nil
}

func (d *fsDescriptor) Sync(_ context.Context) (types.Result_ErrorCode, error) {
	return types.Result_ErrorCodeOk{}, nil
}

func (d *fsDescriptor) SyncData(_ context.Context) (types.Result_ErrorCode, error) {
	return types.Result_ErrorCodeOk{}, nil
}

// Write-side methods all return read-only errors.

func (d *fsDescriptor) Advise(_ context.Context, _ uint64, _ uint64, _ types.Advice) (types.Result_ErrorCode, error) {
	return types.Result_ErrorCodeOk{}, nil
}

func (d *fsDescriptor) Write(_ context.Context, _ []uint8, _ uint64) (types.ResultU64ErrorCode, error) {
	return types.ResultU64ErrorCodeErr{Value: types.ErrorCodeReadOnly}, nil
}

func (d *fsDescriptor) WriteViaStream(_ context.Context, _ uint64) (types.ResultOutputStreamErrorCode, error) {
	return types.ResultOutputStreamErrorCodeErr{Value: types.ErrorCodeReadOnly}, nil
}

func (d *fsDescriptor) AppendViaStream(_ context.Context) (types.ResultOutputStreamErrorCode, error) {
	return types.ResultOutputStreamErrorCodeErr{Value: types.ErrorCodeReadOnly}, nil
}

func (d *fsDescriptor) SetSize(_ context.Context, _ uint64) (types.Result_ErrorCode, error) {
	return types.Result_ErrorCodeErr{Value: types.ErrorCodeReadOnly}, nil
}

func (d *fsDescriptor) SetTimes(_ context.Context, _ types.NewTimestamp, _ types.NewTimestamp) (types.Result_ErrorCode, error) {
	return types.Result_ErrorCodeErr{Value: types.ErrorCodeReadOnly}, nil
}

func (d *fsDescriptor) SetTimesAt(_ context.Context, _ types.PathFlags, _ string, _ types.NewTimestamp, _ types.NewTimestamp) (types.Result_ErrorCode, error) {
	return types.Result_ErrorCodeErr{Value: types.ErrorCodeReadOnly}, nil
}

func (d *fsDescriptor) CreateDirectoryAt(_ context.Context, _ string) (types.Result_ErrorCode, error) {
	return types.Result_ErrorCodeErr{Value: types.ErrorCodeReadOnly}, nil
}

func (d *fsDescriptor) RemoveDirectoryAt(_ context.Context, _ string) (types.Result_ErrorCode, error) {
	return types.Result_ErrorCodeErr{Value: types.ErrorCodeReadOnly}, nil
}

func (d *fsDescriptor) UnlinkFileAt(_ context.Context, _ string) (types.Result_ErrorCode, error) {
	return types.Result_ErrorCodeErr{Value: types.ErrorCodeReadOnly}, nil
}

func (d *fsDescriptor) LinkAt(_ context.Context, _ types.PathFlags, _ string, _ *types.DescriptorHandle, _ string) (types.Result_ErrorCode, error) {
	return types.Result_ErrorCodeErr{Value: types.ErrorCodeReadOnly}, nil
}

func (d *fsDescriptor) RenameAt(_ context.Context, _ string, _ *types.DescriptorHandle, _ string) (types.Result_ErrorCode, error) {
	return types.Result_ErrorCodeErr{Value: types.ErrorCodeReadOnly}, nil
}

func (d *fsDescriptor) SymlinkAt(_ context.Context, _ string, _ string) (types.Result_ErrorCode, error) {
	return types.Result_ErrorCodeErr{Value: types.ErrorCodeReadOnly}, nil
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

// joinPath resolves p relative to base inside an fs.FS rooted at "."
// and rejects results that escape the root or use absolute paths. The
// returned string is suitable for fs.FS methods (uses forward slashes,
// no leading "/", and uses "." for the root).
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

// skipBytes advances by n bytes, preferring io.Seeker when available.
func skipBytes(f fs.File, n int64) error {
	if seeker, ok := f.(io.Seeker); ok {
		_, err := seeker.Seek(n, io.SeekStart)
		return err
	}
	_, err := io.CopyN(io.Discard, f, n)
	return err
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
	case errors.Is(err, fs.ErrNotExist):
		return types.ErrorCodeNoEntry
	case errors.Is(err, fs.ErrPermission):
		return types.ErrorCodeAccess
	case errors.Is(err, fs.ErrInvalid):
		return types.ErrorCodeInvalid
	case errors.Is(err, fs.ErrExist):
		return types.ErrorCodeExist
	}
	return types.ErrorCodeIo
}

