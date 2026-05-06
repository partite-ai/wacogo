package types

import (
	"context"
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

func (b *outgoingBodyImpl) Drop() {
	if b.writeOpened && !b.streamDropped {
		panic("wasi:http/types.outgoing-body: body dropped without finishing")
	}
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
