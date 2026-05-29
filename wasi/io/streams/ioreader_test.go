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

// === test doubles ===

// blockingReader gates every Read on release. After release, reads are
// served by the inner reader. If readErr is non-nil, every Read after
// release returns it. Release is idempotent.
type blockingReader struct {
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	inner   io.Reader
	readErr error
}

func newBlockingReader(inner io.Reader) *blockingReader {
	return &blockingReader{release: make(chan struct{}), inner: inner}
}

func (r *blockingReader) Read(p []byte) (int, error) {
	<-r.release
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.readErr != nil {
		return 0, r.readErr
	}
	return r.inner.Read(p)
}

func (r *blockingReader) Release() { r.once.Do(func() { close(r.release) }) }

// errorReader returns the same error from every Read.
type errorReader struct {
	err error
}

func (r *errorReader) Read(_ []byte) (int, error) { return 0, r.err }

// closingReader satisfies io.Closer and counts Close calls.
type closingReader struct {
	mu     sync.Mutex
	inner  io.Reader
	closes int
}

func (r *closingReader) Read(p []byte) (int, error) { return r.inner.Read(p) }

func (r *closingReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closes++
	return nil
}

func (r *closingReader) Closes() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closes
}

// dataThenErrorReader returns the prepared bytes on the first Read,
// then returns err on every subsequent Read.
type dataThenErrorReader struct {
	mu   sync.Mutex
	data []byte
	err  error
}

func (r *dataThenErrorReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

// === assertion helpers ===

func okRead(t *testing.T, s *IOReaderInputStream, n uint64) []byte {
	t.Helper()
	res, err := s.Read(bgCtx, n)
	if err != nil {
		t.Fatalf("Read: unexpected error: %v", err)
	}
	ok, isOk := res.(ResultListU8StreamErrorOk)
	if !isOk {
		t.Fatalf("Read: expected Ok, got %T", res)
	}
	return ok.Value
}

func okBlockingRead(t *testing.T, s *IOReaderInputStream, n uint64) []byte {
	t.Helper()
	res, err := s.BlockingRead(bgCtx, n)
	if err != nil {
		t.Fatalf("BlockingRead: unexpected error: %v", err)
	}
	ok, isOk := res.(ResultListU8StreamErrorOk)
	if !isOk {
		t.Fatalf("BlockingRead: expected Ok, got %T", res)
	}
	return ok.Value
}

func errRead(t *testing.T, s *IOReaderInputStream, n uint64) StreamError {
	t.Helper()
	res, err := s.Read(bgCtx, n)
	if err != nil {
		t.Fatalf("Read: unexpected host error: %v", err)
	}
	e, ok := res.(ResultListU8StreamErrorErr)
	if !ok {
		t.Fatalf("Read: expected Err, got %T", res)
	}
	return e.Value
}

func errBlockingRead(t *testing.T, s *IOReaderInputStream, n uint64) StreamError {
	t.Helper()
	res, err := s.BlockingRead(bgCtx, n)
	if err != nil {
		t.Fatalf("BlockingRead: unexpected host error: %v", err)
	}
	e, ok := res.(ResultListU8StreamErrorErr)
	if !ok {
		t.Fatalf("BlockingRead: expected Err, got %T", res)
	}
	return e.Value
}

func okSkip(t *testing.T, s *IOReaderInputStream, n uint64) uint64 {
	t.Helper()
	res, err := s.Skip(bgCtx, n)
	if err != nil {
		t.Fatalf("Skip: unexpected error: %v", err)
	}
	ok, isOk := res.(ResultU64StreamErrorOk)
	if !isOk {
		t.Fatalf("Skip: expected Ok, got %T", res)
	}
	return ok.Value
}

func okBlockingSkip(t *testing.T, s *IOReaderInputStream, n uint64) uint64 {
	t.Helper()
	res, err := s.BlockingSkip(bgCtx, n)
	if err != nil {
		t.Fatalf("BlockingSkip: unexpected error: %v", err)
	}
	ok, isOk := res.(ResultU64StreamErrorOk)
	if !isOk {
		t.Fatalf("BlockingSkip: expected Ok, got %T", res)
	}
	return ok.Value
}

func errSkip(t *testing.T, s *IOReaderInputStream, n uint64) StreamError {
	t.Helper()
	res, err := s.Skip(bgCtx, n)
	if err != nil {
		t.Fatalf("Skip: unexpected host error: %v", err)
	}
	e, ok := res.(ResultU64StreamErrorErr)
	if !ok {
		t.Fatalf("Skip: expected Err, got %T", res)
	}
	return e.Value
}

// drainAll reads from s until the stream reports a stream-error and
// returns the accumulated bytes plus the terminal error.
func drainAll(t *testing.T, s *IOReaderInputStream) ([]byte, StreamError) {
	t.Helper()
	var out []byte
	for i := 0; i < 1024; i++ {
		res, err := s.BlockingRead(bgCtx, 4096)
		if err != nil {
			t.Fatalf("BlockingRead: %v", err)
		}
		switch r := res.(type) {
		case ResultListU8StreamErrorOk:
			out = append(out, r.Value...)
		case ResultListU8StreamErrorErr:
			return out, r.Value
		default:
			t.Fatalf("unexpected result type %T", res)
		}
	}
	t.Fatal("drainAll: too many iterations")
	return nil, nil
}

// === tests ===

func TestReaderEmptySourceHitsEOF(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewIOReaderInputStream(nil, nil, bytes.NewReader(nil))
		defer s.Drop(context.Background())
		synctest.Wait()
		// No data, EOF observed → next Read returns Closed.
		got := errRead(t, s, 4)
		if _, ok := got.(StreamErrorClosed); !ok {
			t.Fatalf("expected closed, got %T", got)
		}
	})
}

