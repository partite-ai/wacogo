package insecure

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

func TestGetInsecureRandomBytesReturnsRequestedLength(t *testing.T) {
	r := newRNG()
	for _, n := range []uint64{0, 1, 7, 8, 9, 16, 33, 1024} {
		got, err := r.GetInsecureRandomBytes(bgCtx, n)
		if err != nil {
			t.Fatalf("GetInsecureRandomBytes(%d): %v", n, err)
		}
		if uint64(len(got)) != n {
			t.Fatalf("GetInsecureRandomBytes(%d) len = %d, want %d", n, len(got), n)
		}
	}
}

func TestGetInsecureRandomBytesZeroIsEmpty(t *testing.T) {
	r := newRNG()
	got, err := r.GetInsecureRandomBytes(bgCtx, 0)
	if err != nil {
		t.Fatalf("GetInsecureRandomBytes(0): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("GetInsecureRandomBytes(0) len = %d, want 0", len(got))
	}
}

func TestGetInsecureRandomBytesIsNotAllZero(t *testing.T) {
	r := newRNG()
	got, err := r.GetInsecureRandomBytes(bgCtx, 64)
	if err != nil {
		t.Fatalf("GetInsecureRandomBytes: %v", err)
	}
	zeros := make([]byte, 64)
	if bytes.Equal(got, zeros) {
		t.Fatalf("64 random bytes were all zero")
	}
}

func TestGetInsecureRandomBytesDiffersBetweenCalls(t *testing.T) {
	r := newRNG()
	a, err := r.GetInsecureRandomBytes(bgCtx, 32)
	if err != nil {
		t.Fatalf("GetInsecureRandomBytes: %v", err)
	}
	b, err := r.GetInsecureRandomBytes(bgCtx, 32)
	if err != nil {
		t.Fatalf("GetInsecureRandomBytes: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Fatalf("two consecutive 32-byte reads matched: %x", a)
	}
}

func TestGetInsecureRandomU64AdvancesBetweenCalls(t *testing.T) {
	r := newRNG()
	a, err := r.GetInsecureRandomU64(bgCtx)
	if err != nil {
		t.Fatalf("GetInsecureRandomU64: %v", err)
	}
	b, err := r.GetInsecureRandomU64(bgCtx)
	if err != nil {
		t.Fatalf("GetInsecureRandomU64: %v", err)
	}
	if a == b {
		t.Fatalf("two consecutive GetInsecureRandomU64 returned same value %#x", a)
	}
}

func TestGetInsecureRandomBytesReadsFromInjectedSource(t *testing.T) {
	want := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	r := &rng{src: bytes.NewReader(want)}
	got, err := r.GetInsecureRandomBytes(bgCtx, uint64(len(want)))
	if err != nil {
		t.Fatalf("GetInsecureRandomBytes: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("GetInsecureRandomBytes = %x, want %x", got, want)
	}
}

func TestGetInsecureRandomU64ReadsLittleEndianFromSource(t *testing.T) {
	buf := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	r := &rng{src: bytes.NewReader(buf)}
	got, err := r.GetInsecureRandomU64(bgCtx)
	if err != nil {
		t.Fatalf("GetInsecureRandomU64: %v", err)
	}
	want := binary.LittleEndian.Uint64(buf)
	if got != want {
		t.Fatalf("GetInsecureRandomU64 = %#x, want %#x", got, want)
	}
}

type errReader struct{}

var errFakeIO = errors.New("fake io error")

func (errReader) Read(_ []byte) (int, error) { return 0, errFakeIO }

func TestGetInsecureRandomBytesPropagatesSourceError(t *testing.T) {
	r := &rng{src: errReader{}}
	_, err := r.GetInsecureRandomBytes(bgCtx, 8)
	if !errors.Is(err, errFakeIO) {
		t.Fatalf("GetInsecureRandomBytes err = %v, want wraps errFakeIO", err)
	}
}

func TestGetInsecureRandomU64PropagatesSourceError(t *testing.T) {
	r := &rng{src: errReader{}}
	_, err := r.GetInsecureRandomU64(bgCtx)
	if !errors.Is(err, errFakeIO) {
		t.Fatalf("GetInsecureRandomU64 err = %v, want wraps errFakeIO", err)
	}
}

func TestGetInsecureRandomBytesShortReadIsError(t *testing.T) {
	r := &rng{src: bytes.NewReader([]byte{1, 2, 3, 4})}
	_, err := r.GetInsecureRandomBytes(bgCtx, 8)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("GetInsecureRandomBytes err = %v, want wraps ErrUnexpectedEOF", err)
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
				if _, err := r.GetInsecureRandomU64(bgCtx); err != nil {
					t.Errorf("GetInsecureRandomU64: %v", err)
					return
				}
				if _, err := r.GetInsecureRandomBytes(bgCtx, 32); err != nil {
					t.Errorf("GetInsecureRandomBytes: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
