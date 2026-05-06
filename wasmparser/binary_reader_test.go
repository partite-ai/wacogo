package wasmparser

import (
	"bytes"
	"io"
	"math"
	"testing"
)

// ---- Core Primitives ----

func TestNewBinaryReaderFromBytes(t *testing.T) {
	r := newBinaryReaderAt(bytes.NewReader([]byte{0x01, 0x02, 0x03}), 10)
	if r.Offset() != 10 {
		t.Fatalf("expected offset 10, got %d", r.Offset())
	}
	if r.IsEOF() {
		t.Fatalf("expected non-EOF reader")
	}
}

func TestNewBinaryReader(t *testing.T) {
	buf := bytes.NewReader([]byte{0xAA, 0xBB})
	r := NewBinaryReader(buf)
	if r.IsEOF() {
		t.Fatalf("expected non-EOF reader")
	}
	if r.Offset() != 0 {
		t.Fatalf("expected offset 0, got %d", r.Offset())
	}
}

func TestNewBinaryReaderAt(t *testing.T) {
	buf := bytes.NewReader([]byte{0xAA, 0xBB})
	r := newBinaryReaderAt(buf, 5)
	b, err := r.ReadByte()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b != 0xAA {
		t.Fatalf("expected 0xAA, got 0x%x", b)
	}
	if r.Offset() != 6 {
		t.Fatalf("expected offset 6, got %d", r.Offset())
	}
}

func TestReadByte(t *testing.T) {
	r := newBinaryReaderAt(bytes.NewReader([]byte{0x42}), 0)
	b, err := r.ReadByte()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b != 0x42 {
		t.Fatalf("expected 0x42, got 0x%x", b)
	}
	if !r.IsEOF() {
		t.Fatalf("expected EOF after reading all bytes")
	}
	if r.Offset() != 1 {
		t.Fatalf("expected offset 1, got %d", r.Offset())
	}
}

func TestReadByteEOF(t *testing.T) {
	r := newBinaryReaderAt(bytes.NewReader([]byte{}), 0)
	_, err := r.ReadByte()
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}

func TestPeek(t *testing.T) {
	r := newBinaryReaderAt(bytes.NewReader([]byte{0x7F}), 0)
	b, err := r.Peek()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b != 0x7F {
		t.Fatalf("expected 0x7F, got 0x%x", b)
	}
	// Peek does not advance
	if r.Offset() != 0 {
		t.Fatalf("expected offset 0 after peek, got %d", r.Offset())
	}
}

func TestPeekEOF(t *testing.T) {
	r := newBinaryReaderAt(bytes.NewReader([]byte{}), 0)
	_, err := r.Peek()
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}

func TestReadBytes(t *testing.T) {
	r := newBinaryReaderAt(bytes.NewReader([]byte{0x01, 0x02, 0x03, 0x04}), 0)
	bs, err := r.ReadBytes(3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bs) != 3 {
		t.Fatalf("expected 3 bytes, got %d", len(bs))
	}
	if bs[0] != 0x01 || bs[1] != 0x02 || bs[2] != 0x03 {
		t.Fatalf("unexpected bytes: %v", bs)
	}
	if r.IsEOF() {
		t.Fatalf("expected non-EOF, still 1 byte left")
	}
}

func TestReadBytesPartialEOF(t *testing.T) {
	r := newBinaryReaderAt(bytes.NewReader([]byte{0x01, 0x02}), 0)
	_, err := r.ReadBytes(5)
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}

func TestReadBytesZero(t *testing.T) {
	r := newBinaryReaderAt(bytes.NewReader([]byte{0x01}), 0)
	bs, err := r.ReadBytes(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bs) != 0 {
		t.Fatalf("expected 0 bytes, got %d", len(bs))
	}
}

func TestOffsetTracking(t *testing.T) {
	// base offset 100, read 2 bytes -> Offset() should be 102
	r := newBinaryReaderAt(bytes.NewReader([]byte{0xAA, 0xBB, 0xCC}), 100)
	r.ReadByte()
	r.ReadByte()
	if r.Offset() != 102 {
		t.Fatalf("expected offset 102, got %d", r.Offset())
	}
}

