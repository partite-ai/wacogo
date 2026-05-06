package random

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"testing"
)

var bgCtx = context.Background()

func TestGetRandomBytesReturnsRequestedLength(t *testing.T) {
	r := newRNG()
	for _, n := range []uint64{0, 1, 7, 8, 9, 16, 33, 1024} {
		got, err := r.GetRandomBytes(bgCtx, n)
		if err != nil {
			t.Fatalf("GetRandomBytes(%d): %v", n, err)
		}
		if uint64(len(got)) != n {
			t.Fatalf("GetRandomBytes(%d) len = %d, want %d", n, len(got), n)
		}
	}
}

func TestGetRandomBytesZeroIsEmpty(t *testing.T) {
	r := newRNG()
	got, err := r.GetRandomBytes(bgCtx, 0)
	if err != nil {
		t.Fatalf("GetRandomBytes(0): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("GetRandomBytes(0) len = %d, want 0", len(got))
	}
}

func TestGetRandomBytesIsNotAllZero(t *testing.T) {
	r := newRNG()
	got, err := r.GetRandomBytes(bgCtx, 64)
	if err != nil {
		t.Fatalf("GetRandomBytes: %v", err)
	}
	zeros := make([]byte, 64)
	if bytes.Equal(got, zeros) {
		t.Fatalf("64 random bytes were all zero")
	}
}

func TestGetRandomBytesDiffersBetweenCalls(t *testing.T) {
	r := newRNG()
	a, err := r.GetRandomBytes(bgCtx, 32)
	if err != nil {
		t.Fatalf("GetRandomBytes: %v", err)
	}
	b, err := r.GetRandomBytes(bgCtx, 32)
	if err != nil {
		t.Fatalf("GetRandomBytes: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Fatalf("two consecutive 32-byte reads matched: %x", a)
	}
}

func TestGetRandomU64AdvancesBetweenCalls(t *testing.T) {
	r := newRNG()
	a, err := r.GetRandomU64(bgCtx)
	if err != nil {
		t.Fatalf("GetRandomU64: %v", err)
	}
	b, err := r.GetRandomU64(bgCtx)
	if err != nil {
		t.Fatalf("GetRandomU64: %v", err)
	}
	if a == b {
		t.Fatalf("two consecutive GetRandomU64 returned same value %#x", a)
	}
}

func TestGetRandomBytesReadsFromInjectedSource(t *testing.T) {
	want := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	r := &rng{src: bytes.NewReader(want)}
	got, err := r.GetRandomBytes(bgCtx, uint64(len(want)))
	if err != nil {
		t.Fatalf("GetRandomBytes: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("GetRandomBytes = %x, want %x", got, want)
	}
}

func TestGetRandomU64ReadsLittleEndianFromSource(t *testing.T) {
	buf := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	r := &rng{src: bytes.NewReader(buf)}
	got, err := r.GetRandomU64(bgCtx)
	if err != nil {
		t.Fatalf("GetRandomU64: %v", err)
	}
	want := binary.LittleEndian.Uint64(buf)
	if got != want {
		t.Fatalf("GetRandomU64 = %#x, want %#x", got, want)
	}
}

// errReader returns errFakeIO on every Read.
type errReader struct{}

var errFakeIO = errors.New("fake io error")

func (errReader) Read(_ []byte) (int, error) { return 0, errFakeIO }

func TestGetRandomBytesPropagatesSourceError(t *testing.T) {
	r := &rng{src: errReader{}}
	_, err := r.GetRandomBytes(bgCtx, 8)
	if !errors.Is(err, errFakeIO) {
		t.Fatalf("GetRandomBytes err = %v, want wraps errFakeIO", err)
	}
}

func TestGetRandomU64PropagatesSourceError(t *testing.T) {
	r := &rng{src: errReader{}}
	_, err := r.GetRandomU64(bgCtx)
	if !errors.Is(err, errFakeIO) {
		t.Fatalf("GetRandomU64 err = %v, want wraps errFakeIO", err)
	}
}

func TestGetRandomBytesShortReadIsError(t *testing.T) {
	// Source has only 4 bytes; asking for 8 should yield ErrUnexpectedEOF.
	r := &rng{src: bytes.NewReader([]byte{1, 2, 3, 4})}
	_, err := r.GetRandomBytes(bgCtx, 8)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("GetRandomBytes err = %v, want wraps ErrUnexpectedEOF", err)
	}
}

func TestConcurrentCallsDoNotRace(t *testing.T) {
	r := newRNG()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if _, err := r.GetRandomU64(bgCtx); err != nil {
					t.Errorf("GetRandomU64: %v", err)
					return
				}
				if _, err := r.GetRandomBytes(bgCtx, 32); err != nil {
					t.Errorf("GetRandomBytes: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
