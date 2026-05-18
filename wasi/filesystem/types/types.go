// Package types is the not-yet-implemented wasi:filesystem/types host
// component. Methods return ErrNotImplemented wrapped with their WIT
// call site until a real implementation lands.
package types

import (
	"context"
	"fmt"

	"github.com/partite-ai/wacogo"
	"github.com/partite-ai/wacogo/host"
	wasierr "github.com/partite-ai/wacogo/wasi/internal/wasierr"
	gen "github.com/partite-ai/wacogo/internal/wasi/gen/wasi/filesystem/types"
	"github.com/partite-ai/wacogo/wasi/io/wioerror"
)

// InterfaceName is the canonical WIT id of this interface.
const InterfaceName = gen.InterfaceName

type (
	Types       = gen.Types
	Descriptor  = gen.Descriptor
	DescriptorHandle = gen.DescriptorHandle

	DirectoryEntryStream       = gen.DirectoryEntryStream
	DirectoryEntryStreamHandle = gen.DirectoryEntryStreamHandle

	DescriptorType = gen.DescriptorType
	ErrorCode      = gen.ErrorCode
	Advice         = gen.Advice

	DescriptorFlags = gen.DescriptorFlags
	PathFlags       = gen.PathFlags
	OpenFlags       = gen.OpenFlags

	OptionDirectoryEntry    = gen.OptionDirectoryEntry
	OptionErrorCode         = gen.OptionErrorCode
	OptionWallclockDatetime = gen.OptionWallclockDatetime

	ResultDescriptorErrorCode    = gen.ResultDescriptorErrorCode
	ResultDescriptorErrorCodeOk  = gen.ResultDescriptorErrorCodeOk
	ResultDescriptorErrorCodeErr = gen.ResultDescriptorErrorCodeErr

	ResultDescriptorFlagsErrorCode    = gen.ResultDescriptorFlagsErrorCode
	ResultDescriptorFlagsErrorCodeOk  = gen.ResultDescriptorFlagsErrorCodeOk
	ResultDescriptorFlagsErrorCodeErr = gen.ResultDescriptorFlagsErrorCodeErr

	ResultDescriptorStatErrorCode    = gen.ResultDescriptorStatErrorCode
	ResultDescriptorStatErrorCodeOk  = gen.ResultDescriptorStatErrorCodeOk
	ResultDescriptorStatErrorCodeErr = gen.ResultDescriptorStatErrorCodeErr

	ResultDescriptorTypeErrorCode    = gen.ResultDescriptorTypeErrorCode
	ResultDescriptorTypeErrorCodeOk  = gen.ResultDescriptorTypeErrorCodeOk
	ResultDescriptorTypeErrorCodeErr = gen.ResultDescriptorTypeErrorCodeErr

	ResultDirectoryEntryStreamErrorCode    = gen.ResultDirectoryEntryStreamErrorCode
	ResultDirectoryEntryStreamErrorCodeOk  = gen.ResultDirectoryEntryStreamErrorCodeOk
	ResultDirectoryEntryStreamErrorCodeErr = gen.ResultDirectoryEntryStreamErrorCodeErr

	ResultInputStreamErrorCode    = gen.ResultInputStreamErrorCode
	ResultInputStreamErrorCodeOk  = gen.ResultInputStreamErrorCodeOk
	ResultInputStreamErrorCodeErr = gen.ResultInputStreamErrorCodeErr

	ResultMetadataHashValueErrorCode    = gen.ResultMetadataHashValueErrorCode
	ResultMetadataHashValueErrorCodeOk  = gen.ResultMetadataHashValueErrorCodeOk
	ResultMetadataHashValueErrorCodeErr = gen.ResultMetadataHashValueErrorCodeErr

	ResultOptionDirectoryEntryErrorCode    = gen.ResultOptionDirectoryEntryErrorCode
	ResultOptionDirectoryEntryErrorCodeOk  = gen.ResultOptionDirectoryEntryErrorCodeOk
	ResultOptionDirectoryEntryErrorCodeErr = gen.ResultOptionDirectoryEntryErrorCodeErr

	ResultOutputStreamErrorCode    = gen.ResultOutputStreamErrorCode
	ResultOutputStreamErrorCodeOk  = gen.ResultOutputStreamErrorCodeOk
	ResultOutputStreamErrorCodeErr = gen.ResultOutputStreamErrorCodeErr

	ResultStringErrorCode    = gen.ResultStringErrorCode
	ResultStringErrorCodeOk  = gen.ResultStringErrorCodeOk
	ResultStringErrorCodeErr = gen.ResultStringErrorCodeErr

	ResultTupleListU8BoolErrorCode    = gen.ResultTupleListU8BoolErrorCode
	ResultTupleListU8BoolErrorCodeOk  = gen.ResultTupleListU8BoolErrorCodeOk
	ResultTupleListU8BoolErrorCodeErr = gen.ResultTupleListU8BoolErrorCodeErr

	ResultU64ErrorCode    = gen.ResultU64ErrorCode
	ResultU64ErrorCodeOk  = gen.ResultU64ErrorCodeOk
	ResultU64ErrorCodeErr = gen.ResultU64ErrorCodeErr

	Result_ErrorCode    = gen.Result_ErrorCode
	Result_ErrorCodeOk  = gen.Result_ErrorCodeOk
	Result_ErrorCodeErr = gen.Result_ErrorCodeErr

	DescriptorStat      = gen.DescriptorStat
	DirectoryEntry      = gen.DirectoryEntry
	MetadataHashValue   = gen.MetadataHashValue
	TupleListU8Bool     = gen.TupleListU8Bool

	NewTimestamp          = gen.NewTimestamp
	NewTimestampNoChange  = gen.NewTimestampNoChange
	NewTimestampNow       = gen.NewTimestampNow
	NewTimestampTimestamp = gen.NewTimestampTimestamp
)