func TestReaderDrainsAvailableData(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewIOReaderInputStream(nil, nil, bytes.NewReader([]byte("hello")))
		defer s.Drop(context.Background())
		synctest.Wait()
		got := okRead(t, s, 5)
		if string(got) != "hello" {
			t.Fatalf("Read = %q, want %q", got, "hello")
		}
	})
}

func TestReaderReadAfterDrainReportsClosed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewIOReaderInputStream(nil, nil, bytes.NewReader([]byte("data")))
		defer s.Drop(context.Background())
		synctest.Wait()
		got := okRead(t, s, 4)
		if string(got) != "data" {
			t.Fatalf("Read = %q", got)
		}
		// Buffer empty, EOF observed → Closed.
		e := errRead(t, s, 1)
		if _, ok := e.(StreamErrorClosed); !ok {
			t.Fatalf("expected closed, got %T", e)
		}
	})
}

func TestReaderReadOnEmptyOpenReturnsEmpty(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		br := newBlockingReader(bytes.NewReader([]byte("data")))
		s := NewIOReaderInputStream(nil, nil, br)
		defer func() {
			br.Release()
			s.Drop(context.Background())
		}()
		synctest.Wait() // pumper is parked inside br.Read
		got := okRead(t, s, 4)
		if len(got) != 0 {
			t.Fatalf("Read = %q, want empty", got)
		}
	})
}

func TestReaderReadCappedByAvailableBytes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewIOReaderInputStream(nil, nil, bytes.NewReader([]byte("abcdef")))
		defer s.Drop(context.Background())
		synctest.Wait()
		got := okRead(t, s, 1024)
		if string(got) != "abcdef" {
			t.Fatalf("Read = %q", got)
		}
	})
}

func TestReaderBufferSizeBoundsBufferedBytes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		br := newBlockingReader(bytes.NewReader([]byte("0123456789")))
		s := NewIOReaderInputStream(nil, nil, br, WithBufferSize(4))
		defer s.Drop(context.Background())
		// Pumper is gated; release lets it pull only what fits in the
		// buffer, then it parks waiting for free space.
		br.Release()
		synctest.Wait()
		got := okRead(t, s, 1024)
		if len(got) > 4 {
			t.Fatalf("Read returned %d bytes, want ≤ 4", len(got))
		}
	})
}

