package streams

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/wasi/io/poll"
	"github.com/partite-ai/wacogo/wasi/io/wioerror"
)

const (
	defaultIOWriterBufferSize = 4 * 1024
	blockingWriteCap          = 4096
)

// Flusher is the optional interface implemented by io.Writer destinations
// that support an explicit flush. If the underlying writer satisfies it,
// flush and blocking-flush invoke Flush after all buffered bytes have
// drained.
type Flusher interface {
	Flush() error
}

// BufferSizeOption configures the ring-buffer capacity for both
// IOWriterOutputStream and IOReaderInputStream.
type BufferSizeOption struct{ n int }

// WithBufferSize sets the ring-buffer capacity in bytes. For an
// output stream it caps both check-write's permit and the data the
// producer can stage ahead of the drainer; for an input stream it
// bounds how far the pumper reads ahead of the consumer. Defaults to
// 4096 bytes.
func WithBufferSize(n int) BufferSizeOption { return BufferSizeOption{n: n} }

// IOWriterOutputStreamOption configures an IOWriterOutputStream.
type IOWriterOutputStreamOption interface {
	applyToWriter(*ioWriterConfig)
}

type ioWriterConfig struct {
	bufferSize int
}

func (o BufferSizeOption) applyToWriter(c *ioWriterConfig) { c.bufferSize = o.n }

// IOWriterOutputStream adapts an io.Writer to the wasi:io/streams
// output-stream resource. Writes are staged in a single-producer,
// single-consumer ring buffer and drained on a background goroutine so
// write/flush calls return promptly per the spec's non-blocking
// semantics. Errors from the underlying writer close the stream; the
// first failure is reported as last-operation-failed and subsequent
// calls return closed.
//
// If the underlying writer implements Flusher, flush operations call
// Flush after the buffered bytes drain. If it implements io.Closer,
// Drop invokes Close after stopping the drainer.
//
// Construct with NewIOWriterOutputStream and wrap the result in
// NewOutputStreamHandle (or NewOutputStreamHandleIn) before passing it
// across a component boundary.
type IOWriterOutputStream struct {
	w         io.Writer
	errorInst *host.ComponentInstance
	pollInst  *host.ComponentInstance

	buf    []byte
	bufCap uint64

	tail atomic.Uint64 // producer writes; consumer reads
	head atomic.Uint64 // consumer writes; producer reads

	flushReq atomic.Bool // producer sets; consumer clears
	closed   atomic.Bool // set on terminal close (error or drop)
	stopReq  atomic.Bool // Drop sets; drainer observes

	wake            chan struct{} // producer → drainer doorbell (cap 1)
	activePollables atomic.Int64
	subscriptions   subscriptionSet

	lastErr atomic.Pointer[error]

	drainerWG sync.WaitGroup
}

// NewIOWriterOutputStream returns an output stream that writes to w.
// errorInst is the wasi:io/error component instance whose error
// resource type is referenced by stream-error::last-operation-failed;
// it is used to construct the error handle when the underlying writer
// fails. pollInst is the wasi:io/poll component instance whose pollable
// resource type is returned from Subscribe.
func NewIOWriterOutputStream(errorInst, pollInst *host.ComponentInstance, w io.Writer, opts ...IOWriterOutputStreamOption) *IOWriterOutputStream {
	cfg := ioWriterConfig{bufferSize: defaultIOWriterBufferSize}
	for _, opt := range opts {
		opt.applyToWriter(&cfg)
	}
	if cfg.bufferSize <= 0 {
		cfg.bufferSize = defaultIOWriterBufferSize
	}
	s := &IOWriterOutputStream{
		w:         w,
		errorInst: errorInst,
		pollInst:  pollInst,
		buf:       make([]byte, cfg.bufferSize),
		bufCap:    uint64(cfg.bufferSize),
		wake:      make(chan struct{}, 1),
	}
	s.drainerWG.Add(1)
	go s.drain()
	return s
}