// DescriptorType values (re-exported from the generated bindings).
const (
	DescriptorTypeUnknown         = gen.DescriptorTypeUnknown
	DescriptorTypeBlockDevice     = gen.DescriptorTypeBlockDevice
	DescriptorTypeCharacterDevice = gen.DescriptorTypeCharacterDevice
	DescriptorTypeDirectory       = gen.DescriptorTypeDirectory
	DescriptorTypeFifo            = gen.DescriptorTypeFifo
	DescriptorTypeSymbolicLink    = gen.DescriptorTypeSymbolicLink
	DescriptorTypeRegularFile     = gen.DescriptorTypeRegularFile
	DescriptorTypeSocket          = gen.DescriptorTypeSocket
)

// ErrorCode values (re-exported from the generated bindings).
const (
	ErrorCodeAccess              = gen.ErrorCodeAccess
	ErrorCodeWouldBlock          = gen.ErrorCodeWouldBlock
	ErrorCodeAlready             = gen.ErrorCodeAlready
	ErrorCodeBadDescriptor       = gen.ErrorCodeBadDescriptor
	ErrorCodeBusy                = gen.ErrorCodeBusy
	ErrorCodeDeadlock            = gen.ErrorCodeDeadlock
	ErrorCodeQuota               = gen.ErrorCodeQuota
	ErrorCodeExist               = gen.ErrorCodeExist
	ErrorCodeFileTooLarge        = gen.ErrorCodeFileTooLarge
	ErrorCodeIllegalByteSequence = gen.ErrorCodeIllegalByteSequence
	ErrorCodeInProgress          = gen.ErrorCodeInProgress
	ErrorCodeInterrupted         = gen.ErrorCodeInterrupted
	ErrorCodeInvalid             = gen.ErrorCodeInvalid
	ErrorCodeIo                  = gen.ErrorCodeIo
	ErrorCodeIsDirectory         = gen.ErrorCodeIsDirectory
	ErrorCodeLoop                = gen.ErrorCodeLoop
	ErrorCodeTooManyLinks        = gen.ErrorCodeTooManyLinks
	ErrorCodeMessageSize         = gen.ErrorCodeMessageSize
	ErrorCodeNameTooLong         = gen.ErrorCodeNameTooLong
	ErrorCodeNoDevice            = gen.ErrorCodeNoDevice
	ErrorCodeNoEntry             = gen.ErrorCodeNoEntry
	ErrorCodeNoLock              = gen.ErrorCodeNoLock
	ErrorCodeInsufficientMemory  = gen.ErrorCodeInsufficientMemory
	ErrorCodeInsufficientSpace   = gen.ErrorCodeInsufficientSpace
	ErrorCodeNotDirectory        = gen.ErrorCodeNotDirectory
	ErrorCodeNotEmpty            = gen.ErrorCodeNotEmpty
	ErrorCodeNotRecoverable      = gen.ErrorCodeNotRecoverable
	ErrorCodeUnsupported         = gen.ErrorCodeUnsupported
	ErrorCodeNoTty               = gen.ErrorCodeNoTty
	ErrorCodeNoSuchDevice        = gen.ErrorCodeNoSuchDevice
	ErrorCodeOverflow            = gen.ErrorCodeOverflow
	ErrorCodeNotPermitted        = gen.ErrorCodeNotPermitted
	ErrorCodePipe                = gen.ErrorCodePipe
	ErrorCodeReadOnly            = gen.ErrorCodeReadOnly
	ErrorCodeInvalidSeek         = gen.ErrorCodeInvalidSeek
	ErrorCodeTextFileBusy        = gen.ErrorCodeTextFileBusy
	ErrorCodeCrossDevice         = gen.ErrorCodeCrossDevice
)

