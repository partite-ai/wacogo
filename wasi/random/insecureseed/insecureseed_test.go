package insecureseed

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

func TestInsecureSeedReadsLittleEndianHalves(t *testing.T) {
	buf := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
	}
	s := &seeder{src: bytes.NewReader(buf)}
	got, err := s.InsecureSeed(bgCtx)
	if err != nil {
		t.Fatalf("InsecureSeed: %v", err)
	}
	wantF0 := binary.LittleEndian.Uint64(buf[0:8])
	wantF1 := binary.LittleEndian.Uint64(buf[8:16])
	if got.F0 != wantF0 || got.F1 != wantF1 {
		t.Fatalf("InsecureSeed = {%#x, %#x}, want {%#x, %#x}", got.F0, got.F1, wantF0, wantF1)
	}
}

func TestInsecureSeedAdvancesBetweenCalls(t *testing.T) {
	s := newSeeder()
	a, err := s.InsecureSeed(bgCtx)
	if err != nil {
		t.Fatalf("InsecureSeed: %v", err)
	}
	b, err := s.InsecureSeed(bgCtx)
	if err != nil {
		t.Fatalf("InsecureSeed: %v", err)
	}
	if a.F0 == b.F0 && a.F1 == b.F1 {
		t.Fatalf("two consecutive InsecureSeed returned identical pair {%#x, %#x}", a.F0, a.F1)
	}
}

func TestInsecureSeedHalvesAreNotIdentical(t *testing.T) {
	// 2^-64 collision odds; if this fails it's almost certainly a bug
	// (e.g. F0 and F1 reading from the same offset).
	s := newSeeder()
	got, err := s.InsecureSeed(bgCtx)
	if err != nil {
		t.Fatalf("InsecureSeed: %v", err)
	}
	if got.F0 == got.F1 {
		t.Fatalf("F0 == F1 == %#x, suggests halves aren't independent", got.F0)
	}
}

type errReader struct{}

var errFakeIO = errors.New("fake io error")

func (errReader) Read(_ []byte) (int, error) { return 0, errFakeIO }

func TestInsecureSeedPropagatesSourceError(t *testing.T) {
	s := &seeder{src: errReader{}}
	_, err := s.InsecureSeed(bgCtx)
	if !errors.Is(err, errFakeIO) {
		t.Fatalf("InsecureSeed err = %v, want wraps errFakeIO", err)
	}
}

func TestInsecureSeedShortReadIsError(t *testing.T) {
	s := &seeder{src: bytes.NewReader([]byte{1, 2, 3, 4, 5, 6, 7, 8})}
	_, err := s.InsecureSeed(bgCtx)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("InsecureSeed err = %v, want wraps ErrUnexpectedEOF", err)
	}
}

func TestConcurrentCallsDoNotRace(t *testing.T) {
	s := newSeeder()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if _, err := s.InsecureSeed(bgCtx); err != nil {
					t.Errorf("InsecureSeed: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
