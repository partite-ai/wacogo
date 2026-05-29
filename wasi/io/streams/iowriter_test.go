package streams

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/partite-ai/wacogo/wasi/io/poll"
)

var bgCtx = context.Background()

// === test doubles ===

// blockingWriter blocks each Write until release is closed. After
// release, it accumulates writes into buf. A mutex guards buf so the
// test goroutine can read it after the drainer has settled.
type blockingWriter struct {
	release  chan struct{}
	mu       sync.Mutex
	buf      bytes.Buffer
	writeErr error
}

func newBlockingWriter() *blockingWriter {
	return &blockingWriter{release: make(chan struct{})}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	<-w.release
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *blockingWriter) Release() { close(w.release) }

func (w *blockingWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// flushyWriter satisfies Flusher and counts Flush calls.
type flushyWriter struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	flushes  int
	flushErr error
}

func (w *flushyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *flushyWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushes++
	return w.flushErr
}

func (w *flushyWriter) Flushes() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flushes
}

func (w *flushyWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// closingWriter satisfies io.Closer and counts Close calls.
type closingWriter struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	closes   int
	closeErr error
}

func (w *closingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *closingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closes++
	return w.closeErr
}

func (w *closingWriter) Closes() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closes
}

// errorWriter returns the same error from every Write.
type errorWriter struct {
	err error
}

func (w *errorWriter) Write(_ []byte) (int, error) { return 0, w.err }

// shortWriter writes only the first short bytes of each Write call.
type shortWriter struct {
	short int
}

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) > w.short {
		return w.short, nil
	}
	return len(p), nil
}

// sliceInputStream serves bytes from an in-memory slice. blockBlocking,
// if set, gates BlockingRead until released.
type sliceInputStream struct {
	mu            sync.Mutex
	data          []byte
	off           int
	blockBlocking chan struct{}
}

func (s *sliceInputStream) Read(_ context.Context, n uint64) (ResultListU8StreamError, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rem := uint64(len(s.data) - s.off)
	if rem == 0 {
		return ResultListU8StreamErrorOk{Value: nil}, nil
	}
	if n > rem {
		n = rem
	}
	chunk := append([]byte(nil), s.data[s.off:s.off+int(n)]...)
	s.off += int(n)
	return ResultListU8StreamErrorOk{Value: chunk}, nil
}

func (s *sliceInputStream) BlockingRead(ctx context.Context, n uint64) (ResultListU8StreamError, error) {
	if s.blockBlocking != nil {
		<-s.blockBlocking
	}
	return s.Read(ctx, n)
}

func (s *sliceInputStream) Skip(_ context.Context, _ uint64) (ResultU64StreamError, error) {
	return ResultU64StreamErrorOk{Value: 0}, nil
}

func (s *sliceInputStream) BlockingSkip(_ context.Context, _ uint64) (ResultU64StreamError, error) {
	return ResultU64StreamErrorOk{Value: 0}, nil
}

func (s *sliceInputStream) Subscribe(_ context.Context) (*poll.PollableHandle, error) {
	return nil, nil
}

// closedInputStream always reports a closed stream-error on read.
type closedInputStream struct{}

func (closedInputStream) Read(_ context.Context, _ uint64) (ResultListU8StreamError, error) {
	return ResultListU8StreamErrorErr{Value: StreamErrorClosed{}}, nil
}

func (closedInputStream) BlockingRead(_ context.Context, _ uint64) (ResultListU8StreamError, error) {
	return ResultListU8StreamErrorErr{Value: StreamErrorClosed{}}, nil
}

func (closedInputStream) Skip(_ context.Context, _ uint64) (ResultU64StreamError, error) {
	return ResultU64StreamErrorErr{Value: StreamErrorClosed{}}, nil
}

func (closedInputStream) BlockingSkip(_ context.Context, _ uint64) (ResultU64StreamError, error) {
	return ResultU64StreamErrorErr{Value: StreamErrorClosed{}}, nil
}

func (closedInputStream) Subscribe(_ context.Context) (*poll.PollableHandle, error) {
	return nil, nil
}

// === assertion helpers ===

func okWrite(t *testing.T, s *IOWriterOutputStream, p []byte) {
	t.Helper()
	res, err := s.Write(bgCtx, p)
	if err != nil {
		t.Fatalf("Write: unexpected error: %v", err)
	}
	if _, ok := res.(Result_StreamErrorOk); !ok {
		t.Fatalf("Write: expected Ok, got %T", res)
	}
}