// DescriptorFlags values (re-exported from the generated bindings).
const (
	DescriptorFlagsRead               = gen.DescriptorFlagsRead
	DescriptorFlagsWrite              = gen.DescriptorFlagsWrite
	DescriptorFlagsFileIntegritySync  = gen.DescriptorFlagsFileIntegritySync
	DescriptorFlagsDataIntegritySync  = gen.DescriptorFlagsDataIntegritySync
	DescriptorFlagsRequestedWriteSync = gen.DescriptorFlagsRequestedWriteSync
	DescriptorFlagsMutateDirectory    = gen.DescriptorFlagsMutateDirectory
)

// PathFlags values (re-exported from the generated bindings).
const (
	PathFlagsSymlinkFollow = gen.PathFlagsSymlinkFollow
)

// OpenFlags values (re-exported from the generated bindings).
const (
	OpenFlagsCreate    = gen.OpenFlagsCreate
	OpenFlagsDirectory = gen.OpenFlagsDirectory
	OpenFlagsExclusive = gen.OpenFlagsExclusive
	OpenFlagsTruncate  = gen.OpenFlagsTruncate
)

// Option constructors (re-exported from the generated bindings).
var (
	SomeDirectoryEntry      = gen.SomeDirectoryEntry
	NoneDirectoryEntry      = gen.NoneDirectoryEntry
	SomeErrorCode           = gen.SomeErrorCode
	NoneErrorCode           = gen.NoneErrorCode
	SomeWallclockDatetime   = gen.SomeWallclockDatetime
	NoneWallclockDatetime   = gen.NoneWallclockDatetime
)

func NewDescriptorHandle(impl Descriptor) *DescriptorHandle {
	return gen.NewDescriptorHandle(impl)
}
func NewDescriptorHandleIn(definer *host.ComponentInstance, impl Descriptor) *DescriptorHandle {
	return gen.NewDescriptorHandleIn(definer, impl)
}
func NewDirectoryEntryStreamHandle(impl DirectoryEntryStream) *DirectoryEntryStreamHandle {
	return gen.NewDirectoryEntryStreamHandle(impl)
}
func NewDirectoryEntryStreamHandleIn(definer *host.ComponentInstance, impl DirectoryEntryStream) *DirectoryEntryStreamHandle {
	return gen.NewDirectoryEntryStreamHandleIn(definer, impl)
}

type impl struct{}

func (impl) FilesystemErrorCode(_ context.Context, _ *wioerror.ErrorResourceHandle) (OptionErrorCode, error) {
	return OptionErrorCode{}, fmt.Errorf("wasi:filesystem/types.filesystem-error-code: %w", wasierr.ErrNotImplemented)
}

type descriptorImpl struct{}