func TestReaderBlockingReadWaitsForData(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		br := newBlockingReader(bytes.NewReader([]byte("payload")))
		s := NewIOReaderInputStream(nil, nil, br)
		defer s.Drop(context.Background())

		done := make(chan []byte, 1)
		go func() {
			done <- okBlockingRead(t, s, 7)
		}()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("BlockingRead returned before data was available")
		default:
		}
		br.Release()
		got := <-done
		if string(got) == "" {
			t.Fatal("BlockingRead returned empty after release")
		}
	})
}

func TestReaderBlockingReadWakesOnClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		br := newBlockingReader(bytes.NewReader(nil))
		br.readErr = io.EOF
		s := NewIOReaderInputStream(nil, nil, br)
		defer s.Drop(context.Background())

		done := make(chan StreamError, 1)
		go func() {
			done <- errBlockingRead(t, s, 4)
		}()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("BlockingRead returned before EOF was observed")
		default:
		}
		br.Release()
		got := <-done
		if _, ok := got.(StreamErrorClosed); !ok {
			t.Fatalf("expected closed, got %T", got)
		}
	})
}

func TestReaderRingWrapPreservesByteOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		payload := []byte("AAAABBBBCCCCDDDDEE")
		s := NewIOReaderInputStream(nil, nil, bytes.NewReader(payload), WithBufferSize(8))
		defer s.Drop(context.Background())
		got, e := drainAll(t, s)
		if !bytes.Equal(got, payload) {
			t.Fatalf("got %q, want %q", got, payload)
		}
		if _, ok := e.(StreamErrorClosed); !ok {
			t.Fatalf("expected closed terminator, got %T", e)
		}
	})
}

func TestReaderErrorClosesStreamAndReportsOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		er := &errorReader{err: errors.New("boom")}
		s := NewIOReaderInputStream(nil, nil, er)
		defer s.Drop(context.Background())
		synctest.Wait()

		first := errRead(t, s, 4)
		lof, ok := first.(StreamErrorLastOperationFailed)
		if !ok {
			t.Fatalf("expected last-operation-failed, got %T", first)
		}
		msg, _ := lof.Value.ToDebugString(bgCtx)
		if msg != "boom" {
			t.Fatalf("debug string = %q, want %q", msg, "boom")
		}

		second := errRead(t, s, 4)
		if _, ok := second.(StreamErrorClosed); !ok {
			t.Fatalf("expected closed, got %T", second)
		}
	})
}

func TestReaderPartialDataBeforeError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dr := &dataThenErrorReader{data: []byte("ok"), err: errors.New("boom")}
		s := NewIOReaderInputStream(nil, nil, dr)
		defer s.Drop(context.Background())
		synctest.Wait()

		// Buffered bytes drain first.
		got := okRead(t, s, 4)
		if string(got) != "ok" {
			t.Fatalf("Read = %q, want %q", got, "ok")
		}
		// Then the captured error surfaces once.
		first := errRead(t, s, 1)
		lof, ok := first.(StreamErrorLastOperationFailed)
		if !ok {
			t.Fatalf("expected last-operation-failed, got %T", first)
		}
		msg, _ := lof.Value.ToDebugString(bgCtx)
		if msg != "boom" {
			t.Fatalf("debug string = %q", msg)
		}
		// And subsequent calls report plain closed.
		second := errRead(t, s, 1)
		if _, ok := second.(StreamErrorClosed); !ok {
			t.Fatalf("expected closed, got %T", second)
		}
	})
}

