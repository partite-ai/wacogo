package types

import (
	"context"
	"fmt"
	"io"

	"github.com/partite-ai/wacogo/internal/wasi/gen/wasi/http/types"
	"github.com/partite-ai/wacogo/wasi/io/streams"
)

type outgoingBodyImpl struct {
	request *outgoingRequestImpl

	reader        *io.PipeReader
	writer        *io.PipeWriter
	writeOpened   bool
	streamDropped bool
	selfDropped   bool
}

func newOutgoingBodyImpl(request *outgoingRequestImpl) *outgoingBodyImpl {
	return &outgoingBodyImpl{
		request: request,
	}
}

func (b *outgoingBodyImpl) Write(_ context.Context) (ResultOutputStream_, error) {
	if b.writeOpened {
		return types.ResultOutputStream_Err{}, nil
	}
	b.writeOpened = true
	pr, pw := io.Pipe()
	b.reader = pr
	b.writer = pw

	stream := streams.NewIOWriterOutputStream(b.request.errorInst, b.request.pollInst, &outgoingBodyWriter{Writer: pw, body: b})
	handle := streams.NewOutputStreamHandleIn(b.request.streamInst, stream)
	return types.ResultOutputStream_Ok{Value: handle}, nil
}

// Drop releases the body. Per spec, if an output stream was opened
// from this body it must be dropped first; calling Drop while the
// stream is still live returns an error and does not tear the body
// down. Use DestroyOrphan for orphan-time cleanup that bypasses this
// check.
func (b *outgoingBodyImpl) Drop(_ context.Context) error {
	if b.writeOpened && !b.streamDropped {
		return fmt.Errorf("wasi:http/types.outgoing-body: body dropped without finishing")
	}
	b.destroy()
	return nil
}

// DestroyOrphan tears the body down regardless of the stream's drop
// status. Invoked from instance close drain when the guest never
// followed the drop protocol.
func (b *outgoingBodyImpl) DestroyOrphan(_ context.Context) error {
	b.destroy()
	return nil
}

func (b *outgoingBodyImpl) destroy() {
	if b.selfDropped {
		return
	}
	b.selfDropped = true
	if b.writer != nil {
		b.writer.CloseWithError(io.ErrUnexpectedEOF)
	}
	b.writer = nil
}

type outgoingBodyWriter struct {
	io.Writer
	body *outgoingBodyImpl
}

func (b *outgoingBodyWriter) Close() error {
	b.body.streamDropped = true
	return nil
}