func okFlush(t *testing.T, s *IOWriterOutputStream) {
	t.Helper()
	res, err := s.Flush(bgCtx)
	if err != nil {
		t.Fatalf("Flush: unexpected error: %v", err)
	}
	if _, ok := res.(Result_StreamErrorOk); !ok {
		t.Fatalf("Flush: expected Ok, got %T", res)
	}
}

func okBlockingFlush(t *testing.T, s *IOWriterOutputStream) {
	t.Helper()
	res, err := s.BlockingFlush(bgCtx)
	if err != nil {
		t.Fatalf("BlockingFlush: unexpected error: %v", err)
	}
	if _, ok := res.(Result_StreamErrorOk); !ok {
		t.Fatalf("BlockingFlush: expected Ok, got %T", res)
	}
}

func okBlockingWriteAndFlush(t *testing.T, s *IOWriterOutputStream, p []byte) {
	t.Helper()
	res, err := s.BlockingWriteAndFlush(bgCtx, p)
	if err != nil {
		t.Fatalf("BlockingWriteAndFlush: unexpected error: %v", err)
	}
	if _, ok := res.(Result_StreamErrorOk); !ok {
		t.Fatalf("BlockingWriteAndFlush: expected Ok, got %T", res)
	}
}

func okBlockingWriteZeroesAndFlush(t *testing.T, s *IOWriterOutputStream, n uint64) {
	t.Helper()
	res, err := s.BlockingWriteZeroesAndFlush(bgCtx, n)
	if err != nil {
		t.Fatalf("BlockingWriteZeroesAndFlush: unexpected error: %v", err)
	}
	if _, ok := res.(Result_StreamErrorOk); !ok {
		t.Fatalf("BlockingWriteZeroesAndFlush: expected Ok, got %T", res)
	}
}

func okCheckWrite(t *testing.T, s *IOWriterOutputStream) uint64 {
	t.Helper()
	res, err := s.CheckWrite(bgCtx)
	if err != nil {
		t.Fatalf("CheckWrite: unexpected error: %v", err)
	}
	ok, isOk := res.(ResultU64StreamErrorOk)
	if !isOk {
		t.Fatalf("CheckWrite: expected Ok, got %T", res)
	}
	return ok.Value
}

func errWrite(t *testing.T, s *IOWriterOutputStream, p []byte) StreamError {
	t.Helper()
	res, err := s.Write(bgCtx, p)
	if err != nil {
		t.Fatalf("Write: unexpected host error: %v", err)
	}
	e, ok := res.(Result_StreamErrorErr)
	if !ok {
		t.Fatalf("Write: expected Err, got %T", res)
	}
	return e.Value
}

func errCheckWrite(t *testing.T, s *IOWriterOutputStream) StreamError {
	t.Helper()
	res, err := s.CheckWrite(bgCtx)
	if err != nil {
		t.Fatalf("CheckWrite: unexpected host error: %v", err)
	}
	e, ok := res.(ResultU64StreamErrorErr)
	if !ok {
		t.Fatalf("CheckWrite: expected Err, got %T", res)
	}
	return e.Value
}

func errBlockingFlush(t *testing.T, s *IOWriterOutputStream) StreamError {
	t.Helper()
	res, err := s.BlockingFlush(bgCtx)
	if err != nil {
		t.Fatalf("BlockingFlush: unexpected host error: %v", err)
	}
	e, ok := res.(Result_StreamErrorErr)
	if !ok {
		t.Fatalf("BlockingFlush: expected Err, got %T", res)
	}
	return e.Value
}

func okSplice(t *testing.T, s *IOWriterOutputStream, in *InputStreamHandle, n uint64) uint64 {
	t.Helper()
	res, err := s.Splice(bgCtx, in, n)
	if err != nil {
		t.Fatalf("Splice: unexpected error: %v", err)
	}
	ok, isOk := res.(ResultU64StreamErrorOk)
	if !isOk {
		t.Fatalf("Splice: expected Ok, got %T", res)
	}
	return ok.Value
}

func errSplice(t *testing.T, s *IOWriterOutputStream, in *InputStreamHandle, n uint64) StreamError {
	t.Helper()
	res, err := s.Splice(bgCtx, in, n)
	if err != nil {
		t.Fatalf("Splice: unexpected host error: %v", err)
	}
	e, ok := res.(ResultU64StreamErrorErr)
	if !ok {
		t.Fatalf("Splice: expected Err, got %T", res)
	}
	return e.Value
}