func (descriptorImpl) Advise(_ context.Context, _ uint64, _ uint64, _ Advice) (Result_ErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.advise: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) AppendViaStream(_ context.Context) (ResultOutputStreamErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.append-via-stream: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) CreateDirectoryAt(_ context.Context, _ string) (Result_ErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.create-directory-at: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) GetFlags(_ context.Context) (ResultDescriptorFlagsErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.get-flags: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) GetType(_ context.Context) (ResultDescriptorTypeErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.get-type: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) IsSameObject(_ context.Context, _ *DescriptorHandle) (bool, error) {
	return false, fmt.Errorf("wasi:filesystem/types.descriptor.is-same-object: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) LinkAt(_ context.Context, _ PathFlags, _ string, _ *DescriptorHandle, _ string) (Result_ErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.link-at: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) MetadataHash(_ context.Context) (ResultMetadataHashValueErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.metadata-hash: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) MetadataHashAt(_ context.Context, _ PathFlags, _ string) (ResultMetadataHashValueErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.metadata-hash-at: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) OpenAt(_ context.Context, _ PathFlags, _ string, _ OpenFlags, _ DescriptorFlags) (ResultDescriptorErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.open-at: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) Read(_ context.Context, _ uint64, _ uint64) (ResultTupleListU8BoolErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.read: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) ReadDirectory(_ context.Context) (ResultDirectoryEntryStreamErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.read-directory: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) ReadViaStream(_ context.Context, _ uint64) (ResultInputStreamErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.read-via-stream: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) ReadlinkAt(_ context.Context, _ string) (ResultStringErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.readlink-at: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) RemoveDirectoryAt(_ context.Context, _ string) (Result_ErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.remove-directory-at: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) RenameAt(_ context.Context, _ string, _ *DescriptorHandle, _ string) (Result_ErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.rename-at: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) SetSize(_ context.Context, _ uint64) (Result_ErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.set-size: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) SetTimes(_ context.Context, _ NewTimestamp, _ NewTimestamp) (Result_ErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.set-times: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) SetTimesAt(_ context.Context, _ PathFlags, _ string, _ NewTimestamp, _ NewTimestamp) (Result_ErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.set-times-at: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) Stat(_ context.Context) (ResultDescriptorStatErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.stat: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) StatAt(_ context.Context, _ PathFlags, _ string) (ResultDescriptorStatErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.stat-at: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) SymlinkAt(_ context.Context, _ string, _ string) (Result_ErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.symlink-at: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) Sync(_ context.Context) (Result_ErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.sync: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) SyncData(_ context.Context) (Result_ErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.sync-data: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) UnlinkFileAt(_ context.Context, _ string) (Result_ErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.unlink-file-at: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) Write(_ context.Context, _ []uint8, _ uint64) (ResultU64ErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.write: %w", wasierr.ErrNotImplemented)
}
func (descriptorImpl) WriteViaStream(_ context.Context, _ uint64) (ResultOutputStreamErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.descriptor.write-via-stream: %w", wasierr.ErrNotImplemented)
}

type directoryEntryStreamImpl struct{}

func (directoryEntryStreamImpl) ReadDirectoryEntry(_ context.Context) (ResultOptionDirectoryEntryErrorCode, error) {
	return nil, fmt.Errorf("wasi:filesystem/types.directory-entry-stream.read-directory-entry: %w", wasierr.ErrNotImplemented)
}

var (
	_ gen.Types               = impl{}
	_ gen.Descriptor          = descriptorImpl{}
	_ gen.DirectoryEntryStream = directoryEntryStreamImpl{}
)

func NewInstance(
	ctx context.Context,
	engine *wacogo.Engine,
	errorInst *host.ComponentInstance,
	pollInst *host.ComponentInstance,
	streamsInst *host.ComponentInstance,
	wallClockInst *host.ComponentInstance,
	opts ...host.InstantiateOption,
) (*host.ComponentInstance, error) {
	fac, err := gen.NewFactory(ctx, engine)
	if err != nil {
		return nil, err
	}
	return fac.NewInstance(ctx, impl{}, &gen.Deps{
		Error:     errorInst.Core(),
		Poll:      pollInst.Core(),
		Streams:   streamsInst.Core(),
		WallClock: wallClockInst.Core(),
	}, opts...)
}
