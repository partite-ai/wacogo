package wasm

import (
	"bytes"
	"testing"
)

func TestEncodeU32(t *testing.T) {
	tests := []struct {
		name  string
		value uint32
		want  []byte
	}{
		{"zero", 0, []byte{0x00}},
		{"one", 1, []byte{0x01}},
		{"127", 127, []byte{0x7F}},
		{"128", 128, []byte{0x80, 0x01}},
		{"624485", 624485, []byte{0xE5, 0x8E, 0x26}},
		{"max_uint32", 0xFFFFFFFF, []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x0F}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			encodeU32(&buf, tt.value)
			got := buf.Bytes()
			if !bytes.Equal(got, tt.want) {
				t.Errorf("encodeU32(%d) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestEncodeS32(t *testing.T) {
	tests := []struct {
		name  string
		value int32
		want  []byte
	}{
		{"zero", 0, []byte{0x00}},
		{"negative_one", -1, []byte{0x7F}},
		{"negative_128", -128, []byte{0x80, 0x7F}},
		{"127", 127, []byte{0xFF, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			encodeS32(&buf, tt.value)
			got := buf.Bytes()
			if !bytes.Equal(got, tt.want) {
				t.Errorf("encodeS32(%d) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestEncodeS64(t *testing.T) {
	tests := []struct {
		name  string
		value int64
		want  []byte
	}{
		{"zero", 0, []byte{0x00}},
		{"negative_one", -1, []byte{0x7F}},
		{"negative_123456", -123456, []byte{0xC0, 0xBB, 0x78}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			encodeS64(&buf, tt.value)
			got := buf.Bytes()
			if !bytes.Equal(got, tt.want) {
				t.Errorf("encodeS64(%d) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestEncodeVec(t *testing.T) {
	var buf bytes.Buffer
	encodeVec(&buf, 5)
	got := buf.Bytes()
	want := []byte{0x05}
	if !bytes.Equal(got, want) {
		t.Errorf("encodeVec(5) = %v, want %v", got, want)
	}
}

func TestEncodeName(t *testing.T) {
	var buf bytes.Buffer
	encodeName(&buf, "hello")
	got := buf.Bytes()
	// length prefix (5) + UTF-8 bytes for "hello"
	want := []byte{0x05, 'h', 'e', 'l', 'l', 'o'}
	if !bytes.Equal(got, want) {
		t.Errorf("encodeName(\"hello\") = %v, want %v", got, want)
	}
}