func okBlockingSplice(t *testing.T, s *IOWriterOutputStream, in *InputStreamHandle, n uint64) uint64 {
	t.Helper()
	res, err := s.BlockingSplice(bgCtx, in, n)
	if err != nil {
		t.Fatalf("BlockingSplice: unexpected error: %v", err)
	}
	ok, isOk := res.(ResultU64StreamErrorOk)
	if !isOk {
		t.Fatalf("BlockingSplice: expected Ok, got %T", res)
	}
	return ok.Value
}

// === tests ===

func TestNewStreamCheckWriteReturnsCapacity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest, WithBufferSize(128))
		defer s.Drop(context.Background())
		if got := okCheckWrite(t, s); got != 128 {
			t.Fatalf("CheckWrite = %d, want 128", got)
		}
	})
}

func TestDefaultBufferSize(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest)
		defer s.Drop(context.Background())
		if got := okCheckWrite(t, s); got != defaultIOWriterBufferSize {
			t.Fatalf("CheckWrite = %d, want %d", got, defaultIOWriterBufferSize)
		}
	})
}

func TestZeroOrNegativeBufferSizeFallsBack(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest, WithBufferSize(0))
		defer s.Drop(context.Background())
		if got := okCheckWrite(t, s); got != defaultIOWriterBufferSize {
			t.Fatalf("CheckWrite = %d, want %d", got, defaultIOWriterBufferSize)
		}
	})
}

func TestWriteIsDrainedToUnderlying(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest)
		defer s.Drop(context.Background())
		okWrite(t, s, []byte("hello"))
		synctest.Wait()
		if dest.String() != "hello" {
			t.Fatalf("dest = %q, want %q", dest.String(), "hello")
		}
	})
}

func TestMultipleWritesPreserveOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest, WithBufferSize(64))
		defer s.Drop(context.Background())
		for _, c := range []string{"alpha-", "beta-", "gamma"} {
			okWrite(t, s, []byte(c))
		}
		synctest.Wait()
		if dest.String() != "alpha-beta-gamma" {
			t.Fatalf("dest = %q", dest.String())
		}
	})
}

func TestWriteOverPermitTraps(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest, WithBufferSize(4))
		defer s.Drop(context.Background())
		_, err := s.Write(bgCtx, []byte("hello"))
		if err == nil {
			t.Fatal("expected trap error from over-permit Write")
		}
	})
}

func TestWriteZeroesOverPermitTraps(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest, WithBufferSize(4))
		defer s.Drop(context.Background())
		_, err := s.WriteZeroes(bgCtx, 5)
		if err == nil {
			t.Fatal("expected trap error from over-permit WriteZeroes")
		}
	})
}

func TestCheckWriteShrinksWhileDrainerBlocked(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bw := newBlockingWriter()
		s := NewIOWriterOutputStream(nil, nil, bw, WithBufferSize(16))
		defer func() {
			bw.Release()
			s.Drop(context.Background())
		}()
		okWrite(t, s, []byte("hello"))
		synctest.Wait() // drainer is blocked inside bw.Write
		got := okCheckWrite(t, s)
		// Drainer is mid-write; head not yet advanced so 5 bytes still
		// pinned in the ring. free = cap - 5 = 11.
		if got != 11 {
			t.Fatalf("CheckWrite = %d, want 11", got)
		}
	})
}

func TestCheckWriteReturnsZeroWhileFlushPending(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bw := newBlockingWriter()
		s := NewIOWriterOutputStream(nil, nil, bw, WithBufferSize(16))
		defer func() {
			bw.Release()
			s.Drop(context.Background())
		}()
		okWrite(t, s, []byte("hi"))
		okFlush(t, s)
		synctest.Wait()
		if got := okCheckWrite(t, s); got != 0 {
			t.Fatalf("CheckWrite during pending flush = %d, want 0", got)
		}
	})
}

func TestFlushInvokesUnderlyingFlusher(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fw := &flushyWriter{}
		s := NewIOWriterOutputStream(nil, nil, fw)
		defer s.Drop(context.Background())
		okWrite(t, s, []byte("data"))
		okFlush(t, s)
		synctest.Wait()
		if fw.String() != "data" {
			t.Fatalf("Flushed data = %q, want %q", fw.String(), "data")
		}
		if fw.Flushes() != 1 {
			t.Fatalf("Flush calls = %d, want 1", fw.Flushes())
		}
	})
}