func TestReaderSkipAdvancesHead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewIOReaderInputStream(nil, nil, bytes.NewReader([]byte("abcdef")))
		defer s.Drop(context.Background())
		synctest.Wait()
		if got := okSkip(t, s, 3); got != 3 {
			t.Fatalf("Skip = %d, want 3", got)
		}
		got := okRead(t, s, 16)
		if string(got) != "def" {
			t.Fatalf("Read = %q, want %q", got, "def")
		}
	})
}

func TestReaderSkipOnEmptyOpenReturnsZero(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		br := newBlockingReader(bytes.NewReader([]byte("data")))
		s := NewIOReaderInputStream(nil, nil, br)
		defer func() {
			br.Release()
			s.Drop(context.Background())
		}()
		synctest.Wait()
		if got := okSkip(t, s, 4); got != 0 {
			t.Fatalf("Skip = %d, want 0", got)
		}
	})
}

func TestReaderSkipReportsClosedAfterEOF(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewIOReaderInputStream(nil, nil, bytes.NewReader(nil))
		defer s.Drop(context.Background())
		synctest.Wait()
		got := errSkip(t, s, 4)
		if _, ok := got.(StreamErrorClosed); !ok {
			t.Fatalf("expected closed, got %T", got)
		}
	})
}

func TestReaderBlockingSkipWaitsForData(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		br := newBlockingReader(bytes.NewReader([]byte("payload")))
		s := NewIOReaderInputStream(nil, nil, br)
		defer s.Drop(context.Background())

		done := make(chan uint64, 1)
		go func() {
			done <- okBlockingSkip(t, s, 7)
		}()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("BlockingSkip returned before data was available")
		default:
		}
		br.Release()
		got := <-done
		if got == 0 || got > 7 {
			t.Fatalf("BlockingSkip = %d, want 1..7", got)
		}
	})
}

func TestReaderBlockingReadZeroReturnsEmpty(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		br := newBlockingReader(bytes.NewReader([]byte("payload")))
		s := NewIOReaderInputStream(nil, nil, br)
		defer func() {
			br.Release()
			s.Drop(context.Background())
		}()
		got := okBlockingRead(t, s, 0)
		if len(got) != 0 {
			t.Fatalf("BlockingRead(0) = %q, want empty", got)
		}
	})
}

func TestReaderDropClosesIOCloser(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cr := &closingReader{inner: bytes.NewReader([]byte("z"))}
		s := NewIOReaderInputStream(nil, nil, cr)
		synctest.Wait()
		s.Drop(context.Background())
		if cr.Closes() != 1 {
			t.Fatalf("Close calls = %d, want 1", cr.Closes())
		}
	})
}

func TestReaderDropOnPlainReaderDoesNotPanic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewIOReaderInputStream(nil, nil, bytes.NewReader([]byte("x")))
		s.Drop(context.Background())
	})
}

func TestReaderDropStopsPumper(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		br := newBlockingReader(bytes.NewReader([]byte("x")))
		s := NewIOReaderInputStream(nil, nil, br)
		// Release the source first so the pumper isn't parked inside
		// br.Read when Drop tries to wait on it.
		br.Release()
		s.Drop(context.Background())
	})
}

func TestReaderZeroOrNegativeBufferSizeFallsBack(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewIOReaderInputStream(nil, nil, bytes.NewReader(nil), WithBufferSize(0))
		defer s.Drop(context.Background())
		if int(s.bufCap) != defaultIOReaderBufferSize {
			t.Fatalf("bufCap = %d, want %d", s.bufCap, defaultIOReaderBufferSize)
		}
	})
}

// === pollable tests ===

func subscribeReader(t *testing.T, s *IOReaderInputStream) *poll.PollableHandle {
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

func TestReaderSubscribeReadyWhenDataBuffered(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewIOReaderInputStream(nil, nil, bytes.NewReader([]byte("data")))
		defer s.Drop(context.Background())
		synctest.Wait()
		ph := subscribeReader(t, s)
		defer ph.Drop(t.Context())
		if !pollableReady(t, ph) {
			t.Fatal("pollable should be ready when data is buffered")
		}
	})
}