// Drop stops the drainer, invalidates every pollable derived from the
// stream, and, if the underlying writer is an io.Closer, closes it. The
// spec permits losing in-flight data on drop, so we do not implicitly
// flush. Per spec, every derived pollable must be dropped first;
// calling Drop while any remain returns an error and leaves the stream
// untouched. Use DestroyOrphan for orphan-time cleanup that bypasses
// this check.
func (s *IOWriterOutputStream) Drop(_ context.Context) error {
	if s.activePollables.Load() > 0 {
		return fmt.Errorf("wasi:io/streams: dropping output stream with active pollables")
	}
	return s.destroy()
}

// DestroyOrphan tears down the stream regardless of any outstanding
// pollables. Invoked from instance close drain when the guest never
// followed the drop protocol.
func (s *IOWriterOutputStream) DestroyOrphan(_ context.Context) error {
	return s.destroy()
}

func (s *IOWriterOutputStream) destroy() error {
	s.stopReq.Store(true)
	s.closed.Store(true)
	s.signal(s.wake)
	s.drainerWG.Wait()
	if c, ok := s.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (s *IOWriterOutputStream) signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (s *IOWriterOutputStream) drain() {
	defer s.drainerWG.Done()
	flusher, _ := s.w.(Flusher)
	for {
		if s.stopReq.Load() {
			return
		}
		head := s.head.Load()
		tail := s.tail.Load()
		if head < tail {
			startIdx := head % s.bufCap
			avail := tail - head
			n := avail
			if startIdx+n > s.bufCap {
				n = s.bufCap - startIdx
			}
			chunk := s.buf[startIdx : startIdx+n]
			wn, err := s.w.Write(chunk)
			if err != nil {
				s.failWith(err)
				return
			}
			if uint64(wn) < n {
				s.failWith(io.ErrShortWrite)
				return
			}
			s.head.Store(head + n)
			s.subscriptions.notify()
			continue
		}
		if s.flushReq.Load() {
			if flusher != nil {
				if err := flusher.Flush(); err != nil {
					s.failWith(err)
					return
				}
			}
			s.flushReq.Store(false)
			s.subscriptions.notify()
			continue
		}
		<-s.wake
	}
}

func (s *IOWriterOutputStream) failWith(err error) {
	s.lastErr.Store(&err)
	s.closed.Store(true)
	s.subscriptions.notify()
}

// takeStreamError returns the StreamError to report to the caller. The
// captured underlying-writer error is consumed once and reported as
// last-operation-failed; subsequent calls observe a plain Closed.
func (s *IOWriterOutputStream) takeStreamError() StreamError {
	if errp := s.lastErr.Swap(nil); errp != nil {
		return StreamErrorLastOperationFailed{
			Value: wioerror.NewErrorResourceHandleIn(s.errorInst, errorString((*errp).Error())),
		}
	}
	return StreamErrorClosed{}
}

func (s *IOWriterOutputStream) freeNow() uint64 {
	if s.flushReq.Load() {
		return 0
	}
	return s.bufCap - (s.tail.Load() - s.head.Load())
}

func (s *IOWriterOutputStream) copyIn(contents []byte) {
	tail := s.tail.Load()
	n := uint64(len(contents))
	startIdx := tail % s.bufCap
	if startIdx+n <= s.bufCap {
		copy(s.buf[startIdx:], contents)
	} else {
		first := s.bufCap - startIdx
		copy(s.buf[startIdx:], contents[:first])
		copy(s.buf, contents[first:])
	}
	s.tail.Store(tail + n)
}

func (s *IOWriterOutputStream) zeroIn(n uint64) {
	tail := s.tail.Load()
	startIdx := tail % s.bufCap
	if startIdx+n <= s.bufCap {
		clear(s.buf[startIdx : startIdx+n])
	} else {
		first := s.bufCap - startIdx
		clear(s.buf[startIdx:])
		clear(s.buf[:n-first])
	}
	s.tail.Store(tail + n)
}

func (s *IOWriterOutputStream) CheckWrite(_ context.Context) (ResultU64StreamError, error) {
	if s.closed.Load() {
		return ResultU64StreamErrorErr{Value: s.takeStreamError()}, nil
	}
	return ResultU64StreamErrorOk{Value: s.freeNow()}, nil
}

func (s *IOWriterOutputStream) Write(_ context.Context, contents []uint8) (Result_StreamError, error) {
	if s.closed.Load() {
		return Result_StreamErrorErr{Value: s.takeStreamError()}, nil
	}
	if uint64(len(contents)) > s.freeNow() {
		return nil, fmt.Errorf("wasi:io/streams.output-stream.write: contents exceed check-write permit")
	}
	s.copyIn(contents)
	s.signal(s.wake)
	return Result_StreamErrorOk{}, nil
}

func (s *IOWriterOutputStream) WriteZeroes(_ context.Context, n uint64) (Result_StreamError, error) {
	if s.closed.Load() {
		return Result_StreamErrorErr{Value: s.takeStreamError()}, nil
	}
	if n > s.freeNow() {
		return nil, fmt.Errorf("wasi:io/streams.output-stream.write-zeroes: count exceeds check-write permit")
	}
	s.zeroIn(n)
	s.signal(s.wake)
	return Result_StreamErrorOk{}, nil
}

func (s *IOWriterOutputStream) Flush(_ context.Context) (Result_StreamError, error) {
	if s.closed.Load() {
		return Result_StreamErrorErr{Value: s.takeStreamError()}, nil
	}
	s.flushReq.Store(true)
	s.signal(s.wake)
	return Result_StreamErrorOk{}, nil
}

// readyForWrite reports whether the stream would accept at least one
// byte of write right now. Closed streams report ready (per the
// pollable spec); pending flushes report not-ready until the drainer
// catches up.
func (s *IOWriterOutputStream) readyForWrite() bool {
	if s.closed.Load() {
		return true
	}
	if s.flushReq.Load() {
		return false
	}
	return s.freeNow() > 0
}

func (s *IOWriterOutputStream) BlockingFlush(_ context.Context) (Result_StreamError, error) {
	if s.closed.Load() {
		return Result_StreamErrorErr{Value: s.takeStreamError()}, nil
	}
	s.flushReq.Store(true)
	s.signal(s.wake)
	<-s.subscriptions.subscribe(s.readyForWrite)
	if s.closed.Load() {
		return Result_StreamErrorErr{Value: s.takeStreamError()}, nil
	}
	return Result_StreamErrorOk{}, nil
}

// blockingWriteAndFlush stages remaining bytes (up to blockingWriteCap)
// into the ring in chunks sized to whatever the buffer can absorb,
// waiting between chunks, then issues a flush and waits for it to
// complete. Caller supplies a stage func that copies n bytes starting
// from offset off into the ring; this lets the same routine handle
// both byte slices and zero-fill.
func (s *IOWriterOutputStream) blockingWriteAndFlush(total uint64, stage func(off, n uint64)) (Result_StreamError, error) {
	if s.closed.Load() {
		return Result_StreamErrorErr{Value: s.takeStreamError()}, nil
	}
	off := uint64(0)
	for off < total {
		if !s.readyForWrite() {
			<-s.subscriptions.subscribe(s.readyForWrite)
		}
		n := total - off
		free := s.freeNow()
		if n > free {
			n = free
		}
		stage(off, n)
		s.signal(s.wake)
		off += n
	}
	s.flushReq.Store(true)
	s.signal(s.wake)
	<-s.subscriptions.subscribe(s.readyForWrite)
	return Result_StreamErrorOk{}, nil
}

func (s *IOWriterOutputStream) BlockingWriteAndFlush(_ context.Context, contents []uint8) (Result_StreamError, error) {
	if len(contents) > blockingWriteCap {
		return nil, fmt.Errorf("wasi:io/streams.output-stream.blocking-write-and-flush: contents exceed 4096 bytes")
	}
	return s.blockingWriteAndFlush(uint64(len(contents)), func(off, n uint64) {
		s.copyIn(contents[off : off+n])
	})
}

func (s *IOWriterOutputStream) BlockingWriteZeroesAndFlush(_ context.Context, n uint64) (Result_StreamError, error) {
	if n > blockingWriteCap {
		return nil, fmt.Errorf("wasi:io/streams.output-stream.blocking-write-zeroes-and-flush: count exceeds 4096")
	}
	return s.blockingWriteAndFlush(n, func(_, k uint64) {
		s.zeroIn(k)
	})
}

func (s *IOWriterOutputStream) Splice(ctx context.Context, src *InputStreamHandle, n uint64) (ResultU64StreamError, error) {
	if s.closed.Load() {
		return ResultU64StreamErrorErr{Value: s.takeStreamError()}, nil
	}
	free := s.freeNow()
	if free == 0 {
		return ResultU64StreamErrorOk{Value: 0}, nil
	}
	if n > free {
		n = free
	}
	readRes, err := src.Read(ctx, n)
	if err != nil {
		return nil, err
	}
	return s.spliceWriteResult(ctx, readRes, "splice")
}

func (s *IOWriterOutputStream) BlockingSplice(ctx context.Context, src *InputStreamHandle, n uint64) (ResultU64StreamError, error) {
	if s.closed.Load() {
		return ResultU64StreamErrorErr{Value: s.takeStreamError()}, nil
	}
	if !s.readyForWrite() {
		<-s.subscriptions.subscribe(s.readyForWrite)
	}
	free := s.freeNow()
	if n > free {
		n = free
	}
	readRes, err := src.BlockingRead(ctx, n)
	if err != nil {
		return nil, err
	}
	return s.spliceWriteResult(ctx, readRes, "blocking-splice")
}

func (s *IOWriterOutputStream) spliceWriteResult(ctx context.Context, readRes ResultListU8StreamError, op string) (ResultU64StreamError, error) {
	switch r := readRes.(type) {
	case ResultListU8StreamErrorErr:
		return ResultU64StreamErrorErr{Value: r.Value}, nil
	case ResultListU8StreamErrorOk:
		if len(r.Value) == 0 {
			return ResultU64StreamErrorOk{Value: 0}, nil
		}
		writeRes, err := s.Write(ctx, r.Value)
		if err != nil {
			return nil, err
		}
		if w, ok := writeRes.(Result_StreamErrorErr); ok {
			return ResultU64StreamErrorErr{Value: w.Value}, nil
		}
		return ResultU64StreamErrorOk{Value: uint64(len(r.Value))}, nil
	default:
		return nil, fmt.Errorf("wasi:io/streams.output-stream.%s: unexpected read result", op)
	}
}

// Subscribe returns a fresh pollable that resolves when the stream is
// ready for at least one byte of write or has been closed. Each call
// returns an independent pollable. The stream tracks every pollable it
// hands out and invalidates them on Drop.
func (s *IOWriterOutputStream) Subscribe(_ context.Context) (*poll.PollableHandle, error) {
	p := &outputPollable{stream: s}
	s.activePollables.Add(1)
	return poll.NewPollableHandleIn(s.pollInst, p), nil
}

// outputPollable backs each pollable handed out by Subscribe. It holds
// only a back-reference to the stream and a sticky dropped flag; all
// blocking is done against the stream's cond.
type outputPollable struct {
	stream  *IOWriterOutputStream
	dropped bool
}

func (p *outputPollable) Done() <-chan struct{} {
	return p.stream.subscriptions.subscribe(p.stream.readyForWrite)
}

func (p *outputPollable) Ready(_ context.Context) (bool, error) {
	if p.dropped {
		return false, fmt.Errorf("wasi:io/streams: pollable dropped")
	}
	return p.stream.readyForWrite(), nil
}

func (p *outputPollable) Block(_ context.Context) error {
	if p.dropped {
		return fmt.Errorf("wasi:io/streams: pollable dropped")
	}
	<-p.stream.subscriptions.subscribe(p.stream.readyForWrite)
	return nil
}

// Drop is invoked by the poll resource destructor when the wasm side
// drops the handle. It is also safe to call from the stream's Drop;
// double-drop is a no-op.
func (p *outputPollable) Drop(_ context.Context) error {
	if p.dropped {
		return nil
	}
	p.dropped = true
	p.stream.activePollables.Add(-1)
	return nil
}

// errorString satisfies wioerror.ErrorResource by returning a captured
// debug message.
type errorString string

func (e errorString) ToDebugString(_ context.Context) (string, error) {
	return string(e), nil
}

var _ OutputStream = (*IOWriterOutputStream)(nil)