func TestFlushNoopOnPlainWriter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest)
		defer s.Drop(context.Background())
		okWrite(t, s, []byte("x"))
		okFlush(t, s)
		synctest.Wait()
		if dest.String() != "x" {
			t.Fatalf("dest = %q", dest.String())
		}
		if got := okCheckWrite(t, s); got != defaultIOWriterBufferSize {
			t.Fatalf("post-flush CheckWrite = %d", got)
		}
	})
}

func TestBlockingFlushWaitsForDrainAndFlush(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fw := &flushyWriter{}
		s := NewIOWriterOutputStream(nil, nil, fw)
		defer s.Drop(context.Background())
		okWrite(t, s, []byte("payload"))
		okBlockingFlush(t, s)
		// On return the buffer must be drained and Flush called once.
		if fw.String() != "payload" {
			t.Fatalf("post-blocking-flush data = %q", fw.String())
		}
		if fw.Flushes() != 1 {
			t.Fatalf("Flush calls = %d, want 1", fw.Flushes())
		}
		if got := okCheckWrite(t, s); got != defaultIOWriterBufferSize {
			t.Fatalf("post-blocking-flush CheckWrite = %d", got)
		}
	})
}

func TestBlockingWriteAndFlushSmallBufferChunks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fw := &flushyWriter{}
		s := NewIOWriterOutputStream(nil, nil, fw, WithBufferSize(8))
		defer s.Drop(context.Background())
		payload := []byte("hello world this is a test payload")
		okBlockingWriteAndFlush(t, s, payload)
		if fw.String() != string(payload) {
			t.Fatalf("got %q, want %q", fw.String(), string(payload))
		}
		if fw.Flushes() != 1 {
			t.Fatalf("Flush calls = %d, want 1", fw.Flushes())
		}
	})
}

func TestBlockingWriteAndFlushOver4096Traps(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest, WithBufferSize(8192))
		defer s.Drop(context.Background())
		_, err := s.BlockingWriteAndFlush(bgCtx, make([]byte, 4097))
		if err == nil {
			t.Fatal("expected trap for >4096 bytes")
		}
	})
}

func TestBlockingWriteZeroesAndFlush(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fw := &flushyWriter{}
		s := NewIOWriterOutputStream(nil, nil, fw, WithBufferSize(8))
		defer s.Drop(context.Background())
		okBlockingWriteZeroesAndFlush(t, s, 20)
		got := []byte(fw.String())
		if len(got) != 20 {
			t.Fatalf("len = %d, want 20", len(got))
		}
		for i, b := range got {
			if b != 0 {
				t.Fatalf("byte %d = %d, want 0", i, b)
			}
		}
		if fw.Flushes() != 1 {
			t.Fatalf("Flush calls = %d, want 1", fw.Flushes())
		}
	})
}

func TestBlockingWriteZeroesAndFlushOver4096Traps(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest, WithBufferSize(8192))
		defer s.Drop(context.Background())
		_, err := s.BlockingWriteZeroesAndFlush(bgCtx, 4097)
		if err == nil {
			t.Fatal("expected trap for >4096 zeroes")
		}
	})
}

func TestRingWrapWritesAreContiguousAtSink(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fw := &flushyWriter{}
		s := NewIOWriterOutputStream(nil, nil, fw, WithBufferSize(8))
		defer s.Drop(context.Background())
		okBlockingWriteAndFlush(t, s, []byte("AAAABBBB")) // 8 bytes, exact fill
		okBlockingWriteAndFlush(t, s, []byte("CCCCDDDD")) // wraps
		okBlockingWriteAndFlush(t, s, []byte("EE"))       // partial slot
		want := "AAAABBBBCCCCDDDDEE"
		if fw.String() != want {
			t.Fatalf("got %q, want %q", fw.String(), want)
		}
	})
}

func TestWriteErrorClosesStreamAndReportsOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ew := &errorWriter{err: errors.New("boom")}
		s := NewIOWriterOutputStream(nil, nil, ew)
		defer s.Drop(context.Background())
		okWrite(t, s, []byte("hi"))
		synctest.Wait()

		first := errWrite(t, s, []byte("x"))
		lof, ok := first.(StreamErrorLastOperationFailed)
		if !ok {
			t.Fatalf("expected last-operation-failed, got %T", first)
		}
		msg, _ := lof.Value.ToDebugString(bgCtx)
		if msg != "boom" {
			t.Fatalf("debug string = %q, want %q", msg, "boom")
		}

		second := errWrite(t, s, []byte("y"))
		if _, ok := second.(StreamErrorClosed); !ok {
			t.Fatalf("expected closed, got %T", second)
		}
		third := errCheckWrite(t, s)
		if _, ok := third.(StreamErrorClosed); !ok {
			t.Fatalf("expected closed from CheckWrite, got %T", third)
		}
	})
}

func TestShortWriteIsTreatedAsError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewIOWriterOutputStream(nil, nil, &shortWriter{short: 1})
		defer s.Drop(context.Background())
		okWrite(t, s, []byte("ab"))
		synctest.Wait()
		first := errWrite(t, s, []byte("z"))
		lof, ok := first.(StreamErrorLastOperationFailed)
		if !ok {
			t.Fatalf("expected last-operation-failed, got %T", first)
		}
		msg, _ := lof.Value.ToDebugString(bgCtx)
		if msg != io.ErrShortWrite.Error() {
			t.Fatalf("debug string = %q, want %q", msg, io.ErrShortWrite.Error())
		}
	})
}

func TestFlushErrorClosesStream(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fw := &flushyWriter{flushErr: errors.New("flushfail")}
		s := NewIOWriterOutputStream(nil, nil, fw)
		defer s.Drop(context.Background())
		okFlush(t, s)
		synctest.Wait()
		first := errCheckWrite(t, s)
		lof, ok := first.(StreamErrorLastOperationFailed)
		if !ok {
			t.Fatalf("expected last-operation-failed, got %T", first)
		}
		if msg, _ := lof.Value.ToDebugString(bgCtx); msg != "flushfail" {
			t.Fatalf("debug string = %q", msg)
		}
	})
}

func TestBlockingFlushReturnsErrorWhenWriteFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ew := &errorWriter{err: errors.New("nope")}
		s := NewIOWriterOutputStream(nil, nil, ew)
		defer s.Drop(context.Background())
		okWrite(t, s, []byte("x"))
		first := errBlockingFlush(t, s)
		if _, ok := first.(StreamErrorLastOperationFailed); !ok {
			t.Fatalf("expected last-operation-failed, got %T", first)
		}
	})
}

func TestSpliceCopiesBytes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest)
		defer s.Drop(context.Background())
		in := NewInputStreamHandle(&sliceInputStream{data: []byte("payload")})
		if got := okSplice(t, s, in, 7); got != 7 {
			t.Fatalf("splice returned %d, want 7", got)
		}
		okBlockingFlush(t, s)
		if dest.String() != "payload" {
			t.Fatalf("dest = %q", dest.String())
		}
	})
}

func TestSpliceLengthCappedByFreeSpace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bw := newBlockingWriter()
		s := NewIOWriterOutputStream(nil, nil, bw, WithBufferSize(4))
		defer func() {
			bw.Release()
			s.Drop(context.Background())
		}()
		in := NewInputStreamHandle(&sliceInputStream{data: []byte("abcdefghij")})
		if got := okSplice(t, s, in, 100); got != 4 {
			t.Fatalf("splice returned %d, want 4 (capped by buffer)", got)
		}
	})
}

func TestSpliceReturnsZeroWhenNoFreeSpace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bw := newBlockingWriter()
		s := NewIOWriterOutputStream(nil, nil, bw, WithBufferSize(4))
		defer func() {
			bw.Release()
			s.Drop(context.Background())
		}()
		// Fill the ring; drainer is blocked, so head won't advance.
		okWrite(t, s, []byte("xxxx"))
		synctest.Wait()
		in := NewInputStreamHandle(&sliceInputStream{data: []byte("more")})
		if got := okSplice(t, s, in, 4); got != 0 {
			t.Fatalf("splice returned %d, want 0", got)
		}
	})
}

func TestSpliceForwardsClosedFromInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest)
		defer s.Drop(context.Background())
		in := NewInputStreamHandle(closedInputStream{})
		err := errSplice(t, s, in, 4)
		if _, ok := err.(StreamErrorClosed); !ok {
			t.Fatalf("expected closed, got %T", err)
		}
	})
}