func TestReaderSubscribeNotReadyWhenEmpty(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		br := newBlockingReader(bytes.NewReader([]byte("data")))
		s := NewIOReaderInputStream(nil, nil, br)
		defer func() {
			br.Release()
			s.Drop(context.Background())
		}()
		synctest.Wait()
		ph := subscribeReader(t, s)
		defer ph.Drop(t.Context())
		if pollableReady(t, ph) {
			t.Fatal("empty-buffer pollable should not be ready")
		}
	})
}

func TestReaderSubscribeReadyWhenStreamClosedByEOF(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewIOReaderInputStream(nil, nil, bytes.NewReader(nil))
		defer s.Drop(context.Background())
		synctest.Wait()
		ph := subscribeReader(t, s)
		defer ph.Drop(t.Context())
		if !pollableReady(t, ph) {
			t.Fatal("EOF-closed pollable should be ready")
		}
	})
}

func TestReaderSubscribeAfterStreamClosedReturnsReadyPollable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		er := &errorReader{err: errors.New("boom")}
		s := NewIOReaderInputStream(nil, nil, er)
		defer s.Drop(context.Background())
		synctest.Wait()
		ph := subscribeReader(t, s)
		defer ph.Drop(t.Context())
		if !pollableReady(t, ph) {
			t.Fatal("post-close subscribe should yield a ready pollable")
		}
	})
}

func TestReaderPollableBlockReturnsImmediatelyWhenReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewIOReaderInputStream(nil, nil, bytes.NewReader([]byte("x")))
		defer s.Drop(context.Background())
		synctest.Wait()
		ph := subscribeReader(t, s)
		defer ph.Drop(t.Context())
		if err := ph.Block(bgCtx); err != nil {
			t.Fatalf("Block: %v", err)
		}
	})
}

func TestReaderPollableBlockWaitsForData(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		br := newBlockingReader(bytes.NewReader([]byte("payload")))
		s := NewIOReaderInputStream(nil, nil, br)
		synctest.Wait()
		ph := subscribeReader(t, s)
		blocked := make(chan struct{})
		go func() {
			_ = ph.Block(bgCtx)
			close(blocked)
		}()
		synctest.Wait()
		select {
		case <-blocked:
			t.Fatal("Block returned before data appeared")
		default:
		}

		br.Release()
		<-blocked
		ph.Drop(t.Context())
		s.Drop(context.Background())
	})
}

func TestReaderSubscribeReturnsIndependentPollables(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		br := newBlockingReader(bytes.NewReader([]byte("data")))
		s := NewIOReaderInputStream(nil, nil, br)
		defer func() {
			br.Release()
			s.Drop(context.Background())
		}()
		synctest.Wait()
		ph1 := subscribeReader(t, s)
		defer ph1.Drop(t.Context())
		ph2 := subscribeReader(t, s)
		defer ph2.Drop(t.Context())
		if ph1 == ph2 {
			t.Fatal("Subscribe returned the same handle twice")
		}
		if pollableReady(t, ph1) || pollableReady(t, ph2) {
			t.Fatal("both pollables should be not-ready when buffer empty")
		}
		br.Release()
		synctest.Wait()
		if !pollableReady(t, ph1) || !pollableReady(t, ph2) {
			t.Fatal("both pollables should be ready after data arrives")
		}
	})
}

func TestReaderPollableDropRemovesFromTracking(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		br := newBlockingReader(bytes.NewReader([]byte("x")))
		s := NewIOReaderInputStream(nil, nil, br)
		defer func() {
			br.Release()
			s.Drop(context.Background())
		}()
		ph := subscribeReader(t, s)

		got := s.activePollables.Load()
		if got != 1 {
			t.Fatalf("tracked = %d, want 1", got)
		}

		ph.Drop(t.Context())

		got = s.activePollables.Load()
		if got != 0 {
			t.Fatalf("tracked after Drop = %d, want 0", got)
		}
	})
}