// ---- LEB128 ----

func TestReadU32(t *testing.T) {
	cases := []struct {
		name     string
		data     []byte
		expected uint32
	}{
		{"zero", []byte{0x00}, 0},
		{"127", []byte{0x7f}, 127},
		{"128", []byte{0x80, 0x01}, 128},
		{"624485", []byte{0xe5, 0x8e, 0x26}, 624485},
		{"max_u32", []byte{0xff, 0xff, 0xff, 0xff, 0x0f}, 0xFFFFFFFF},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newBinaryReaderAt(bytes.NewReader(tc.data), 0)
			v, err := r.ReadU32()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v != tc.expected {
				t.Fatalf("expected %d, got %d", tc.expected, v)
			}
		})
	}
}

func TestReadU32Invalid(t *testing.T) {
	t.Run("too_long", func(t *testing.T) {
		// 6 bytes — continuation bit on byte 5 (index 4) is set past threshold
		data := []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x00}
		r := newBinaryReaderAt(bytes.NewReader(data), 0)
		_, err := r.ReadU32()
		if err == nil {
			t.Fatal("expected error for too-long LEB128")
		}
	})
	t.Run("overflow", func(t *testing.T) {
		// 5 bytes but value > 2^32-1
		data := []byte{0xff, 0xff, 0xff, 0xff, 0x1f}
		r := newBinaryReaderAt(bytes.NewReader(data), 0)
		_, err := r.ReadU32()
		if err == nil {
			t.Fatal("expected error for overflow LEB128")
		}
	})
	t.Run("truncated", func(t *testing.T) {
		data := []byte{0x80} // continuation bit set, no more bytes
		r := newBinaryReaderAt(bytes.NewReader(data), 0)
		_, err := r.ReadU32()
		if err != io.ErrUnexpectedEOF {
			t.Fatalf("expected io.ErrUnexpectedEOF, got %v", err)
		}
	})
}

func TestReadU64(t *testing.T) {
	cases := []struct {
		name     string
		data     []byte
		expected uint64
	}{
		{"zero", []byte{0x00}, 0},
		{"127", []byte{0x7f}, 127},
		{"128", []byte{0x80, 0x01}, 128},
		{"max_u64", []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}, 0xFFFFFFFFFFFFFFFF},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newBinaryReaderAt(bytes.NewReader(tc.data), 0)
			v, err := r.ReadU64()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v != tc.expected {
				t.Fatalf("expected %d, got %d", tc.expected, v)
			}
		})
	}
}

func TestReadU64Invalid(t *testing.T) {
	t.Run("too_long", func(t *testing.T) {
		data := []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x00}
		r := newBinaryReaderAt(bytes.NewReader(data), 0)
		_, err := r.ReadU64()
		if err == nil {
			t.Fatal("expected error for too-long LEB128")
		}
	})
	t.Run("truncated", func(t *testing.T) {
		data := []byte{0x80}
		r := newBinaryReaderAt(bytes.NewReader(data), 0)
		_, err := r.ReadU64()
		if err != io.ErrUnexpectedEOF {
			t.Fatalf("expected io.ErrUnexpectedEOF, got %v", err)
		}
	})
}

func TestReadS32(t *testing.T) {
	cases := []struct {
		name     string
		data     []byte
		expected int32
	}{
		{"zero", []byte{0x00}, 0},
		{"8", []byte{0x08}, 8},
		{"-1", []byte{0x7f}, -1},
		{"-128", []byte{0x80, 0x7f}, -128},
		{"min_i32", []byte{0x80, 0x80, 0x80, 0x80, 0x78}, math.MinInt32},
		{"max_i32", []byte{0xff, 0xff, 0xff, 0xff, 0x07}, math.MaxInt32},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newBinaryReaderAt(bytes.NewReader(tc.data), 0)
			v, err := r.ReadS32()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v != tc.expected {
				t.Fatalf("expected %d, got %d", tc.expected, v)
			}
		})
	}
}