func TestBlockingSpliceCopiesBytes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest)
		defer s.Drop(context.Background())
		in := NewInputStreamHandle(&sliceInputStream{data: []byte("payload")})
		if got := okBlockingSplice(t, s, in, 7); got != 7 {
			t.Fatalf("blocking splice returned %d, want 7", got)
		}
		okBlockingFlush(t, s)
		if dest.String() != "payload" {
			t.Fatalf("dest = %q", dest.String())
		}
	})
}

func TestBlockingSpliceWaitsForRoom(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bw := newBlockingWriter()
		s := NewIOWriterOutputStream(nil, nil, bw, WithBufferSize(4))

		// Fill the buffer with the drainer gated.
		okWrite(t, s, []byte("xxxx"))
		synctest.Wait()

		in := NewInputStreamHandle(&sliceInputStream{data: []byte("ABCDE")})
		spliceDone := make(chan uint64, 1)
		go func() {
			res, err := s.BlockingSplice(bgCtx, in, 5)
			if err != nil {
				panic(err)
			}
			ok, isOk := res.(ResultU64StreamErrorOk)
			if !isOk {
				panic("BlockingSplice returned non-ok")
			}
			spliceDone <- ok.Value
		}()
		synctest.Wait()
		select {
		case <-spliceDone:
			t.Fatal("BlockingSplice returned before buffer freed")
		default:
		}

		bw.Release()
		got := <-spliceDone
		if got == 0 || got > 5 {
			t.Fatalf("BlockingSplice returned %d, want 1..5", got)
		}
		s.Drop(context.Background())
	})
}

func TestDropClosesIOCloser(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cw := &closingWriter{}
		s := NewIOWriterOutputStream(nil, nil, cw)
		okWrite(t, s, []byte("z"))
		s.Drop(context.Background())
		if cw.Closes() != 1 {
			t.Fatalf("Close calls = %d, want 1", cw.Closes())
		}
	})
}

func TestDropOnPlainWriterDoesNotPanic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest)
		s.Drop(context.Background())
	})
}

func TestDropStopsDrainer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest)
		s.Drop(context.Background())
		// If the drainer hadn't exited, the synctest bubble would panic
		// when this function returns.
	})
}

// === pollable helpers ===

func subscribe(t *testing.T, s *IOWriterOutputStream) *poll.PollableHandle {
	t.Helper()
	ph, err := s.Subscribe(bgCtx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if ph == nil {
		t.Fatal("Subscribe returned nil handle")
	}
	return ph
}

func pollableReady(t *testing.T, ph *poll.PollableHandle) bool {
	t.Helper()
	r, err := ph.Ready(bgCtx)
	if err != nil {
		t.Fatalf("Ready: %v", err)
	}
	return r
}

// === pollable tests ===

func TestSubscribeReadyOnFreshStream(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest, WithBufferSize(8))
		defer s.Drop(context.Background())
		ph := subscribe(t, s)
		defer ph.Drop(t.Context())
		if !pollableReady(t, ph) {
			t.Fatal("fresh stream pollable should be ready")
		}
	})
}

func TestSubscribeNotReadyWhenBufferFull(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bw := newBlockingWriter()
		s := NewIOWriterOutputStream(nil, nil, bw, WithBufferSize(4))
		defer func() { bw.Release(); s.Drop(context.Background()) }()
		okWrite(t, s, []byte("xxxx"))
		synctest.Wait()
		ph := subscribe(t, s)
		defer ph.Drop(t.Context())
		if pollableReady(t, ph) {
			t.Fatal("full-buffer pollable should not be ready")
		}
	})
}

func TestSubscribeNotReadyWhileFlushPending(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bw := newBlockingWriter()
		s := NewIOWriterOutputStream(nil, nil, bw, WithBufferSize(8))
		defer func() { bw.Release(); s.Drop(context.Background()) }()
		okWrite(t, s, []byte("hi"))
		okFlush(t, s)
		synctest.Wait()
		ph := subscribe(t, s)
		defer ph.Drop(t.Context())
		if pollableReady(t, ph) {
			t.Fatal("flush-pending pollable should not be ready")
		}
	})
}

