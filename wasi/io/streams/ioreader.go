package streams

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/partite-ai/wacogo/host"
	"github.com/partite-ai/wacogo/wasi/io/poll"
	"github.com/partite-ai/wacogo/wasi/io/wioerror"
)

const defaultIOReaderBufferSize = 4 * 1024

// IOReaderInputStreamOption configures an IOReaderInputStream.
type IOReaderInputStreamOption interface {
	applyToReader(*ioReaderConfig)
}

type ioReaderConfig struct {
	bufferSize int
}

func (o BufferSizeOption) applyToReader(c *ioReaderConfig) { c.bufferSize = o.n }

// IOReaderInputStream adapts an io.Reader to the wasi:io/streams
// input-stream resource. A background pumper goroutine reads bytes
// from the underlying reader into a single-producer, single-consumer
// ring buffer; reads pull from the buffer so calls return promptly per
// the spec's non-blocking semantics. EOF closes the stream cleanly;
// non-EOF errors are reported once as last-operation-failed and
// subsequent calls return closed.
//
// If the underlying reader implements io.Closer, Drop invokes Close
// after stopping the pumper.
//
// Construct with NewIOReaderInputStream and wrap the result in
// NewInputStreamHandle (or NewInputStreamHandleIn) before passing it
// across a component boundary.
type IOReaderInputStream struct {
	r         io.Reader
	errorInst *host.ComponentInstance
	pollInst  *host.ComponentInstance

	buf    []byte
	bufCap uint64

	head atomic.Uint64 // consumer writes; pumper reads
	tail atomic.Uint64 // pumper writes; consumer reads

	closed  atomic.Bool // EOF, error, or drop
	stopReq atomic.Bool // Drop sets; pumper observes

	wake            chan struct{} // consumer → pumper doorbell (cap 1)
	activePollables atomic.Int64
	subscriptions   subscriptionSet
	lastErr         atomic.Pointer[error]

	pumperWG sync.WaitGroup
}

// NewIOReaderInputStream returns an input stream that reads from r.
// errorInst is the wasi:io/error component instance whose error
// resource type is referenced by stream-error::last-operation-failed;
// it is used to construct the error handle when the underlying reader
// fails. pollInst is the wasi:io/poll component instance whose
// pollable resource type is returned from Subscribe.
func NewIOReaderInputStream(errorInst, pollInst *host.ComponentInstance, r io.Reader, opts ...IOReaderInputStreamOption) *IOReaderInputStream {
	cfg := ioReaderConfig{bufferSize: defaultIOReaderBufferSize}
	for _, opt := range opts {
		opt.applyToReader(&cfg)
	}
	if cfg.bufferSize <= 0 {
		cfg.bufferSize = defaultIOReaderBufferSize
	}
	s := &IOReaderInputStream{
		r:         r,
		errorInst: errorInst,
		pollInst:  pollInst,
		buf:       make([]byte, cfg.bufferSize),
		bufCap:    uint64(cfg.bufferSize),
		wake:      make(chan struct{}, 1),
	}
	s.pumperWG.Add(1)
	go s.pump()
	return s
}

// Drop stops the pumper, invalidates every pollable derived from the
// stream, and, if the underlying reader is an io.Closer, closes it.
// Bytes still in the buffer are discarded.
func (s *IOReaderInputStream) Drop() {
	if s.activePollables.Load() > 0 {
		panic("wasi:io/streams: dropping input stream with active pollables")
	}
	s.stopReq.Store(true)
	s.closed.Store(true)

	s.signal(s.wake)
	s.pumperWG.Wait()
	if c, ok := s.r.(io.Closer); ok {
		_ = c.Close()
	}
}

func (s *IOReaderInputStream) signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// pump fills the ring buffer from the underlying reader until the
// stream is dropped, EOF arrives, or a non-EOF error is observed.
func (s *IOReaderInputStream) pump() {
	defer s.pumperWG.Done()
	for {
		if s.stopReq.Load() {
			return
		}
		head := s.head.Load()
		tail := s.tail.Load()
		used := tail - head
		free := s.bufCap - used
		if free == 0 {
			<-s.wake
			continue
		}
		startIdx := tail % s.bufCap
		n := free
		if startIdx+n > s.bufCap {
			n = s.bufCap - startIdx
		}
		chunk := s.buf[startIdx : startIdx+n]
		rn, err := s.r.Read(chunk)
		if rn > 0 {
			s.tail.Store(tail + uint64(rn))
			s.subscriptions.notify()
		}
		if errors.Is(err, io.EOF) {
			s.closed.Store(true)
			s.subscriptions.notify()
			return
		}
		if err != nil {
			s.failWith(err)
			return
		}
	}
}

func (s *IOReaderInputStream) failWith(err error) {
	s.lastErr.Store(&err)
	s.closed.Store(true)
	s.subscriptions.notify()
}

// takeStreamError returns the StreamError to report to the caller.
// The captured underlying-reader error is consumed once and reported
// as last-operation-failed; subsequent calls observe a plain Closed.
func (s *IOReaderInputStream) takeStreamError() StreamError {
	if errp := s.lastErr.Swap(nil); errp != nil {
		return StreamErrorLastOperationFailed{
			Value: wioerror.NewErrorResourceHandleIn(s.errorInst, errorString((*errp).Error())),
		}
	}
	return StreamErrorClosed{}
}