func TestReadS33(t *testing.T) {
	// S33 fits in int64 — used for component model type indices
	cases := []struct {
		name     string
		data     []byte
		expected int64
	}{
		{"zero", []byte{0x00}, 0},
		{"8", []byte{0x08}, 8},
		{"-1", []byte{0x7f}, -1},
		// max s33 = 0xFFFFFFFF (as unsigned 32-bit -> signed 33-bit it's positive)
		{"max_s33", []byte{0xff, 0xff, 0xff, 0xff, 0x0f}, 0xFFFFFFFF},
		// min s33 = -0x100000000
		{"min_s33", []byte{0x80, 0x80, 0x80, 0x80, 0x70}, -0x100000000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newBinaryReaderAt(bytes.NewReader(tc.data), 0)
			v, err := r.ReadS33()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v != tc.expected {
				t.Fatalf("expected %d, got %d", tc.expected, v)
			}
		})
	}
}

func TestReadS64(t *testing.T) {
	cases := []struct {
		name     string
		data     []byte
		expected int64
	}{
		{"zero", []byte{0x00}, 0},
		{"8", []byte{0x08}, 8},
		{"-1", []byte{0x7f}, -1},
		{"min_i64", []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x7f}, math.MinInt64},
		{"max_i64", []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00}, math.MaxInt64},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newBinaryReaderAt(bytes.NewReader(tc.data), 0)
			v, err := r.ReadS64()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v != tc.expected {
				t.Fatalf("expected %d, got %d", tc.expected, v)
			}
		})
	}
}

// ---- Strings and Floats ----

func TestReadString(t *testing.T) {
	t.Run("hello", func(t *testing.T) {
		data := []byte{0x05, 'h', 'e', 'l', 'l', 'o'}
		r := newBinaryReaderAt(bytes.NewReader(data), 0)
		s, err := r.ReadString()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s != "hello" {
			t.Fatalf("expected \"hello\", got %q", s)
		}
	})
	t.Run("empty", func(t *testing.T) {
		data := []byte{0x00}
		r := newBinaryReaderAt(bytes.NewReader(data), 0)
		s, err := r.ReadString()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s != "" {
			t.Fatalf("expected empty string, got %q", s)
		}
	})
	t.Run("invalid_utf8", func(t *testing.T) {
		data := []byte{0x02, 0xFF, 0xFE}
		r := newBinaryReaderAt(bytes.NewReader(data), 0)
		_, err := r.ReadString()
		if err == nil {
			t.Fatal("expected error for invalid UTF-8")
		}
	})
	t.Run("too_long", func(t *testing.T) {
		// encode length 100001 as u32 LEB128: 100001 = 0x000186A1
		// LEB128: 0xA1, 0x8D, 0x06
		data := []byte{0xa1, 0x8d, 0x06}
		r := newBinaryReaderAt(bytes.NewReader(data), 0)
		_, err := r.ReadString()
		if err == nil {
			t.Fatal("expected error for string too long")
		}
	})
}

func TestReadF32(t *testing.T) {
	// 1.0 as f32 little-endian = 0x3F800000 -> bytes [0x00, 0x00, 0x80, 0x3F]
	data := []byte{0x00, 0x00, 0x80, 0x3F}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	v, err := r.ReadF32()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 1.0 {
		t.Fatalf("expected 1.0, got %f", v)
	}
}

func TestReadF32EOF(t *testing.T) {
	r := newBinaryReaderAt(bytes.NewReader([]byte{0x00, 0x00}), 0)
	_, err := r.ReadF32()
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}

func TestReadF64(t *testing.T) {
	// 1.0 as f64 little-endian = 0x3FF0000000000000 -> bytes [0x00,0x00,0x00,0x00,0x00,0x00,0xF0,0x3F]
	data := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF0, 0x3F}
	r := newBinaryReaderAt(bytes.NewReader(data), 0)
	v, err := r.ReadF64()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 1.0 {
		t.Fatalf("expected 1.0, got %f", v)
	}
}

func TestReadF64EOF(t *testing.T) {
	r := newBinaryReaderAt(bytes.NewReader([]byte{0x00, 0x00, 0x00, 0x00}), 0)
	_, err := r.ReadF64()
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}