func TestSubscribeReadyWhenStreamClosedByError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ew := &errorWriter{err: errors.New("boom")}
		s := NewIOWriterOutputStream(nil, nil, ew)
		defer s.Drop(context.Background())
		ph := subscribe(t, s)
		defer ph.Drop(t.Context())
		okWrite(t, s, []byte("x"))
		synctest.Wait()
		if !pollableReady(t, ph) {
			t.Fatal("closed-stream pollable should be ready")
		}
	})
}

func TestSubscribeAfterStreamClosedReturnsReadyPollable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ew := &errorWriter{err: errors.New("boom")}
		s := NewIOWriterOutputStream(nil, nil, ew)
		defer s.Drop(context.Background())
		okWrite(t, s, []byte("x"))
		synctest.Wait()
		// Stream is now closed. A new Subscribe should still succeed and
		// return a pollable that is immediately ready.
		ph := subscribe(t, s)
		defer ph.Drop(t.Context())
		if !pollableReady(t, ph) {
			t.Fatal("post-close subscribe should yield a ready pollable")
		}
	})
}

func TestPollableBlockReturnsImmediatelyWhenReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest)
		defer s.Drop(context.Background())
		ph := subscribe(t, s)
		defer ph.Drop(t.Context())
		if err := ph.Block(bgCtx); err != nil {
			t.Fatalf("Block: %v", err)
		}
	})
}

func TestPollableBlockWaitsForFreeSpace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bw := newBlockingWriter()
		s := NewIOWriterOutputStream(nil, nil, bw, WithBufferSize(4))
		okWrite(t, s, []byte("xxxx"))
		synctest.Wait()
		ph := subscribe(t, s)

		blocked := make(chan struct{})
		go func() {
			_ = ph.Block(bgCtx)
			close(blocked)
		}()
		synctest.Wait()
		select {
		case <-blocked:
			t.Fatal("Block returned before free space appeared")
		default:
		}

		// Release the writer; drainer drains, free space appears, Block
		// returns.
		bw.Release()
		<-blocked
		ph.Drop(t.Context())
		s.Drop(context.Background())
	})
}

func TestPollableBlockWaitsForFlushCompletion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bw := newBlockingWriter()
		s := NewIOWriterOutputStream(nil, nil, bw, WithBufferSize(8))
		okWrite(t, s, []byte("hi"))
		okFlush(t, s)
		synctest.Wait()
		ph := subscribe(t, s)

		blocked := make(chan struct{})
		go func() {
			_ = ph.Block(bgCtx)
			close(blocked)
		}()
		synctest.Wait()
		select {
		case <-blocked:
			t.Fatal("Block returned during pending flush")
		default:
		}
		bw.Release()
		<-blocked
		ph.Drop(t.Context())
		s.Drop(context.Background())
	})
}

func TestSubscribeReturnsIndependentPollables(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bw := newBlockingWriter()
		s := NewIOWriterOutputStream(nil, nil, bw, WithBufferSize(4))
		defer func() { bw.Release(); s.Drop(context.Background()) }()
		ph1 := subscribe(t, s)
		ph2 := subscribe(t, s)
		defer ph1.Drop(t.Context())
		defer ph2.Drop(t.Context())
		if ph1 == ph2 {
			t.Fatal("Subscribe returned the same handle twice")
		}
		// Both report ready initially.
		if !pollableReady(t, ph1) || !pollableReady(t, ph2) {
			t.Fatal("both pollables should be ready initially")
		}
		// Fill the buffer; both should report not-ready.
		okWrite(t, s, []byte("xxxx"))
		synctest.Wait()
		if pollableReady(t, ph1) || pollableReady(t, ph2) {
			t.Fatal("both pollables should be not-ready when buffer full")
		}
	})
}

func TestPollableDropRemovesFromTracking(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var dest bytes.Buffer
		s := NewIOWriterOutputStream(nil, nil, &dest)
		defer s.Drop(context.Background())
		ph := subscribe(t, s)

		got := s.activePollables.Load()
		if got != 1 {
			t.Fatalf("tracked = %d, want 1", got)
		}

		// Simulate the canon dtor that fires when wasm drops the
		// pollable handle. The handle's Drop only fires the dtor when
		// the handle is bound to an instance, which would require full
		// component plumbing — invoke the impl method directly here.
		ph.Drop(t.Context())

		got = s.activePollables.Load()
		if got != 0 {
			t.Fatalf("tracked after Drop = %d, want 0", got)
		}
		if got != 0 {
			t.Fatalf("tracked after Drop = %d, want 0", got)
		}
	})
}
