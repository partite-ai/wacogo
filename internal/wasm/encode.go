// Package wasm provides helpers for building WebAssembly binaries programmatically.
package wasm

import "bytes"

// encodeU32 writes v as an unsigned LEB128-encoded value to w.
func encodeU32(w *bytes.Buffer, v uint32) {
	b := [5]byte{}
	n := 0
	for {
		b[n] = byte(v & 0x7F)
		v >>= 7
		if v != 0 {
			b[n] |= 0x80
		}
		n++
		if v == 0 {
			break
		}
	}
	w.Write(b[:n])
}

// encodeS32 writes v as a signed LEB128-encoded value to w.
func encodeS32(w *bytes.Buffer, v int32) {
	b := [5]byte{}
	n := 0
	for {
		b[n] = byte(v & 0x7F)
		v >>= 6 // arithmetic shift to check sign bit
		more := (v != 0 || b[n]&0x40 != 0) && (v != -1 || b[n]&0x40 == 0)
		v >>= 1 // complete the 7-bit shift
		if more {
			b[n] |= 0x80
		}
		n++
		if !more {
			break
		}
	}
	w.Write(b[:n])
}

// encodeS64 writes v as a signed LEB128-encoded value to w.
func encodeS64(w *bytes.Buffer, v int64) {
	b := [10]byte{}
	n := 0
	for {
		b[n] = byte(v & 0x7F)
		v >>= 6 // arithmetic shift to check sign bit
		more := (v != 0 || b[n]&0x40 != 0) && (v != -1 || b[n]&0x40 == 0)
		v >>= 1 // complete the 7-bit shift
		if more {
			b[n] |= 0x80
		}
		n++
		if !more {
			break
		}
	}
	w.Write(b[:n])
}

// encodeVec writes a vector length prefix (unsigned LEB128) to w.
func encodeVec(w *bytes.Buffer, length uint32) {
	encodeU32(w, length)
}

// encodeName writes a length-prefixed UTF-8 string to w.
func encodeName(w *bytes.Buffer, name string) {
	encodeU32(w, uint32(len(name)))
	w.Write([]byte(name))
}