func (s *IOReaderInputStream) availableNow() uint64 {
	return s.tail.Load() - s.head.Load()
}

// readyForRead reports whether a non-blocking read would return data
// or report a stream error. Closed streams are always ready.
func (s *IOReaderInputStream) readyForRead() bool {
	// Read closed first so a true value implies tail is current.
	if s.closed.Load() {
		return true
	}
	return s.availableNow() > 0
}

// copyOut returns a fresh copy of n bytes from the buffer head and
// advances head by n.
func (s *IOReaderInputStream) copyOut(n uint64) []byte {
	head := s.head.Load()
	out := make([]byte, n)
	startIdx := head % s.bufCap
	if startIdx+n <= s.bufCap {
		copy(out, s.buf[startIdx:startIdx+n])
	} else {
		first := s.bufCap - startIdx
		copy(out[:first], s.buf[startIdx:])
		copy(out[first:], s.buf[:n-first])
	}
	s.head.Store(head + n)
	return out
}

// dropOut advances head by n without copying — used by Skip.
func (s *IOReaderInputStream) dropOut(n uint64) {
	s.head.Store(s.head.Load() + n)
}

// waitReadable blocks the consumer until at least one byte is
// available or the stream becomes closed and empty. Returns 0 only
// when the stream is closed with nothing buffered.
func (s *IOReaderInputStream) waitReadable() uint64 {
	if s.readyForRead() {
		return s.availableNow()
	}

	<-s.subscriptions.subscribe(s.readyForRead)
	return s.availableNow()
}

func (s *IOReaderInputStream) Read(_ context.Context, n uint64) (ResultListU8StreamError, error) {
	closed := s.closed.Load()
	avail := s.availableNow()
	if avail == 0 {
		if closed {
			return ResultListU8StreamErrorErr{Value: s.takeStreamError()}, nil
		}
		return ResultListU8StreamErrorOk{Value: nil}, nil
	}
	if n > avail {
		n = avail
	}
	out := s.copyOut(n)
	s.signal(s.wake)
	return ResultListU8StreamErrorOk{Value: out}, nil
}

func (s *IOReaderInputStream) BlockingRead(ctx context.Context, n uint64) (ResultListU8StreamError, error) {
	if n == 0 {
		return s.Read(ctx, 0)
	}
	avail := s.waitReadable()
	if avail == 0 {
		return ResultListU8StreamErrorErr{Value: s.takeStreamError()}, nil
	}
	if n > avail {
		n = avail
	}
	out := s.copyOut(n)
	s.signal(s.wake)
	return ResultListU8StreamErrorOk{Value: out}, nil
}

func (s *IOReaderInputStream) Skip(_ context.Context, n uint64) (ResultU64StreamError, error) {
	closed := s.closed.Load()
	avail := s.availableNow()
	if avail == 0 {
		if closed {
			return ResultU64StreamErrorErr{Value: s.takeStreamError()}, nil
		}
		return ResultU64StreamErrorOk{Value: 0}, nil
	}
	if n > avail {
		n = avail
	}
	s.dropOut(n)
	s.signal(s.wake)
	return ResultU64StreamErrorOk{Value: n}, nil
}

func (s *IOReaderInputStream) BlockingSkip(ctx context.Context, n uint64) (ResultU64StreamError, error) {
	if n == 0 {
		return s.Skip(ctx, 0)
	}
	avail := s.waitReadable()
	if avail == 0 {
		return ResultU64StreamErrorErr{Value: s.takeStreamError()}, nil
	}
	if n > avail {
		n = avail
	}
	s.dropOut(n)
	s.signal(s.wake)
	return ResultU64StreamErrorOk{Value: n}, nil
}

// Subscribe returns a fresh pollable that resolves when the stream
// has at least one byte available or has been closed. Each call
// returns an independent pollable. The stream tracks every pollable
// it hands out and invalidates them on Drop.
func (s *IOReaderInputStream) Subscribe(_ context.Context) (*poll.PollableHandle, error) {
	s.activePollables.Add(1)
	p := &inputPollable{stream: s}
	return poll.NewPollableHandleIn(s.pollInst, p), nil
}

// inputPollable backs each pollable handed out by Subscribe. It holds
// only a back-reference to the stream and a sticky dropped flag; all
// blocking is done against the stream's cond.
type inputPollable struct {
	stream  *IOReaderInputStream
	dropped bool
}

func (p *inputPollable) Done() <-chan struct{} {
	return p.stream.subscriptions.subscribe(p.stream.readyForRead)
}

func (p *inputPollable) Ready(_ context.Context) (bool, error) {
	if p.dropped {
		return false, fmt.Errorf("wasi:io/streams: pollable dropped")
	}
	return p.stream.readyForRead(), nil
}

func (p *inputPollable) Block(_ context.Context) error {
	if p.dropped {
		return fmt.Errorf("wasi:io/streams: pollable dropped")
	}
	if p.stream.readyForRead() {
		return nil
	}
	<-p.stream.subscriptions.subscribe(p.stream.readyForRead)
	return nil
}

// Drop is invoked by the poll resource destructor when the wasm side
// drops the handle.
func (p *inputPollable) Drop() {
	if p.dropped {
		return
	}
	p.dropped = true
	p.stream.activePollables.Add(-1)
}

var _ InputStream = (*IOReaderInputStream)(nil)
